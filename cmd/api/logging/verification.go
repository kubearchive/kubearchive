// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package logging

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/kubearchive/kubearchive/pkg/database/interfaces"
	"github.com/ohler55/ojg/jp"
	"github.com/ohler55/ojg/oj"
)

const (
	defaultVerificationTailLines  = 10
	defaultVerificationInterval   = 5 * time.Second
	defaultVerificationMaxRetries = 6
	verificationSkipAge           = 5 * time.Minute
	coarseSkipFallbackAge         = 24 * time.Hour
)

type VerificationConfig struct {
	TailLines         int    `yaml:"tail-lines"`
	Interval          string `yaml:"interval"`
	MaxRetries        int    `yaml:"max-retries"`
	TimestampJsonPath string `yaml:"timestamp-json-path"`
	TimestampFormat   string `yaml:"timestamp-format"`
}

func (vc *VerificationConfig) defaults() {
	if vc.TailLines <= 0 {
		vc.TailLines = defaultVerificationTailLines
	}
	if vc.Interval == "" {
		vc.Interval = defaultVerificationInterval.String()
	}
	if vc.MaxRetries <= 0 {
		vc.MaxRetries = defaultVerificationMaxRetries
	}
}

func (vc *VerificationConfig) validate(providerURL string) error {
	if vc.Interval != "" {
		if _, err := time.ParseDuration(vc.Interval); err != nil {
			return fmt.Errorf("provider '%s' verification: invalid interval '%s': %w", providerURL, vc.Interval, err)
		}
	}
	if vc.TimestampJsonPath != "" {
		if _, err := jp.ParseString(vc.TimestampJsonPath); err != nil {
			return fmt.Errorf("provider '%s' verification: invalid timestamp-json-path '%s': %w", providerURL, vc.TimestampJsonPath, err)
		}
	}
	if vc.TimestampFormat != "" && vc.TimestampJsonPath == "" {
		return fmt.Errorf("provider '%s' verification: timestamp-format requires timestamp-json-path", providerURL)
	}
	return nil
}

func (vc *VerificationConfig) intervalDuration() time.Duration {
	d, _ := time.ParseDuration(vc.Interval)
	return d
}

type verificationResult struct {
	performed bool
	lastLine  string
	stable    bool
}

func parseTimestamp(raw string, format string) (time.Time, error) {
	switch format {
	case "unix_nano":
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return time.Time{}, fmt.Errorf("cannot parse '%s' as unix_nano: %w", raw, err)
		}
		return time.Unix(0, n), nil
	case "unix_ms":
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return time.Time{}, fmt.Errorf("cannot parse '%s' as unix_ms: %w", raw, err)
		}
		return time.Unix(0, n*int64(time.Millisecond)), nil
	case "unix_sec":
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return time.Time{}, fmt.Errorf("cannot parse '%s' as unix_sec: %w", raw, err)
		}
		return time.Unix(n, 0), nil
	default:
		return time.Parse(format, raw)
	}
}

// tryParseTimestamp attempts to parse a timestamp in common formats:
// integer-based (nanoseconds, milliseconds, seconds) and RFC3339.
func tryParseTimestamp(s string) (time.Time, error) {
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		if n > 1e15 {
			return time.Unix(0, n), nil
		}
		if n > 1e12 {
			return time.Unix(0, n*int64(time.Millisecond)), nil
		}
		return time.Unix(n, 0), nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf("cannot parse timestamp: %s", s)
}

func shouldCoarseSkip(record *interfaces.LogRecord, now time.Time) bool {
	if record.End != "" {
		if endTime, err := tryParseTimestamp(record.End); err == nil && now.After(endTime) {
			return true
		}
	}
	if record.End == "" && record.Start != "" {
		if startTime, err := tryParseTimestamp(record.Start); err == nil && now.After(startTime.Add(coarseSkipFallbackAge)) {
			return true
		}
	}
	return false
}

