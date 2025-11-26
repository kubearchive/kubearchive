// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/kubearchive/kubearchive/pkg/cmd/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetOptions_completeSet(t *testing.T) {
	tests := []struct {
		name          string
		args          []string
		expectedKey   string
		expectedValue string
		expectError   bool
		errorContains string
	}{
		{
			name:          "missing key",
			args:          []string{},
			expectError:   true,
			errorContains: "missing configuration key",
		},
		{
			name:          "too many arguments",
			args:          []string{"host", "value", "extra"},
			expectError:   true,
			errorContains: "too many arguments",
		},
		{
			name:          "key with value",
			args:          []string{"host", "https://example.com"},
			expectedKey:   "host",
			expectedValue: "https://example.com",
			expectError:   false,
		},
		{
			name:          "insecure without value",
			args:          []string{"insecure"},
			expectedKey:   "insecure",
			expectedValue: "true",
			expectError:   false,
		},
		{
			name:          "insecure-skip-tls-verify without value",
			args:          []string{"insecure-skip-tls-verify"},
			expectedKey:   "insecure-skip-tls-verify",
			expectedValue: "true",
			expectError:   false,
		},
		{
			name:          "other key without value",
			args:          []string{"token"},
			expectError:   true,
			errorContains: "missing value for key 'token'",
		},
		{
			name:          "ca key without value",
			args:          []string{"ca"},
			expectError:   true,
			errorContains: "missing value for key 'ca'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configOpts := NewConfigOptions()
			setOpts := &SetOptions{ConfigOptions: configOpts}

			err := setOpts.completeSet(tt.args)

			if tt.expectError {
				require.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedKey, setOpts.key)
				assert.Equal(t, tt.expectedValue, setOpts.value)
			}
		})
	}
}

func TestConfigOptions_newSetCmd(t *testing.T) {
	opts := NewConfigOptions()
	cmd := opts.newSetCmd()

	assert.NotNil(t, cmd)
	assert.Equal(t, "set <key> [value]", cmd.Use)
	assert.Equal(t, "Set a configuration option", cmd.Short)
	assert.NotEmpty(t, cmd.Long)
	assert.True(t, cmd.SilenceUsage)

	// Test argument validation (MinimumNArgs(1))
	assert.NotNil(t, cmd.Args(cmd, []string{}))            // 0 args should be rejected
	assert.Nil(t, cmd.Args(cmd, []string{"key"}))          // 1 arg should be allowed
	assert.Nil(t, cmd.Args(cmd, []string{"key", "value"})) // 2 args should be allowed
}

func TestSetOptions_runSet_ErrorCases(t *testing.T) {
	tests := []struct {
		name          string
		key           string
		value         string
		errorContains string
	}{
		{
			name:          "set token without current cluster",
			key:           "token",
			value:         "test.token.value",
			errorContains: "cannot set configuration 'token' to a cluster that isn't configured",
		},
		{
			name:          "set ca without current cluster",
			key:           "ca",
			value:         "/tmp/ca.crt",
			errorContains: "cannot set configuration 'ca' to a cluster that isn't configured",
		},
		{
			name:          "set insecure without current cluster",
			key:           "insecure",
			value:         "true",
			errorContains: "cannot set configuration 'insecure' to a cluster that isn't configured",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockConnectivityTester := &MockConnectivityTester{}
			setOpts := setupSetOptionsTest(t, mockConnectivityTester, nil) // No cluster for error cases

			setOpts.key = tt.key
			setOpts.value = tt.value

			err := setOpts.runSet()

			assertTestError(t, err, true, tt.errorContains)
		})
	}
}

func TestSetOptions_setHost(t *testing.T) {
	tests := []struct {
		name                string
		hostURL             string
		connectivitySuccess bool
		connectivityError   error
		expectError         bool
		errorContains       string
	}{
		{
			name:                "valid URL with successful connectivity",
			hostURL:             "https://kubearchive.example.com",
			connectivitySuccess: true,
			expectError:         false,
		},
		{
			name:          "invalid URL format",
			hostURL:       "not-a-url",
			expectError:   true,
			errorContains: "URL must include scheme",
		},
		{
			name:              "connectivity test fails",
			hostURL:           "https://kubearchive.example.com",
			connectivityError: fmt.Errorf("connection refused"),
			expectError:       true,
			errorContains:     "cannot connect to https://kubearchive.example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockConnectivityTester := &MockConnectivityTester{
				livezShouldFail: !tt.connectivitySuccess,
				livezFailError:  tt.connectivityError,
			}

			clusterConfig := &config.ClusterConfig{
				ClusterName: "test-cluster",
				Current:     true,
			}
			setOpts := setupSetOptionsTest(t, mockConnectivityTester, clusterConfig)

			err := setOpts.setHost(clusterConfig, tt.hostURL)

			assertTestError(t, err, tt.expectError, tt.errorContains)
			if !tt.expectError {
				assert.Equal(t, tt.hostURL, clusterConfig.Host)
			}
		})
	}
}

