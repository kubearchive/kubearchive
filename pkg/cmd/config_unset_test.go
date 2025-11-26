// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"testing"

	"github.com/kubearchive/kubearchive/pkg/cmd/config"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func TestCompleteUnsetArgs(t *testing.T) {
	// Test basic completion functionality
	result, directive := completeUnsetArgs(nil, []string{}, "")
	assert.NotEmpty(t, result)
	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
	assert.Contains(t, result, "ca\tClear certificate authority file path")
	assert.Contains(t, result, "token\tClear bearer token for authentication")

	// Test with one argument
	result, directive = completeUnsetArgs(nil, []string{"ca"}, "")
	assert.Nil(t, result)
	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
}

func TestConfigOptions_newUnsetCmd(t *testing.T) {
	opts := NewConfigOptions()
	cmd := opts.newUnsetCmd()

	assert.NotNil(t, cmd)
	assert.Equal(t, "unset <key>", cmd.Use)
	assert.Equal(t, "Unset a configuration option", cmd.Short)
	assert.True(t, cmd.SilenceUsage)
	assert.NotNil(t, cmd.ValidArgsFunction)

	// Test argument validation (ExactArgs(1))
	assert.NotNil(t, cmd.Args(cmd, []string{}))           // 0 args should be rejected
	assert.Nil(t, cmd.Args(cmd, []string{"key"}))         // 1 arg should be allowed
	assert.NotNil(t, cmd.Args(cmd, []string{"k1", "k2"})) // 2 args should be rejected
}

func TestConfigOptions_runUnset(t *testing.T) {
	tests := []struct {
		name           string
		key            string
		clusterConfig  *config.ClusterConfig
		expectError    bool
		errorContains  string
		validateResult func(t *testing.T, clusterConfig *config.ClusterConfig)
	}{
		{
			name: "unset ca certificate authority",
			key:  "ca",
			clusterConfig: &config.ClusterConfig{
				ClusterName: "test-cluster",
				Host:        "https://kubearchive.example.com",
				CertPath:    "/path/to/cert.pem",
				Token:       "test-token",
				Current:     true,
			},
			expectError: false,
			validateResult: func(t *testing.T, clusterConfig *config.ClusterConfig) {
				assert.Empty(t, clusterConfig.CertPath, "CertPath should be cleared")
				assert.Equal(t, "test-token", clusterConfig.Token, "Token should remain unchanged")
			},
		},
		{
			name: "unset certificate-authority (full name)",
			key:  "certificate-authority",
			clusterConfig: &config.ClusterConfig{
				ClusterName: "test-cluster",
				Host:        "https://kubearchive.example.com",
				CertPath:    "/path/to/cert.pem",
				Token:       "test-token",
				Current:     true,
			},
			expectError: false,
			validateResult: func(t *testing.T, clusterConfig *config.ClusterConfig) {
				assert.Empty(t, clusterConfig.CertPath, "CertPath should be cleared")
			},
		},
		{
			name: "unset token",
			key:  "token",
			clusterConfig: &config.ClusterConfig{
				ClusterName: "test-cluster",
				Host:        "https://kubearchive.example.com",
				CertPath:    "/path/to/cert.pem",
				Token:       "test-token",
				Current:     true,
			},
			expectError: false,
			validateResult: func(t *testing.T, clusterConfig *config.ClusterConfig) {
				assert.Empty(t, clusterConfig.Token, "Token should be cleared")
				assert.Equal(t, "/path/to/cert.pem", clusterConfig.CertPath, "CertPath should remain unchanged")
			},
		},
		{
			name: "unset host (should fail)",
			key:  "host",
			clusterConfig: &config.ClusterConfig{
				ClusterName: "test-cluster",
				Host:        "https://kubearchive.example.com",
				Current:     true,
			},
			expectError:   true,
			errorContains: "cannot unset host - use 'kubectl ka config remove' to remove the entire cluster configuration",
		},
		{
			name: "unset unknown key",
			key:  "unknown",
			clusterConfig: &config.ClusterConfig{
				ClusterName: "test-cluster",
				Host:        "https://kubearchive.example.com",
				Current:     true,
			},
			expectError:   true,
			errorContains: "unknown configuration key: unknown",
		},
		{
			name:          "no current cluster configured",
			key:           "token",
			clusterConfig: nil, // No current cluster
			expectError:   true,
			errorContains: "failed to get current cluster configuration",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Use setupSetOptionsTest helper and extract ConfigOptions
			setOpts := setupSetOptionsTest(t, &MockConnectivityTester{}, tt.clusterConfig)
			opts := setOpts.ConfigOptions

			err := opts.runUnset(tt.key)

			assertTestError(t, err, tt.expectError, tt.errorContains)

			if !tt.expectError && tt.validateResult != nil && tt.clusterConfig != nil {
				tt.validateResult(t, tt.clusterConfig)
			}
		})
	}
}
