// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package logging

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kubearchive/kubearchive/pkg/database/interfaces"
	"github.com/stretchr/testify/assert"
)

func getServer(withLogs bool, retErr bool) *httptest.Server {
	if retErr {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			_, err := fmt.Fprintln(w, "no logs found")
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
		}))
	}
	if withLogs {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, err := fmt.Fprintln(w, `{"message":"log-example1"}`)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			_, err = fmt.Fprintf(w, `{"message":"log-example2"}`)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusOK)
		}))
	} else {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, err := fmt.Fprintln(w, `{"message":""}`)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
			}
			w.WriteHeader(http.StatusOK)
		}))
	}
}

func TestSetLoggingConfigSuccess(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	t.Setenv("KUBEARCHIVE_LOGGING_DIR", "./testdata/config")
	SetLoggingConfig()(c)

	val, exists := c.Get(headersCtxKey)
	assert.True(t, exists)
	headers, ok := val.(map[string]map[string]string)
	assert.True(t, ok)
	assert.Equal(t, map[string]map[string]string{
		"http://localhost:8080": {
			"Authorization": "Basic YWRtaW46cGFzc3dvcmQ=",
			"X-Scope-OrgID": "tenant-id",
		},
	}, headers)

	pVal, pExists := c.Get(providersCtxKey)
	assert.True(t, pExists)
	providers, ok := pVal.(map[string]ProviderConfig)
	assert.True(t, ok)
	assert.Contains(t, providers, "http://localhost:8080")
	assert.NotNil(t, providers["http://localhost:8080"].Tail)
	assert.NotNil(t, providers["http://localhost:8080"].Full)
}

func TestLogRetrievalSuccess(t *testing.T) {

	tests := []struct {
		name      string
		logs      bool
		err       bool
		tailLines string
	}{
		{
			name: "logs available",
			logs: true,
			err:  false,
		},
		{
			name:      "logs available with tailLines",
			logs:      true,
			err:       false,
			tailLines: "50",
		},
		{
			name: "no logs available",
			logs: false,
			err:  false,
		},
		{
			name: "error returned",
			logs: false,
			err:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(res)

			server := getServer(tt.logs, tt.err)
			defer server.Close()

			c.Set(headersCtxKey, map[string]map[string]string{
				server.URL: {
					"Authorization": "Basic YWRtaW46cGFzc3dvcmQ=",
				},
			})
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

			requestURL := "/"
			if tt.tailLines != "" {
				requestURL = "/?tailLines=" + tt.tailLines
			}
			c.Request = httptest.NewRequest(http.MethodGet, requestURL, nil)
			LogRetrieval()(c)
			if !tt.err {
				if tt.logs {
					assert.Equal(t, http.StatusOK, res.Code)
					assert.Equal(t, res.Body.String(), "log-example1\nlog-example2\n")
					assert.Equal(t, "test-pod", res.Header().Get("X-Pod-Name"))
					assert.Equal(t, "abc-123", res.Header().Get("X-Pod-UUID"))
					assert.Equal(t, "test-container", res.Header().Get("X-Container-Name"))
				} else {
					assert.Equal(t, http.StatusNotFound, res.Code)
					assert.Contains(t, res.Body.String(), "no logs found for the requested resource")
				}
			} else {
				assert.Equal(t, http.StatusNotFound, res.Code)
				assert.Contains(t, res.Body.String(), "no logs found")
			}
		})
	}
}

