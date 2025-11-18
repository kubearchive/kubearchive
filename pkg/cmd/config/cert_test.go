// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadCertData(t *testing.T) {
	// Use the test certificate from testdata directory
	testdataCertPath := filepath.Join("testdata", "test-cert.crt")

	tests := []struct {
		name        string
		certPath    string
		expectError bool
		errorMsg    string
	}{
		{
			name:     "valid certificate file from testdata",
			certPath: testdataCertPath,
		},
		{
			name:        "empty cert path",
			certPath:    "",
			expectError: true,
			errorMsg:    "cert path is empty",
		},
		{
			name:        "non-existent file",
			certPath:    "/non/existent/path.crt",
			expectError: true,
			errorMsg:    "failed to read certificate file",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			expandedPath, certData, err := LoadCertData(tt.certPath)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
				assert.Empty(t, expandedPath)
				assert.Nil(t, certData)
			} else {
				require.NoError(t, err)
				assert.NotEmpty(t, expandedPath)
				assert.NotNil(t, certData)
				assert.Contains(t, string(certData), "BEGIN CERTIFICATE")
				assert.Contains(t, string(certData), "END CERTIFICATE")

				assert.Equal(t, testdataCertPath, expandedPath)
			}
		})
	}
}
