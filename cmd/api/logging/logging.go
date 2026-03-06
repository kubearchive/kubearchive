// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package logging

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kubearchive/kubearchive/pkg/abort"
	"github.com/kubearchive/kubearchive/pkg/database/interfaces"
	"github.com/kubearchive/kubearchive/pkg/files"
	"github.com/ohler55/ojg/jp"
	"github.com/ohler55/ojg/oj"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"gopkg.in/yaml.v3"
)

const (
	headersCtxKey   string = "headers"
	providersCtxKey string = "providers"
	logProvidersKey string = "LOG_PROVIDERS"
	logRecordCtxKey string = "logRecord"
)

// ProviderConfig defines the tail and full strategies for a log provider
type ProviderConfig struct {
	Tail *ApiEndpoint `yaml:"tail"`
	Full *ApiEndpoint `yaml:"full"`
}

// ApiEndpoint defines how to query a logging backend
type ApiEndpoint struct {
	Reverse  bool                   `yaml:"reverse"`
	Path     string                 `yaml:"path"`
	Method   string                 `yaml:"method"`
	Params   map[string]interface{} `yaml:"params"`
	Body     map[string]interface{} `yaml:"body"`
	JsonPath string                 `yaml:"json-path"`
}

// validate checks that mandatory fields are set and applies defaults.
func (e *ApiEndpoint) validate(providerURL, endpointType string) error {
	if e.Path == "" {
		return fmt.Errorf("provider '%s' endpoint '%s': path is required", providerURL, endpointType)
	}
	if e.Method == "" {
		e.Method = http.MethodGet
	}
	if e.JsonPath == "" {
		e.JsonPath = "$."
	}
	return nil
}

// parseLoggingConfig reads the logging directory and parses both HEADERS and LOG_PROVIDERS.
func parseLoggingConfig() (map[string]map[string]string, map[string]ProviderConfig) {
	loggingDir, exists := os.LookupEnv(files.LoggingDirEnvVar)
	if !exists {
		slog.Warn(fmt.Sprintf("environment variable not set: %s", files.LoggingDirEnvVar))
		return nil, nil
	}
	configFiles, err := files.FilesInDir(loggingDir)
	if err != nil {
		slog.Warn("could not read kubearchive-logging config files", "err", err)
		return nil, nil
	}
	if len(configFiles) == 0 {
		slog.Warn("logging configuration not specified")
		return nil, nil
	}

	loggingConf, err := files.LoggingConfigFromFiles(configFiles)
	if err != nil {
		slog.Warn("could not read kubearchive-logging config files", "err", err)
		return nil, nil
	}

	var headers map[string]map[string]string
	if headersYAML, ok := loggingConf["HEADERS"]; ok && headersYAML != "" {
		headers = make(map[string]map[string]string)
		if err := yaml.Unmarshal([]byte(headersYAML), &headers); err != nil {
			slog.Warn("could not parse HEADERS YAML", "err", err)
			headers = nil
		}
	}

	var providers map[string]ProviderConfig
	if providersYAML, ok := loggingConf[logProvidersKey]; ok && providersYAML != "" {
		providers = make(map[string]ProviderConfig)
		if err := yaml.Unmarshal([]byte(providersYAML), &providers); err != nil {
			slog.Warn("could not parse LOG_PROVIDERS YAML", "err", err)
			providers = nil
		}
		for providerURL, config := range providers {
			if config.Full != nil {
				if err := config.Full.validate(providerURL, "full"); err != nil {
					slog.Error("invalid log provider configuration", "err", err)
					return nil, nil
				}
			}
			if config.Tail != nil {
				if err := config.Tail.validate(providerURL, "tail"); err != nil {
					slog.Error("invalid log provider configuration", "err", err)
					return nil, nil
				}
			}
		}
	}

	return headers, providers
}

// SetLoggingConfig reads the logging configuration and sets both headers and providers in the context.
func SetLoggingConfig() gin.HandlerFunc {
	headers, providers := parseLoggingConfig()
	return func(c *gin.Context) {
		c.Set(headersCtxKey, headers)
		c.Set(providersCtxKey, providers)
	}
}

