// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package logging

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kubearchive/kubearchive/pkg/database/interfaces"
	"github.com/stretchr/testify/assert"
)

func TestParseTimestamp(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		format  string
		want    time.Time
		wantErr bool
	}{
		{
			name:   "unix_nano",
			raw:    "1746879420000000000",
			format: "unix_nano",
			want:   time.Unix(0, 1746879420000000000),
		},
		{
			name:   "unix_ms",
			raw:    "1746879420000",
			format: "unix_ms",
			want:   time.Unix(0, 1746879420000*int64(time.Millisecond)),
		},
		{
			name:   "unix_sec",
			raw:    "1746879420",
			format: "unix_sec",
			want:   time.Unix(1746879420, 0),
		},
		{
			name:   "go time layout RFC3339",
			raw:    "2025-05-09T12:17:00Z",
			format: time.RFC3339,
			want:   time.Date(2025, 5, 9, 12, 17, 0, 0, time.UTC),
		},
		{
			name:   "custom go time layout",
			raw:    "2025-05-09T12:17:00.000Z",
			format: "2006-01-02T15:04:05.000Z",
			want:   time.Date(2025, 5, 9, 12, 17, 0, 0, time.UTC),
		},
		{
			name:    "unix_nano invalid",
			raw:     "not-a-number",
			format:  "unix_nano",
			wantErr: true,
		},
		{
			name:    "unix_ms invalid",
			raw:     "abc",
			format:  "unix_ms",
			wantErr: true,
		},
		{
			name:    "unix_sec invalid",
			raw:     "xyz",
			format:  "unix_sec",
			wantErr: true,
		},
		{
			name:    "bad go layout",
			raw:     "not-a-date",
			format:  time.RFC3339,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseTimestamp(tt.raw, tt.format)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.True(t, tt.want.Equal(got), "want %v, got %v", tt.want, got)
		})
	}
}

func TestTryParseTimestamp(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    time.Time
		wantErr bool
	}{
		{
			name:  "nanoseconds",
			input: "1746879420000000000",
			want:  time.Unix(0, 1746879420000000000),
		},
		{
			name:  "milliseconds",
			input: "1746879420000",
			want:  time.Unix(0, 1746879420000*int64(time.Millisecond)),
		},
		{
			name:  "seconds",
			input: "1746879420",
			want:  time.Unix(1746879420, 0),
		},
		{
			name:  "RFC3339",
			input: "2025-05-09T12:17:00Z",
			want:  time.Date(2025, 5, 9, 12, 17, 0, 0, time.UTC),
		},
		{
			name:  "RFC3339Nano",
			input: "2025-05-09T12:17:00.123456789Z",
			want:  time.Date(2025, 5, 9, 12, 17, 0, 123456789, time.UTC),
		},
		{
			name:    "unparseable",
			input:   "not-a-timestamp",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tryParseTimestamp(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.True(t, tt.want.Equal(got), "want %v, got %v", tt.want, got)
		})
	}
}

