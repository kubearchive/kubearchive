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

func TestSetLoggingHeadersSuccess(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	t.Setenv("KUBEARCHIVE_LOGGING_DIR", "./testdata/headers")
	SetLoggingHeaders()(c)
	assert.Equal(t, c.GetStringMapString(loggingKey), map[string]string{
		"Authorization": "Basic YWRtaW46cGFzc3dvcmQ=",
		"X-Scope-OrgID": "tenant-id",
	})
}

func TestLogRetrievalSuccess(t *testing.T) {

	tests := []struct {
		name string
		logs bool
		err  bool
	}{
		{
			name: "logs available",
			logs: true,
			err:  false,
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

			c.Set("logURL", server.URL)
			c.Set("jsonPath", "$.message")

			c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
			LogRetrieval()(c)
			if !tt.err {
				if tt.logs {
					assert.Equal(t, http.StatusOK, res.Code)
					assert.Equal(t, res.Body.String(), "log-example1\nlog-example2\n")
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

func TestLogRetrievalError(t *testing.T) {

	tests := []struct {
		name          string
		contextValues map[string]string
		jsonPath      string
		expectedCode  int
		errStr        string
	}{
		{
			name:          "No URL set",
			contextValues: map[string]string{},
			jsonPath:      "",
			expectedCode:  http.StatusNotFound,
			errStr:        "no log URL found",
		},
		{
			name:          "Invalid JsonPath",
			contextValues: map[string]string{"logURL": "http://example.com"},
			jsonPath:      ".", // should start with $.
			expectedCode:  http.StatusInternalServerError,
			errStr:        "invalid jsonPath",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(res)
			c.Request = &http.Request{}
			c.Request = c.Request.WithContext(context.Background())

			for key, value := range tt.contextValues {
				c.Set(key, value)
			}
			if tt.jsonPath != "" {
				c.Set("jsonPath", tt.jsonPath)
			}
			LogRetrieval()(c)
			assert.Equal(t, tt.expectedCode, res.Code)
			assert.Contains(t, res.Body.String(), tt.errStr)
		})
	}
}