func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func performTailProbe(
	ctx context.Context,
	client *http.Client,
	tailEndpoint *ApiEndpoint,
	record *interfaces.LogRecord,
	headers map[string]string,
	tailLines int,
	verification *VerificationConfig,
) (lines []string, lastTimestamp time.Time, err error) {
	extraVars := map[string]string{
		"TAIL_LINES": strconv.Itoa(tailLines),
	}

	reqCtx, reqCancel := context.WithTimeout(ctx, tailEndpoint.requestTimeout())
	defer reqCancel()

	request, err := buildProviderRequest(reqCtx, tailEndpoint, record, extraVars)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("building tail probe request: %w", err)
	}

	for key, value := range headers {
		request.Header.Set(key, value)
	}

	response, err := client.Do(request)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("tail probe request failed: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 512))
		return nil, time.Time{}, fmt.Errorf("tail probe got status %d: %s", response.StatusCode, body)
	}

	var jsonPathParser jp.Expr
	if tailEndpoint.JsonPath != "" {
		jsonPathParser, err = jp.ParseString(tailEndpoint.JsonPath)
		if err != nil {
			return nil, time.Time{}, fmt.Errorf("invalid tail jsonPath: %w", err)
		}
	}

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("reading tail probe response: %w", err)
	}

	parsed, errParse := oj.Parse(body)
	if errParse != nil {
		reader := bufio.NewReader(bytes.NewReader(body))
		for {
			line, readErr := reader.ReadBytes('\n')
			if readErr != nil && readErr != io.EOF {
				break
			}
			parsedLines, _ := parseLine(line, jsonPathParser)
			lines = append(lines, parsedLines...)
			if readErr == io.EOF {
				break
			}
		}
	} else {
		lines = entriesToStrings(jsonPathParser.Get(parsed))

		if verification.TimestampJsonPath != "" && len(lines) > 0 {
			tsParser, tsErr := jp.ParseString(verification.TimestampJsonPath)
			if tsErr == nil {
				tsResults := tsParser.Get(parsed)
				if len(tsResults) > 0 {
					tsRaw := fmt.Sprintf("%v", tsResults[len(tsResults)-1])
					if ts, parseErr := parseTimestamp(tsRaw, verification.TimestampFormat); parseErr == nil {
						lastTimestamp = ts
					}
				}
			}
		}
	}

	return lines, lastTimestamp, nil
}

func performVerification(
	ctx context.Context,
	client *http.Client,
	tailEndpoint *ApiEndpoint,
	record *interfaces.LogRecord,
	headers map[string]string,
	verification *VerificationConfig,
	nowFunc func() time.Time,
) verificationResult {
	now := nowFunc()

	if shouldCoarseSkip(record, now) {
		slog.Info("Verification skipped: coarse time check passed",
			"pod", record.PodName, "end", record.End, "start", record.Start)
		return verificationResult{performed: false}
	}

	hasTimestampPath := verification.TimestampJsonPath != ""
	tailLines := verification.TailLines
	if hasTimestampPath {
		tailLines = 1
	}

	prevLines, prevTimestamp, err := performTailProbe(ctx, client, tailEndpoint, record, headers, tailLines, verification)
	if err != nil {
		slog.Warn("Verification tail probe failed, skipping verification", "error", err)
		return verificationResult{performed: false}
	}

	if hasTimestampPath && !prevTimestamp.IsZero() {
		age := now.Sub(prevTimestamp)
		if age > verificationSkipAge {
			slog.Info("Verification skipped: last entry is old enough",
				"age", age, "threshold", verificationSkipAge)
			lastLine := ""
			if len(prevLines) > 0 {
				lastLine = prevLines[len(prevLines)-1]
			}
			return verificationResult{performed: true, lastLine: lastLine, stable: true}
		}
	}

	interval := verification.intervalDuration()

	for attempt := range verification.MaxRetries {
		select {
		case <-ctx.Done():
			slog.Warn("Verification aborted: context cancelled")
			return verificationResult{performed: true, stable: false}
		case <-time.After(interval):
		}

		currLines, currTimestamp, err := performTailProbe(ctx, client, tailEndpoint, record, headers, tailLines, verification)
		if err != nil {
			slog.Warn("Verification poll failed", "attempt", attempt+1, "error", err)
			continue
		}

		stable := false
		if hasTimestampPath {
			stable = prevTimestamp.Equal(currTimestamp)
		} else {
			stable = slicesEqual(prevLines, currLines)
		}

		if stable {
			lastLine := ""
			if len(currLines) > 0 {
				lastLine = currLines[len(currLines)-1]
			}
			slog.Info("Verification: logs stable",
				"attempt", attempt+1, "pod", record.PodName)
			return verificationResult{performed: true, lastLine: lastLine, stable: true}
		}

		prevLines = currLines
		prevTimestamp = currTimestamp
	}

	lastLine := ""
	if len(prevLines) > 0 {
		lastLine = prevLines[len(prevLines)-1]
	}
	slog.Warn("Verification: max retries reached, logs may be incomplete",
		"maxRetries", verification.MaxRetries, "pod", record.PodName)
	return verificationResult{performed: true, lastLine: lastLine, stable: false}
}
