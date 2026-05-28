// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConnectivityUsesNamespacedEndpoint(t *testing.T) {
	var requestedPath string
	var requestedQuery string
	var authHeader string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedPath = r.URL.Path
		requestedQuery = r.URL.RawQuery
		authHeader = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	tester := &DefaultConnectivityTester{}
	err := tester.TestKubeArchiveConnectivity(server.URL, true, "test-namespace", "fake-token", nil)
	require.NoError(t, err)
	assert.Equal(t, "/api/v1/namespaces/test-namespace/pods", requestedPath)
	assert.Equal(t, "limit=1", requestedQuery)
	assert.Equal(t, "Bearer fake-token", authHeader)
}

func TestConnectivityDefaultsToDefaultNamespace(t *testing.T) {
	var requestedPath string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	tester := &DefaultConnectivityTester{}
	err := tester.TestKubeArchiveConnectivity(server.URL, true, "", "fake-token", nil)
	require.NoError(t, err)
	assert.Equal(t, "/api/v1/namespaces/default/pods", requestedPath)
}
