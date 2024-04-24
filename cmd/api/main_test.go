// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetAllResources(t *testing.T) {
	router := setupRouter()

	res := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/apis/stable.example.com/v1/crontabs", nil)
	router.ServeHTTP(res, req)

	assert.Equal(t, 200, res.Code)
	resources := resources{}
	if err := json.NewDecoder(res.Body).Decode(&resources); err != nil {
		t.Fail()
	}
	assert.Equal(t, resources.Kind, "crontab")
	assert.Equal(t, resources.APIVersion, "stable.example.com/v1")
	assert.Greater(t, len(resources.Items), 0)
}

func TestInvalidMethods(t *testing.T) {
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

			assert.Equal(t, 404, res.Code)
		})
	}
}
