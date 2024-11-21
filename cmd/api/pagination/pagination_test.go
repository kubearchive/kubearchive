// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0
package pagination

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestMiddleware(t *testing.T) {
	tests := []struct {
		name               string
		query              string
		expectedLimit      string
		expectedInt64      string
		expectedDate       string
		expectedStatusCode int
		expectedBody       string
	}{
		{
			name:               "default limit is applied",
			query:              "/",
			expectedLimit:      "100",
			expectedInt64:      "",
			expectedDate:       "",
			expectedStatusCode: http.StatusOK,
			expectedBody:       "",
		},
		{
			name:               "invalid limit",
			query:              "/?limit=abc",
			expectedLimit:      "",
			expectedInt64:      "",
			expectedDate:       "",
			expectedStatusCode: http.StatusBadRequest,
			expectedBody:       `{"message":"limit 'abc' could not be converted to integer"}`,
		},
		{
			name:               "valid limit",
			query:              "/?limit=250",
			expectedLimit:      "250",
			expectedInt64:      "",
			expectedDate:       "",
			expectedStatusCode: http.StatusOK,
			expectedBody:       "",
		},
		{
			name:               "limit too large",
			query:              "/?limit=2000",
			expectedLimit:      "",
			expectedInt64:      "",
			expectedDate:       "",
			expectedStatusCode: http.StatusBadRequest,
			expectedBody:       `{"message":"limit '2000' exceeds the maximum allowed '1000'"}`,
		},
		{
			name:               "invalid continue",
			query:              "/?continue=abc",
			expectedLimit:      "",
			expectedInt64:      "",
			expectedDate:       "",
			expectedStatusCode: http.StatusBadRequest,
			expectedBody:       `{"message":"could not decode the continuation token"}`,
		},
		{
			name:               "invalid first part of continue",
			query:              fmt.Sprintf("/?continue=%s", base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("abc %s", time.Now().Format(time.RFC3339))))),
			expectedLimit:      "",
			expectedInt64:      "",
			expectedDate:       "",
			expectedStatusCode: http.StatusBadRequest,
			expectedBody:       `{"message":"first element of the continue token is not a valid int64"}`,
		},
		{
			name:               "invalid second part of continue",
			query:              fmt.Sprintf("/?continue=%s", base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("1 %s", "2024-11-08")))),
			expectedLimit:      "",
			expectedInt64:      "",
			expectedDate:       "",
			expectedStatusCode: http.StatusBadRequest,
			expectedBody:       `{"message":"second element of the continue token '2024-11-08' does not match '2006-01-02T15:04:05Z07:00'"}`,
		},
		{
			name:               "valid limit and continue",
			query:              fmt.Sprintf("/?limit=250&continue=%s", base64.StdEncoding.EncodeToString([]byte("1 2024-10-22T08:13:52+02:00"))),
			expectedLimit:      "250",
			expectedInt64:      "1",
			expectedDate:       "2024-10-22T08:13:52+02:00",
			expectedStatusCode: http.StatusOK,
			expectedBody:       "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			middleware := Middleware()
			w := httptest.NewRecorder()

			c, _ := gin.CreateTestContext(w)
			// This is used to setup query parameters so `context.Query()` works
			c.Request = httptest.NewRequest(http.MethodGet, tc.query, nil)
			t.Cleanup(func() {
				c.Request.Body.Close()
			})

			middleware(c)

			result := w.Result()
			t.Cleanup(func() {
				result.Body.Close()
			})

			limit, uuid, date := GetValuesFromContext(c)
			assert.Equal(t, tc.expectedLimit, limit)
			assert.Equal(t, tc.expectedInt64, uuid)
			assert.Equal(t, tc.expectedDate, date)
			assert.Equal(t, tc.expectedStatusCode, result.StatusCode)
			assert.Equal(t, tc.expectedBody, w.Body.String())
		})
	}

}