func TestShouldCoarseSkip(t *testing.T) {
	now := time.Date(2025, 5, 10, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name   string
		record *interfaces.LogRecord
		want   bool
	}{
		{
			name:   "END in the past (unix_nano)",
			record: &interfaces.LogRecord{End: fmt.Sprintf("%d", now.Add(-time.Hour).UnixNano())},
			want:   true,
		},
		{
			name:   "END in the future (unix_nano)",
			record: &interfaces.LogRecord{End: fmt.Sprintf("%d", now.Add(time.Hour).UnixNano())},
			want:   false,
		},
		{
			name:   "no END, START + 24h in the past",
			record: &interfaces.LogRecord{Start: fmt.Sprintf("%d", now.Add(-25*time.Hour).UnixNano())},
			want:   true,
		},
		{
			name:   "no END, START + 24h in the future",
			record: &interfaces.LogRecord{Start: fmt.Sprintf("%d", now.Add(-23*time.Hour).UnixNano())},
			want:   false,
		},
		{
			name:   "neither START nor END",
			record: &interfaces.LogRecord{},
			want:   false,
		},
		{
			name:   "END set as RFC3339 in the past",
			record: &interfaces.LogRecord{End: "2025-05-10T10:00:00Z"},
			want:   true,
		},
		{
			name:   "unparseable END",
			record: &interfaces.LogRecord{End: "invalid-end"},
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldCoarseSkip(tt.record, now)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestSlicesEqual(t *testing.T) {
	assert.True(t, slicesEqual([]string{"a", "b"}, []string{"a", "b"}))
	assert.False(t, slicesEqual([]string{"a", "b"}, []string{"a", "c"}))
	assert.False(t, slicesEqual([]string{"a"}, []string{"a", "b"}))
	assert.True(t, slicesEqual(nil, nil))
	assert.True(t, slicesEqual([]string{}, []string{}))
}

func TestVerificationConfigDefaults(t *testing.T) {
	vc := &VerificationConfig{}
	vc.defaults()
	assert.Equal(t, defaultVerificationTailLines, vc.TailLines)
	assert.Equal(t, defaultVerificationInterval.String(), vc.Interval)
	assert.Equal(t, defaultVerificationMaxRetries, vc.MaxRetries)
}

func TestVerificationConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		config  *VerificationConfig
		wantErr string
	}{
		{
			name:   "valid with timestamp path",
			config: &VerificationConfig{Interval: "5s", TimestampJsonPath: "$.data[0]", TimestampFormat: "unix_nano"},
		},
		{
			name:   "valid without timestamp path",
			config: &VerificationConfig{Interval: "5s"},
		},
		{
			name:    "invalid interval",
			config:  &VerificationConfig{Interval: "not-a-duration"},
			wantErr: "invalid interval",
		},
		{
			name:    "invalid timestamp json path",
			config:  &VerificationConfig{Interval: "5s", TimestampJsonPath: "[$$$invalid"},
			wantErr: "invalid timestamp-json-path",
		},
		{
			name:    "timestamp format without json path",
			config:  &VerificationConfig{Interval: "5s", TimestampFormat: "unix_nano"},
			wantErr: "timestamp-format requires timestamp-json-path",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.validate("http://test")
			if tt.wantErr != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// lokiResponse builds a Loki-style JSON response with the given log lines and timestamps.
func lokiResponse(lines []string, timestamps []string) []byte {
	type valueEntry [2]string
	var values []valueEntry
	for i, line := range lines {
		ts := ""
		if i < len(timestamps) {
			ts = timestamps[i]
		}
		values = append(values, valueEntry{ts, line})
	}

	resp := map[string]any{
		"data": map[string]any{
			"result": []map[string]any{
				{
					"values": values,
				},
			},
		},
	}
	b, _ := json.Marshal(resp)
	return b
}

func TestPerformTailProbe(t *testing.T) {
	ts := "1746879420000000000"
	body := lokiResponse([]string{"log-line-1"}, []string{ts})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write(body)
	}))
	defer server.Close()

	endpoint := &ApiEndpoint{
		Path:     "/",
		Method:   "GET",
		JsonPath: "$.data.result[*].values[*][1]",
	}
	record := &interfaces.LogRecord{URL: server.URL}
	verification := &VerificationConfig{
		TimestampJsonPath: "$.data.result[*].values[*][0]",
		TimestampFormat:   "unix_nano",
	}

	lines, lastTs, err := performTailProbe(context.Background(), server.Client(), endpoint, record, nil, 1, verification)
	assert.NoError(t, err)
	assert.Equal(t, []string{"log-line-1"}, lines)
	assert.True(t, time.Unix(0, 1746879420000000000).Equal(lastTs))
}

func TestPerformTailProbeError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintln(w, "internal error")
	}))
	defer server.Close()

	endpoint := &ApiEndpoint{Path: "/", Method: "GET", JsonPath: "$.message"}
	record := &interfaces.LogRecord{URL: server.URL}
	verification := &VerificationConfig{}

	_, _, err := performTailProbe(context.Background(), server.Client(), endpoint, record, nil, 10, verification)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "tail probe got status 500")
}