// substituteVars replaces ${QUERY}, ${START}, ${END}, ${NAMESPACE}, and ${TAIL_LINES}
// placeholders with values from the LogRecord and request parameters.
func substituteVars(s string, record *interfaces.LogRecord, extraVars map[string]string) string {
	s = strings.ReplaceAll(s, "${NAMESPACE}", record.Namespace)
	s = strings.ReplaceAll(s, "${QUERY}", record.Query)
	s = strings.ReplaceAll(s, "${START}", record.Start)
	s = strings.ReplaceAll(s, "${END}", record.End)
	for k, v := range extraVars {
		s = strings.ReplaceAll(s, "${"+k+"}", v)
	}
	return s
}

// substituteVarsInMap performs variable substitution on all string values in a map
func substituteVarsInMap(m map[string]interface{}, record *interfaces.LogRecord, extraVars map[string]string) map[string]interface{} {
	result := make(map[string]interface{}, len(m))
	for k, v := range m {
		switch val := v.(type) {
		case string:
			// deepcode ignore Server-Side Request Forgery (SSRF): values come from admin-configured logging ConfigMap, not user input
			result[k] = substituteVars(val, record, extraVars)
		default:
			result[k] = v
		}
	}
	return result
}

// buildProviderRequest constructs an HTTP request from the endpoint configuration,
// substituting variables in the path, query parameters, and body.
func buildProviderRequest(ctx context.Context, endpoint *ApiEndpoint, record *interfaces.LogRecord, extraVars map[string]string) (*http.Request, error) {
	path := substituteVars(endpoint.Path, record, extraVars)

	// Parse and validate the constructed URL to prevent SSRF: ensure that
	// variable substitution in the path has not altered the target host.
	baseURL, err := url.Parse(record.URL)
	if err != nil {
		return nil, fmt.Errorf("invalid base URL: %w", err)
	}
	parsedURL, err := url.Parse(record.URL + path)
	if err != nil {
		return nil, fmt.Errorf("invalid request URL: %w", err)
	}
	if parsedURL.Host != baseURL.Host {
		return nil, fmt.Errorf("request URL host %q does not match expected host %q", parsedURL.Host, baseURL.Host)
	}
	requestURL := parsedURL.String()

	method := strings.ToUpper(endpoint.Method)

	if method == http.MethodGet {
		if len(endpoint.Params) > 0 {
			params := substituteVarsInMap(endpoint.Params, record, extraVars)
			var u *url.URL
			u, err = url.Parse(requestURL)
			if err != nil {
				return nil, err
			}
			q := u.Query()
			for k, v := range params {
				val := fmt.Sprintf("%v", v)
				if decoded, decErr := url.QueryUnescape(val); decErr == nil && decoded != val {
					// Value is already URL-encoded; set it directly on the raw query
					// to avoid double-encoding.
					if u.RawQuery != "" {
						u.RawQuery += "&"
					}
					u.RawQuery += url.QueryEscape(k) + "=" + val
				} else {
					q.Set(k, val)
				}
			}
			if len(q) > 0 {
				encoded := q.Encode()
				if u.RawQuery != "" {
					u.RawQuery += "&" + encoded
				} else {
					u.RawQuery = encoded
				}
			}
			requestURL = u.String()
		}
		return http.NewRequestWithContext(ctx, method, requestURL, nil)
	}

	// POST (or other method) with JSON body
	bodyMap := substituteVarsInMap(endpoint.Body, record, extraVars)
	bodyBytes, err := json.Marshal(bodyMap)
	if err != nil {
		return nil, err
	}
	// deepcode ignore Server-Side Request Forgery (SSRF): values come from admin-configured logging ConfigMap, not user input
	req, err := http.NewRequestWithContext(ctx, method, requestURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	return req, nil
}

func setLogResponseHeaders(c *gin.Context, record *interfaces.LogRecord) {
	c.Header("Content-Type", "text/plain; charset=utf-8")
	c.Header("X-Pod-Name", record.PodName)
	c.Header("X-Pod-UUID", record.PodUUID)
	c.Header("X-Container-Name", record.ContainerName)
}

func abortNoLogsFound(c *gin.Context, record *interfaces.LogRecord, requestURL string, mode string) {
	slog.Error("no logs found", "url", requestURL, "mode", mode) // #nosec G706 -- URL and mode come from admin config, not user input
	abort.Abort(c, fmt.Errorf("no logs found for the requested resource (pod %s/%s, container %s)", record.PodName, record.PodUUID, record.ContainerName), http.StatusNotFound)
}

// writeBufferedLogs reads all log entries into memory before writing the response.
// It is used when tailLines is requested (to return only the last N lines) or when the
// backend returns entries in reverse order (to re-order them). Entries are optionally
// reversed, flattened into individual lines, and truncated to tailLinesInt.
func writeBufferedLogs(c *gin.Context, reader *bufio.Reader, jsonPathParser jp.Expr, record *interfaces.LogRecord, requestURL string, mode string, reverse bool, tailLinesInt int) {
	isTailing := tailLinesInt > 0
	var allEntries []string
	var lineCount int
	for {
		line, readErr := reader.ReadBytes('\n')
		if readErr != nil && readErr != io.EOF {
			abort.Abort(c, readErr, http.StatusInternalServerError)
			return
		}
		parsedLines, errParseLine := parseLine(line, jsonPathParser)
		if errParseLine != nil {
			abort.Abort(c, errParseLine, http.StatusInternalServerError)
			return
		}
		for _, pl := range parsedLines {
			allEntries = append(allEntries, pl)
			lineCount += strings.Count(pl, "\n") + 1
		}
		if readErr == io.EOF {
			break
		}
		if isTailing && lineCount >= tailLinesInt {
			break
		}
	}

	if len(allEntries) == 0 {
		abortNoLogsFound(c, record, requestURL, mode)
		return
	}

	if reverse {
		slices.Reverse(allEntries)
	}

	// Flatten multi-line entries into individual lines
	var flatLines []string
	for _, entry := range allEntries {
		for _, l := range strings.Split(entry, "\n") {
			if l != "" {
				flatLines = append(flatLines, l)
			}
		}
	}

	// Truncate to the requested tailLines count since the backend's
	// limit applies to entries (which may contain multiple lines),
	// not individual lines
	if isTailing && len(flatLines) > tailLinesInt {
		flatLines = flatLines[len(flatLines)-tailLinesInt:]
	}

	setLogResponseHeaders(c, record)

	for _, l := range flatLines {
		c.String(http.StatusOK, l+"\n")
	}
}

// streamLogs writes log lines directly to the response as they are read, without buffering.
// It is used when returning full logs from a backend that delivers entries in forward order
// (i.e. tailLines is not set and the endpoint's Reverse flag is false).
func streamLogs(c *gin.Context, reader *bufio.Reader, jsonPathParser jp.Expr, record *interfaces.LogRecord, requestURL string, mode string) {
	var logsReturned bool
	for {
		line, readErr := reader.ReadBytes('\n')
		if readErr != nil && readErr != io.EOF {
			if logsReturned {
				slog.ErrorContext(c.Request.Context(), "error reading log line after streaming started", "error", readErr)
			} else {
				abort.Abort(c, readErr, http.StatusInternalServerError)
			}
			return
		}
		parsedLines, errParseLine := parseLine(line, jsonPathParser)
		if errParseLine != nil {
			if logsReturned {
				slog.ErrorContext(c.Request.Context(), "error parsing log line after streaming started", "error", errParseLine)
			} else {
				abort.Abort(c, errParseLine, http.StatusInternalServerError)
			}
			return
		}
		for _, parsedLine := range parsedLines {
			if !logsReturned {
				setLogResponseHeaders(c, record)
				c.Status(http.StatusOK)
				logsReturned = true
			}
			if _, err := fmt.Fprintln(c.Writer, parsedLine); err != nil { // #nosec G705 -- Content-Type is text/plain
				slog.ErrorContext(c.Request.Context(), "error writing log line", "error", err)
				return
			}
			c.Writer.Flush()
		}
		if readErr == io.EOF {
			break
		}
	}

	if !logsReturned {
		abortNoLogsFound(c, record, requestURL, mode)
	}
}

// LogRetrieval retrieves a middleware function that checks the logging config and when set
// expects to find a log record in the context from where to retrieve and parse logs.
// It reads LOG_PROVIDERS configuration to determine how to query each logging backend.
func LogRetrieval() gin.HandlerFunc {
	// FIXME For now the queries to the logging backend server are done insecurely. Needed for the current test env.
	client := http.Client{
		Transport: otelhttp.NewTransport(&http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}), // #nosec G402
		Timeout: 60 * time.Second,
	}

	return func(c *gin.Context) {
		recordVal, exists := c.Get(logRecordCtxKey)
		if !exists || recordVal == nil {
			abort.Abort(c, fmt.Errorf("no log URL found"), http.StatusNotFound)
			return
		}
		record, ok := recordVal.(*interfaces.LogRecord)
		if !ok || record == nil {
			abort.Abort(c, fmt.Errorf("no log URL found"), http.StatusNotFound)
			return
		}

		if record.URL == "" {
			abort.Abort(c, fmt.Errorf("no log URL found"), http.StatusNotFound)
			return
		}

		slog.Info("Log retrieval started",
			"record_url", record.URL,
			"pod", record.PodName,
			"pod_uuid", record.PodUUID,
			"container", record.ContainerName,
			"namespace", record.Namespace,
		)

		// Look up provider config from context
		var providers map[string]ProviderConfig
		if p, exists := c.Get(providersCtxKey); exists && p != nil {
			providers, _ = p.(map[string]ProviderConfig)
		}

		// Determine the mode based on tailLines query parameter
		tailLines := c.Query("tailLines")
		mode := "full"
		if tailLines != "" {
			mode = "tail"
		}
		slog.Info("Log retrieval mode determined", "mode", mode, "tailLines", tailLines)

		extraVars := map[string]string{
			"TAIL_LINES": tailLines,
		}

		// Look up provider config for this base URL.
		// If no provider matched, the record may contain an old full URL (pre-migration).
		// Extract the base URL, resolve the provider from it, and use the
		// full URL directly instead of building it from the endpoint path.
		providerKey := record.URL
		providerConfig, hasProvider := providers[providerKey]
		var legacyFullURL string
		if hasProvider {
			slog.Info("Provider found by direct URL match", "provider_key", providerKey)
		}
		if !hasProvider {
			baseURL, errBase := extractBaseURL(record.URL)
			if errBase != nil {
				abort.Abort(c, fmt.Errorf("no log provider configured for URL '%s' (pod %s/%s, container %s): %w", record.URL, record.PodName, record.PodUUID, record.ContainerName, errBase), http.StatusInternalServerError)
				return
			}
			providerKey = baseURL
			providerConfig, hasProvider = providers[providerKey]
			if !hasProvider {
				abort.Abort(c, fmt.Errorf("no log provider configured for base URL '%s' (pod %s/%s, container %s)", baseURL, record.PodName, record.PodUUID, record.ContainerName), http.StatusInternalServerError)
				return
			}
			if record.Query == "" {
				legacyFullURL = record.URL
				slog.Info("Resolved legacy full URL to provider", "url", record.URL, "base_url", baseURL)
			} else {
				record.URL = baseURL
				slog.Info("Resolved base URL from record URL", "original_url", record.URL, "base_url", baseURL)
			}
		}

		// Look up headers using the resolved provider key
		var headers map[string]string
		if allHeaders, exists := c.Get(headersCtxKey); exists && allHeaders != nil {
			if h, ok := allHeaders.(map[string]map[string]string); ok {
				headers = h[providerKey]
			}
		}

		if mode == "tail" && legacyFullURL != "" {
			abort.Abort(c, fmt.Errorf("tailing is not supported for legacy log URLs (pod %s/%s, container %s)", record.PodName, record.PodUUID, record.ContainerName), http.StatusBadRequest)
			return
		}

		var endpoint *ApiEndpoint
		switch mode {
		case "tail":
			endpoint = providerConfig.Tail
		default:
			endpoint = providerConfig.Full
		}
		if endpoint == nil {
			abort.Abort(c, fmt.Errorf("no '%s' endpoint configured for provider '%s' (pod %s/%s, container %s)", mode, providerKey, record.PodName, record.PodUUID, record.ContainerName), http.StatusInternalServerError)
			return
		}

		var request *http.Request
		var errReq error
		if legacyFullURL != "" {
			// Legacy full URL: use it as-is instead of interpolating variables
			// deepcode ignore Server-Side Request Forgery (SSRF): URL comes from the database, originally written by admin-configured sink
			request, errReq = http.NewRequestWithContext(c.Request.Context(), http.MethodGet, legacyFullURL, nil)
		} else {
			request, errReq = buildProviderRequest(c.Request.Context(), endpoint, record, extraVars)
		}
		if errReq != nil {
			abort.Abort(c, errReq, http.StatusInternalServerError)
			return
		}

		requestURL := request.URL.String()
		slog.Info("Sending request to log provider", // #nosec G706 -- URL comes from admin config or database records
			"request_url", requestURL,
			"method", request.Method,
			"mode", mode,
			"legacy", legacyFullURL != "",
		)

		for key, value := range headers {
			request.Header.Set(key, value)
		}

		response, errReq := client.Do(request) // #nosec G704 -- URL is built from admin-configured LOG_URL and endpoint paths
		if errReq != nil {
			abort.Abort(c, errReq, http.StatusInternalServerError)
			return
		}
		defer response.Body.Close()

		slog.Info("Received response from log provider", // #nosec G706 -- URL comes from admin config or database records
			"request_url", requestURL,
			"status_code", response.StatusCode,
		)

		if response.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(io.LimitReader(response.Body, 1024))
			abort.Abort(c, fmt.Errorf("error response: %d - %s", response.StatusCode, body), response.StatusCode)
			return
		}

		// Parse the jsonPath
		var jsonPathParser jp.Expr
		if endpoint.JsonPath != "" {
			var errJP error
			jsonPathParser, errJP = jp.ParseString(endpoint.JsonPath)
			if errJP != nil {
				abort.Abort(c, fmt.Errorf("invalid jsonPath %s: %w", endpoint.JsonPath, errJP), http.StatusInternalServerError)
				return
			}
		}

		reader := bufio.NewReader(response.Body)
		tailLinesInt, _ := strconv.Atoi(tailLines)

		if tailLinesInt > 0 || endpoint.Reverse {
			writeBufferedLogs(c, reader, jsonPathParser, record, requestURL, mode, endpoint.Reverse, tailLinesInt)
		} else {
			streamLogs(c, reader, jsonPathParser, record, requestURL, mode)
		}
	}
}

// extractBaseURL parses a URL and returns scheme://host (with port if present).
func extractBaseURL(rawURL string) (string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("invalid URL %q: %w", rawURL, err)
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("URL %q has no host", rawURL)
	}
	return fmt.Sprintf("%s://%s", parsed.Scheme, parsed.Host), nil
}

func parseLine(line []byte, jsonPathParser jp.Expr) ([]string, error) {
	if len(bytes.TrimSpace(line)) == 0 {
		return nil, nil
	}

	if jsonPathParser == nil {
		s := strings.TrimRight(string(line), "\n")
		if s == "" {
			return nil, nil
		}
		return []string{s}, nil
	}

	jsonLine, errJson := oj.Parse(line)
	if errJson != nil {
		return nil, errJson
	}

	var results []string
	for _, res := range jsonPathParser.Get(jsonLine) {
		s, ok := res.(string)
		if !ok {
			s = fmt.Sprintf("%v", res)
		}
		if s != "" {
			results = append(results, s)
		}
	}
	return results, nil
}
