// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0
package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// loadGoldenFile loads content from a golden file in testdata
func loadGoldenFile(t *testing.T, filename string) string {
	t.Helper()
	path := filepath.Join("testdata", filename)
	content, err := os.ReadFile(path)
	require.NoError(t, err, "Failed to read golden file: %s", path)
	return string(content)
}

// MockConnectivityTester implements config.ConnectivityTester for testing
type MockConnectivityTester struct {
	shouldFail      bool
	failError       error
	livezShouldFail bool
	livezFailError  error
}

// TestKubeArchiveConnectivity mocks the connectivity test
func (m *MockConnectivityTester) TestKubeArchiveConnectivity(host string, tlsInsecure bool, token string, caData []byte) error {
	if m.shouldFail {
		if m.failError != nil {
			return m.failError
		}
		return fmt.Errorf("mock connectivity test failed")
	}
	return nil
}

// TestKubeArchiveLivezEndpoint mocks the livez endpoint test
func (m *MockConnectivityTester) TestKubeArchiveLivezEndpoint(host string, tlsInsecure bool, caData []byte) error {
	if m.livezShouldFail {
		if m.livezFailError != nil {
			return m.livezFailError
		}
		return fmt.Errorf("mock livez test failed")
	}
	return nil
}
