// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"k8s.io/client-go/rest"
)

// ConnectivityTester defines the interface for testing KubeArchive connectivity.
type ConnectivityTester interface {
	TestKubeArchiveConnectivity(host string, tlsInsecure bool, token string, caData []byte) error
	TestKubeArchiveLivezEndpoint(host string, tlsInsecure bool, caData []byte) error
}

// DefaultConnectivityTester implements ConnectivityTester using the real connectivity functions
type DefaultConnectivityTester struct{}

// TestKubeArchiveConnectivity calls the real connectivity test function
func (d *DefaultConnectivityTester) TestKubeArchiveConnectivity(host string, tlsInsecure bool, token string, caData []byte) error {
	// Create REST config with the provided parameters
	restConfig := &rest.Config{
		Host: host,
		TLSClientConfig: rest.TLSClientConfig{
			Insecure: tlsInsecure,
		},
	}

	if token != "" {
		restConfig.BearerToken = token
	}

	if caData != nil {
		restConfig.CAData = caData
	}

	// Use rest.HTTPClientFor to create client with proper configuration
	client, err := rest.HTTPClientFor(restConfig)
	if err != nil {
		return fmt.Errorf("failed to create HTTP client: %w", err)
	}

	// Test the /api/v1/pods?limit=1 endpoint
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/api/v1/pods?limit=1", host), nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("connection failed: %w", err)
	}
	defer resp.Body.Close()

	// Consider 200 (OK), 404 (Not Found), and 401 (Unauthorized) as success
	switch resp.StatusCode {
	case http.StatusOK, http.StatusNotFound:
		return nil // Success - server is reachable and responding
	case http.StatusUnauthorized:
		body, _ := io.ReadAll(resp.Body)
		if strings.Contains(string(body), "authentication failed") {
			return fmt.Errorf("authentication failed")
		}
		return nil // If the user is authenticated but unauthorized it is OK
	default:
		// Read response body for better error messages
		body, _ := io.ReadAll(resp.Body)
		if len(body) > 0 {
			return fmt.Errorf("server returned status %d: %s", resp.StatusCode, string(body))
		}
		return fmt.Errorf("server returned status %d", resp.StatusCode)
	}
}

// TestKubeArchiveLivezEndpoint calls the real livez endpoint test function
func (d *DefaultConnectivityTester) TestKubeArchiveLivezEndpoint(host string, tlsInsecure bool, caData []byte) error {
	restConfig := &rest.Config{
		Host: host,
		TLSClientConfig: rest.TLSClientConfig{
			Insecure: tlsInsecure,
		},
	}
	if caData != nil {
		restConfig.CAData = caData
	}
	client, err := rest.HTTPClientFor(restConfig)
	if err != nil {
		return fmt.Errorf("failed to create HTTP client: %w", err)
	}

	// Test the /livez endpoint
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/livez", host), nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("connection failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		return nil
	}
	return fmt.Errorf("server returned status %d", resp.StatusCode)
}

// NewDefaultConnectivityTester creates a new default connectivity tester.
// This is the recommended way to create a connectivity tester for production use.
func NewDefaultConnectivityTester() ConnectivityTester {
	return &DefaultConnectivityTester{}
}