func TestPerformVerificationCoarseSkip(t *testing.T) {
	now := time.Date(2025, 5, 10, 12, 0, 0, 0, time.UTC)
	record := &interfaces.LogRecord{
		URL:     "http://unused",
		PodName: "test-pod",
		End:     fmt.Sprintf("%d", now.Add(-time.Hour).UnixNano()),
	}

	result := performVerification(
		context.Background(),
		http.DefaultClient,
		&ApiEndpoint{Path: "/", Method: "GET"},
		record,
		nil,
		&VerificationConfig{Interval: "100ms", MaxRetries: 1},
		func() time.Time { return now },
	)

	assert.False(t, result.performed)
}

func TestPerformVerificationFineGrainedSkip(t *testing.T) {
	now := time.Date(2025, 5, 10, 12, 0, 0, 0, time.UTC)
	oldTs := fmt.Sprintf("%d", now.Add(-10*time.Minute).UnixNano())

	body := lokiResponse([]string{"old-log-line"}, []string{oldTs})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(body)
	}))
	defer server.Close()

	record := &interfaces.LogRecord{URL: server.URL, PodName: "test-pod"}

	result := performVerification(
		context.Background(),
		server.Client(),
		&ApiEndpoint{Path: "/", Method: "GET", JsonPath: "$.data.result[*].values[*][1]"},
		record,
		nil,
		&VerificationConfig{
			Interval:          "100ms",
			MaxRetries:        3,
			TimestampJsonPath: "$.data.result[*].values[*][0]",
			TimestampFormat:   "unix_nano",
		},
		func() time.Time { return now },
	)

	assert.True(t, result.performed)
	assert.True(t, result.stable)
	assert.Equal(t, "old-log-line", result.lastLine)
}

func TestPerformVerificationImmediateStability(t *testing.T) {
	now := time.Date(2025, 5, 10, 12, 0, 0, 0, time.UTC)
	recentTs := fmt.Sprintf("%d", now.Add(-30*time.Second).UnixNano())

	body := lokiResponse([]string{"recent-log"}, []string{recentTs})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(body)
	}))
	defer server.Close()

	record := &interfaces.LogRecord{URL: server.URL, PodName: "test-pod"}

	result := performVerification(
		context.Background(),
		server.Client(),
		&ApiEndpoint{Path: "/", Method: "GET", JsonPath: "$.data.result[*].values[*][1]"},
		record,
		nil,
		&VerificationConfig{
			Interval:          "100ms",
			MaxRetries:        3,
			TimestampJsonPath: "$.data.result[*].values[*][0]",
			TimestampFormat:   "unix_nano",
		},
		func() time.Time { return now },
	)

	assert.True(t, result.performed)
	assert.True(t, result.stable)
	assert.Equal(t, "recent-log", result.lastLine)
}

func TestPerformVerificationStabilityAfterRetries(t *testing.T) {
	now := time.Date(2025, 5, 10, 12, 0, 0, 0, time.UTC)
	ts1 := fmt.Sprintf("%d", now.Add(-10*time.Second).UnixNano())
	ts2 := fmt.Sprintf("%d", now.Add(-5*time.Second).UnixNano())

	var callCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := callCount.Add(1)
		if n <= 2 {
			ts := ts1
			if n == 2 {
				ts = ts2
			}
			w.Write(lokiResponse([]string{fmt.Sprintf("log-%d", n)}, []string{ts}))
		} else {
			w.Write(lokiResponse([]string{"log-stable"}, []string{ts2}))
		}
	}))
	defer server.Close()

	record := &interfaces.LogRecord{URL: server.URL, PodName: "test-pod"}

	result := performVerification(
		context.Background(),
		server.Client(),
		&ApiEndpoint{Path: "/", Method: "GET", JsonPath: "$.data.result[*].values[*][1]"},
		record,
		nil,
		&VerificationConfig{
			Interval:          "50ms",
			MaxRetries:        5,
			TimestampJsonPath: "$.data.result[*].values[*][0]",
			TimestampFormat:   "unix_nano",
		},
		func() time.Time { return now },
	)

	assert.True(t, result.performed)
	assert.True(t, result.stable)
	assert.Equal(t, "log-stable", result.lastLine)
}

