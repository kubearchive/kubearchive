// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/client-go/kubernetes/fake"
)

func TestNewServer(t *testing.T) {
	server := NewServer(fake.NewSimpleClientset())
	res := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/", nil)
	server.router.ServeHTTP(res, req)

	assert.Equal(t, http.StatusUnauthorized, res.Code)
}

func TestAuthenticationMiddlewareIsConfigured(t *testing.T) {
	tests := []struct {
		name     string
		method   string
		expected int
	}{
		{
			name:     "Invalid POST",
			method:   "POST",
			expected: http.StatusUnauthorized,
		},
		{
			name:     "Invalid PUT",
			method:   "PUT",
			expected: http.StatusUnauthorized,
		},
		{
			name:     "Invalid DELETE",
			method:   "DELETE",
			expected: http.StatusUnauthorized,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := NewServer(fake.NewSimpleClientset())
			res := httptest.NewRecorder()
			req, _ := http.NewRequest(tc.method, "/apis/stable.example.com/v1/crontabs", nil)
			server.router.ServeHTTP(res, req)

			b, err := httputil.DumpResponse(res.Result(), true)
			if err != nil {
				t.Error(err)
				t.FailNow()
			}

			assert.Equal(t, tc.expected, res.Code, string(b))
		})
	}
}
