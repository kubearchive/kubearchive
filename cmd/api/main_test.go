// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"github.com/stretchr/testify/assert"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRouterInvalidMethods(t *testing.T) {
	router := setupRouter()

	tests := []struct {
		name   string
		method string
	}{
		{
			name:   "Invalid POST",
			method: "POST",
		},
		{
			name:   "Invalid PUT",
			method: "PUT",
		},
		{
			name:   "Invalid DELETE",
			method: "DELETE",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res := httptest.NewRecorder()
			req, _ := http.NewRequest(tc.method, "/apis/stable.example.com/v1/crontabs", nil)
			router.ServeHTTP(res, req)

			assert.Equal(t, http.StatusNotFound, res.Code)
		})
	}
}