func TestPerformVerificationMaxRetriesExhausted(t *testing.T) {
	now := time.Date(2025, 5, 10, 12, 0, 0, 0, time.UTC)

	var callCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := callCount.Add(1)
		ts := fmt.Sprintf("%d", now.Add(-time.Duration(n)*time.Second).UnixNano())
		w.Write(lokiResponse([]string{fmt.Sprintf("log-%d", n)}, []string{ts}))
	}))
	defer server.Close()

	record := &interfaces.LogRecord{URL: server.URL, PodName: "test-pod"}

	result := performVerification(
		context.Background(),
		server.Client(),
		&ApiEndpoint{Path: "/", Method: "GET", JsonPath: "$.data.result[*].values[*][1]"},
		record,
		nil,
		&VerificationConfig{
			Interval:          "50ms",
			MaxRetries:        2,
			TimestampJsonPath: "$.data.result[*].values[*][0]",
			TimestampFormat:   "unix_nano",
		},
		func() time.Time { return now },
	)

	assert.True(t, result.performed)
	assert.False(t, result.stable)
}

func TestPerformVerificationFallbackContentComparison(t *testing.T) {
	var callCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		w.Write(lokiResponse([]string{"line-a", "line-b"}, nil))
	}))
	defer server.Close()

	record := &interfaces.LogRecord{URL: server.URL, PodName: "test-pod"}

	result := performVerification(
		context.Background(),
		server.Client(),
		&ApiEndpoint{Path: "/", Method: "GET", JsonPath: "$.data.result[*].values[*][1]"},
		record,
		nil,
		&VerificationConfig{
			Interval:   "50ms",
			MaxRetries: 3,
			TailLines:  10,
		},
		time.Now,
	)

	assert.True(t, result.performed)
	assert.True(t, result.stable)
	assert.Equal(t, "line-b", result.lastLine)
}

func TestPerformVerificationTailProbeError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	record := &interfaces.LogRecord{URL: server.URL, PodName: "test-pod"}

	result := performVerification(
		context.Background(),
		server.Client(),
		&ApiEndpoint{Path: "/", Method: "GET", JsonPath: "$.message"},
		record,
		nil,
		&VerificationConfig{Interval: "50ms", MaxRetries: 1},
		time.Now,
	)

	assert.False(t, result.performed)
}

func TestLogRetrievalWithVerificationEndToEnd(t *testing.T) {
	recentTs := fmt.Sprintf("%d", time.Now().Add(-2*time.Second).UnixNano())

	var tailProbes atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		limit := r.URL.Query().Get("limit")
		if limit == "1" {
			tailProbes.Add(1)
			w.Write(lokiResponse([]string{"last-log-line"}, []string{recentTs}))
		} else {
			fmt.Fprintln(w, `{"message":"log-line-1"}`)
			fmt.Fprintln(w, `{"message":"last-log-line"}`)
		}
	}))
	defer server.Close()

	res := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(res)

	c.Set(providersCtxKey, map[string]ProviderConfig{
		server.URL: {
			Full: &ApiEndpoint{
				Path:     "/",
				Method:   "GET",
				JsonPath: "$.message",
				Params: map[string]any{
					"limit": "300",
				},
				Verification: &VerificationConfig{
					Interval:          "50ms",
					MaxRetries:        3,
					TailLines:         1,
					TimestampJsonPath: "$.data.result[*].values[*][0]",
					TimestampFormat:   "unix_nano",
				},
			},
			Tail: &ApiEndpoint{
				Path:     "/",
				Method:   "GET",
				JsonPath: "$.data.result[*].values[*][1]",
				Params: map[string]any{
					"limit": "${TAIL_LINES}",
				},
			},
		},
	})
	c.Set(logRecordCtxKey, &interfaces.LogRecord{
		URL:           server.URL,
		PodName:       "test-pod",
		PodUUID:       "abc-123",
		ContainerName: "test-container",
	})

	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	LogRetrieval()(c)

	assert.Equal(t, http.StatusOK, res.Code)
	assert.Contains(t, res.Body.String(), "log-line-1")
	assert.True(t, tailProbes.Load() >= 2, "expected at least 2 tail probes, got %d", tailProbes.Load())
	assert.Equal(t, "X-Logs-Incomplete", res.Header().Get("Trailer"))
}