func TestSetOptions_setCertificateAuthority(t *testing.T) {
	// Use the existing certificate file from testdata
	certPath := filepath.Join("config", "testdata", "test-cert.crt")

	tests := []struct {
		name                string
		certPath            string
		connectivitySuccess bool
		connectivityError   error
		expectError         bool
		errorContains       string
	}{
		{
			name:                "valid certificate with successful connectivity",
			certPath:            certPath,
			connectivitySuccess: true,
			expectError:         false,
		},
		{
			name:          "empty certificate path",
			certPath:      "",
			expectError:   true,
			errorContains: "certificate path cannot be empty",
		},
		{
			name:          "non-existent certificate file",
			certPath:      "/non/existent/path.pem",
			expectError:   true,
			errorContains: "failed to read certificate file for testing",
		},
		{
			name:              "connectivity test fails with certificate",
			certPath:          certPath,
			connectivityError: fmt.Errorf("certificate validation failed"),
			expectError:       true,
			errorContains:     "cannot connect using certificate",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockConnectivityTester := &MockConnectivityTester{
				livezShouldFail: !tt.connectivitySuccess,
				livezFailError:  tt.connectivityError,
			}

			clusterConfig := &config.ClusterConfig{
				ClusterName: "test-cluster",
				Host:        "https://kubearchive.example.com",
				Current:     true,
			}
			setOpts := setupSetOptionsTest(t, mockConnectivityTester, clusterConfig)

			err := setOpts.setCertificateAuthority(clusterConfig, tt.certPath)

			assertTestError(t, err, tt.expectError, tt.errorContains)
			if !tt.expectError {
				assert.NotEmpty(t, clusterConfig.CertPath)
				assert.False(t, clusterConfig.TLSInsecure) // Should be set to false when cert is set
			}
		})
	}
}

func TestSetOptions_setInsecure(t *testing.T) {
	tests := []struct {
		name          string
		value         string
		expectError   bool
		errorContains string
		expectedValue bool
	}{
		{
			name:          "set insecure to true",
			value:         "true",
			expectError:   false,
			expectedValue: true,
		},
		{
			name:          "set insecure to false",
			value:         "false",
			expectError:   false,
			expectedValue: false,
		},
		{
			name:          "invalid boolean value",
			value:         "maybe",
			expectError:   true,
			errorContains: "invalid boolean value",
		},
		{
			name:          "empty value",
			value:         "",
			expectError:   true,
			errorContains: "invalid boolean value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockConnectivityTester := &MockConnectivityTester{}
			clusterConfig := &config.ClusterConfig{
				ClusterName: "test-cluster",
				Current:     true,
			}
			setOpts := setupSetOptionsTest(t, mockConnectivityTester, clusterConfig)

			err := setOpts.setInsecure(clusterConfig, tt.value)

			assertTestError(t, err, tt.expectError, tt.errorContains)
			if !tt.expectError {
				assert.Equal(t, tt.expectedValue, clusterConfig.TLSInsecure)
			}
		})
	}
}

func TestSetOptions_setToken(t *testing.T) {
	tests := []struct {
		name                string
		token               string
		clusterHost         string
		connectivitySuccess bool
		connectivityError   error
		expectError         bool
		errorContains       string
	}{
		{
			name:                "valid token with successful connectivity",
			token:               "a.valid.token",
			clusterHost:         "https://kubearchive.example.com",
			connectivitySuccess: true,
			expectError:         false,
		},
		{
			name:          "empty token",
			token:         "",
			expectError:   true,
			errorContains: "token cannot be empty",
		},
		{
			name:          "invalid JWT format - too few parts",
			token:         "invalid.token",
			expectError:   true,
			errorContains: "invalid JWT format: token must have 3 parts separated by dots",
		},
		{
			name:          "invalid JWT format - too many parts",
			token:         "part1.part2.part3.part4",
			expectError:   true,
			errorContains: "invalid JWT format: token must have 3 parts separated by dots",
		},
		{
			name:              "connectivity test fails with token",
			token:             "valid.unauth.token",
			clusterHost:       "https://kubearchive.example.com",
			connectivityError: fmt.Errorf("authentication failed"),
			expectError:       true,
			errorContains:     "token validation failed",
		},
		{
			name:        "valid token without host configured",
			token:       "a.valid.token",
			clusterHost: "",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockConnectivityTester := &MockConnectivityTester{
				shouldFail: !tt.connectivitySuccess,
				failError:  tt.connectivityError,
			}

			clusterConfig := &config.ClusterConfig{
				ClusterName: "test-cluster",
				Host:        tt.clusterHost,
				Current:     true,
			}
			setOpts := setupSetOptionsTest(t, mockConnectivityTester, clusterConfig)

			err := setOpts.setToken(clusterConfig, tt.token)

			assertTestError(t, err, tt.expectError, tt.errorContains)
			if !tt.expectError {
				assert.Equal(t, tt.token, clusterConfig.Token)
			}
		})
	}
}

// setupSetOptionsTest creates a SetOptions with mocks for testing
func setupSetOptionsTest(t *testing.T, connectivityTester *MockConnectivityTester, clusterConfig *config.ClusterConfig) *SetOptions {
	t.Helper()
	mockConfigManager := NewMockConfigManager()

	if clusterConfig != nil {
		mockConfigManager.AddCluster(clusterConfig)
	}

	setOpts := &SetOptions{
		ConfigOptions: &ConfigOptions{
			KACLICommand:  NewMockKACLICommand(),
			configManager: mockConfigManager,
		},
		tester: connectivityTester,
	}

	return setOpts
}

// assertTestError checks error expectations in a standardized way
func assertTestError(t *testing.T, err error, expectError bool, errorContains string) {
	t.Helper()
	if expectError {
		require.Error(t, err)
		if errorContains != "" {
			assert.Contains(t, err.Error(), errorContains)
		}
	} else {
		require.NoError(t, err)
	}
}
