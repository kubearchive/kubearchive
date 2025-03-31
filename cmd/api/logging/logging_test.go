// Copyright Kronicler Authors
// SPDX-License-Identifier: Apache-2.0

package logging

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func getServer() *httptest.Server {
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
}

func TestSetLoggingCredentialsSuccess(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	SetLoggingCredentials(map[string]string{userKey: "user", passwordKey: "pwd"}, nil)(c)
	assert.Equal(t, c.GetString(userKey), "user")
	assert.Equal(t, c.GetString(passwordKey), "pwd")
}

func TestSetLoggingCredentialsError(t *testing.T) {
	tests := []struct {
		name            string
		loggingCreds    map[string]string
		loggingCredsErr error
	}{
		{
			name:            "empty credentials",
			loggingCreds:    map[string]string{},
			loggingCredsErr: fmt.Errorf("logging secret is empty"),
		},
		{
			name:            "no user",
			loggingCreds:    map[string]string{passwordKey: "pwd"},
			loggingCredsErr: nil,
		},
		{
			name:            "no pwd",
			loggingCreds:    map[string]string{userKey: "user"},
			loggingCredsErr: nil,
		},
	}
	for _, tt := range tests {
		res := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(res)
		SetLoggingCredentials(tt.loggingCreds, tt.loggingCredsErr)(c)
		assert.Equal(t, http.StatusBadRequest, res.Code)
		assert.Contains(t, res.Body.String(), "logging secret user or password unset")
	}
}

func TestLogRetrievalSuccess(t *testing.T) {

	res := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(res)

	server := getServer()
	defer server.Close()
	c.Set(userKey, "user")
	c.Set(passwordKey, "password")
	c.Set("logURL", server.URL)
	c.Set("jsonPath", "$.message")

	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	LogRetrieval()(c)
	assert.Equal(t, http.StatusOK, res.Code)
	assert.Equal(t, res.Body.String(), "log-example1\nlog-example2\n")
	assert.Contains(t, res.Body.String(), "log-example1")
	assert.Contains(t, res.Body.String(), "log-example2")
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
			name:          "Empty kronicler-logging secret",
			contextValues: map[string]string{"logURL": "http://example.com"},
			jsonPath:      "",
			expectedCode:  http.StatusInternalServerError,
			errStr:        "Logging credentials are unset",
		},
		{
			name:          "No user in kronicler-logging secret",
			contextValues: map[string]string{passwordKey: "pwd", "logURL": "http://example.com"},
			jsonPath:      "",
			expectedCode:  http.StatusInternalServerError,
			errStr:        "Logging credentials are unset",
		},
		{
			name:          "No password in kronicler-logging secret",
			contextValues: map[string]string{userKey: "user", "logURL": "http://example.com"},
			jsonPath:      "",
			expectedCode:  http.StatusInternalServerError,
			errStr:        "Logging credentials are unset",
		},
		{
			name:          "No URL set",
			contextValues: map[string]string{userKey: "user", passwordKey: "pwd"},
			jsonPath:      "",
			expectedCode:  http.StatusNotFound,
			errStr:        "no log URL found",
		},
		{
			name:          "Invalid JsonPath",
			contextValues: map[string]string{userKey: "user", passwordKey: "pwd", "logURL": "http://example.com"},
			jsonPath:      ".", // should start with $.
			expectedCode:  http.StatusInternalServerError,
			errStr:        "invalid jsonPath",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(res)
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