func TestLogRetrievalNoVerificationForTailMode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, `{"message":"log-line-1"}`)
	}))
	defer server.Close()

	res := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(res)

	c.Set(providersCtxKey, map[string]ProviderConfig{
		server.URL: {
			Full: &ApiEndpoint{
				Path:     "/",
				Method:   "GET",
				JsonPath: "$.message",
				Verification: &VerificationConfig{
					Interval:   "50ms",
					MaxRetries: 3,
					TailLines:  1,
				},
			},
			Tail: &ApiEndpoint{
				Path:     "/",
				Method:   "GET",
				JsonPath: "$.message",
			},
		},
	})
	c.Set(logRecordCtxKey, &interfaces.LogRecord{
		URL:           server.URL,
		PodName:       "test-pod",
		PodUUID:       "abc-123",
		ContainerName: "test-container",
	})

	c.Request = httptest.NewRequest(http.MethodGet, "/?tailLines=10", nil)
	LogRetrieval()(c)

	assert.Equal(t, http.StatusOK, res.Code)
	assert.Empty(t, res.Header().Get("Trailer"))
}

func TestLogRetrievalNoVerificationWhenNotConfigured(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, `{"message":"log-line-1"}`)
	}))
	defer server.Close()

	res := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(res)

	c.Set(providersCtxKey, map[string]ProviderConfig{
		server.URL: {
			Full: &ApiEndpoint{
				Path:     "/",
				Method:   "GET",
				JsonPath: "$.message",
			},
			Tail: &ApiEndpoint{
				Path:     "/",
				Method:   "GET",
				JsonPath: "$.message",
			},
		},
	})
	c.Set(logRecordCtxKey, &interfaces.LogRecord{
		URL:           server.URL,
		PodName:       "test-pod",
		PodUUID:       "abc-123",
		ContainerName: "test-container",
	})

	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	LogRetrieval()(c)

	assert.Equal(t, http.StatusOK, res.Code)
	assert.Empty(t, res.Header().Get("Trailer"))
}

func TestLogRetrievalVerificationUnstable(t *testing.T) {
	var callCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := callCount.Add(1)
		limit := r.URL.Query().Get("limit")
		if limit == "10" {
			w.Write(lokiResponse([]string{fmt.Sprintf("changing-line-%d", n)}, nil))
		} else {
			fmt.Fprintln(w, `{"message":"full-log-line"}`)
		}
	}))
	defer server.Close()

	res := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(res)

	c.Set(providersCtxKey, map[string]ProviderConfig{
		server.URL: {
			Full: &ApiEndpoint{
				Path:     "/",
				Method:   "GET",
				JsonPath: "$.message",
				Params: map[string]any{
					"limit": "300",
				},
				Verification: &VerificationConfig{
					Interval:   "50ms",
					MaxRetries: 2,
					TailLines:  10,
				},
			},
			Tail: &ApiEndpoint{
				Path:     "/",
				Method:   "GET",
				JsonPath: "$.data.result[*].values[*][1]",
				Params: map[string]any{
					"limit": "${TAIL_LINES}",
				},
			},
		},
	})
	c.Set(logRecordCtxKey, &interfaces.LogRecord{
		URL:           server.URL,
		PodName:       "test-pod",
		PodUUID:       "abc-123",
		ContainerName: "test-container",
	})

	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	LogRetrieval()(c)

	assert.Equal(t, http.StatusOK, res.Code)
	assert.Equal(t, "true", res.Header().Get("X-Logs-Incomplete"))
}