func TestLogRetrievalLegacyFullURL(t *testing.T) {
	tests := []struct {
		name         string
		logs         bool
		err          bool
		tailLines    string
		expectedCode int
		errStr       string
	}{
		{
			name:         "legacy full URL with logs",
			logs:         true,
			expectedCode: http.StatusOK,
		},
		{
			name:         "legacy full URL with tailLines rejected",
			logs:         true,
			tailLines:    "50",
			expectedCode: http.StatusBadRequest,
			errStr:       "tailing is not supported for legacy log URLs",
		},
		{
			name:         "legacy full URL no logs",
			logs:         false,
			expectedCode: http.StatusNotFound,
			errStr:       "no logs found for the requested resource",
		},
		{
			name:         "legacy full URL error response",
			logs:         false,
			err:          true,
			expectedCode: http.StatusNotFound,
			errStr:       "no logs found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(res)

			server := getServer(tt.logs, tt.err)
			defer server.Close()

			// Provider and headers are registered by base URL only
			c.Set(headersCtxKey, map[string]map[string]string{
				server.URL: {
					"Authorization": "Basic YWRtaW46cGFzc3dvcmQ=",
				},
			})
			c.Set(providersCtxKey, map[string]ProviderConfig{
				server.URL: {
					Full: &ApiEndpoint{
						Path:     "/",
						JsonPath: "$.message",
					},
					Tail: &ApiEndpoint{
						Path:     "/",
						JsonPath: "$.message",
					},
				},
			})
			// Record URL is a legacy full URL with path and query params
			c.Set(logRecordCtxKey, &interfaces.LogRecord{
				URL:           server.URL + "/loki/api/v1/query_range?query=%7Bapp%3D%22test%22%7D&start=1000&end=2000",
				PodName:       "test-pod",
				PodUUID:       "abc-123",
				ContainerName: "test-container",
			})

			requestURL := "/"
			if tt.tailLines != "" {
				requestURL = "/?tailLines=" + tt.tailLines
			}
			c.Request = httptest.NewRequest(http.MethodGet, requestURL, nil)
			LogRetrieval()(c)

			assert.Equal(t, tt.expectedCode, res.Code)
			if tt.errStr != "" {
				assert.Contains(t, res.Body.String(), tt.errStr)
			}
			if tt.expectedCode == http.StatusOK {
				assert.Equal(t, "log-example1\nlog-example2\n", res.Body.String())
				assert.Equal(t, "test-pod", res.Header().Get("X-Pod-Name"))
				assert.Equal(t, "abc-123", res.Header().Get("X-Pod-UUID"))
				assert.Equal(t, "test-container", res.Header().Get("X-Container-Name"))
			}
		})
	}
}

func TestLogRetrievalLegacyFullURLNoProvider(t *testing.T) {
	res := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(res)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	// Provider registered for a different base URL
	c.Set(providersCtxKey, map[string]ProviderConfig{
		"http://other-host:3100": {
			Full: &ApiEndpoint{JsonPath: "$.message"},
		},
	})
	c.Set(logRecordCtxKey, &interfaces.LogRecord{
		URL:           "http://unknown-host:3100/loki/api/v1/query_range?query=test",
		PodName:       "my-pod",
		PodUUID:       "uid-456",
		ContainerName: "my-container",
	})

	LogRetrieval()(c)
	assert.Equal(t, http.StatusInternalServerError, res.Code)
	assert.Contains(t, res.Body.String(), "no log provider configured for base URL")
}

func TestLogRetrievalError(t *testing.T) {

	tests := []struct {
		name         string
		logRecord    *interfaces.LogRecord
		expectedCode int
		errStr       string
	}{
		{
			name:         "No record set",
			logRecord:    nil,
			expectedCode: http.StatusNotFound,
			errStr:       "no log URL found",
		},
		{
			name:         "Empty URL",
			logRecord:    &interfaces.LogRecord{URL: ""},
			expectedCode: http.StatusNotFound,
			errStr:       "no log URL found",
		},
		{
			name:         "No provider configured",
			logRecord:    &interfaces.LogRecord{URL: "http://unknown-host:1234", PodName: "my-pod", PodUUID: "uid-456", ContainerName: "my-container"},
			expectedCode: http.StatusInternalServerError,
			errStr:       "no log provider configured",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(res)
			c.Request = &http.Request{}
			c.Request = c.Request.WithContext(context.Background())

			if tt.logRecord != nil {
				c.Set(logRecordCtxKey, tt.logRecord)
			}
			LogRetrieval()(c)
			assert.Equal(t, tt.expectedCode, res.Code)
			assert.Contains(t, res.Body.String(), tt.errStr)
		})
	}
}
