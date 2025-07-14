// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0
package config

import (
	"os"
	"reflect"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/rest"
)

// Test helpers
// -------------

// withEnv sets an environment variable for the duration of the test.
func withEnv(t *testing.T, key, value string, testFunc func()) {
	t.Helper()
	original := os.Getenv(key)
	require.NoError(t, os.Setenv(key, value))
	t.Cleanup(func() { os.Setenv(key, original) })
	testFunc()
}

func TestNewArchiveOptionsDefaults(t *testing.T) {
	opts := NewArchiveOptions()

	// Test default values
	assert.Equal(t, "https://localhost:8081", opts.Host)
	assert.Equal(t, "", opts.Token)
	assert.Equal(t, false, opts.TLSInsecure)
	assert.Equal(t, "", opts.CertificatePath)
}

func TestNewArchiveOptionsEnvOverrides(t *testing.T) {
	testCases := []struct {
		name          string
		envVar        string
		envValue      string
		expectedValue interface{}
		fieldName     string
	}{
		{
			name:          "Host override",
			envVar:        "KUBECTL_PLUGIN_ARCHIVE_HOST",
			envValue:      "https://example.com:8082",
			expectedValue: "https://example.com:8082",
			fieldName:     "Host",
		},
		{
			name:          "Certificate path override",
			envVar:        "KUBECTL_PLUGIN_ARCHIVE_CERT_PATH",
			envValue:      "/path/to/cert.crt",
			expectedValue: "/path/to/cert.crt",
			fieldName:     "CertificatePath",
		},
		{
			name:          "TLS insecure true",
			envVar:        "KUBECTL_PLUGIN_ARCHIVE_TLS_INSECURE",
			envValue:      "true",
			expectedValue: true,
			fieldName:     "TLSInsecure",
		},
		{
			name:          "TLS insecure false",
			envVar:        "KUBECTL_PLUGIN_ARCHIVE_TLS_INSECURE",
			envValue:      "false",
			expectedValue: false,
			fieldName:     "TLSInsecure",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Setup environment variable
			withEnv(t, tc.envVar, tc.envValue, func() {
				opts := NewArchiveOptions()

				// Use reflection to get the field value dynamically
				val := reflect.ValueOf(opts).Elem()
				field := val.FieldByName(tc.fieldName)
				assert.True(t, field.IsValid(), "Field %s not found", tc.fieldName)

				var actualValue interface{}
				if field.Kind() == reflect.Bool {
					actualValue = field.Bool()
				} else {
					actualValue = field.Interface()
				}
				assert.Equal(t, tc.expectedValue, actualValue)
			})
		})
	}
}

func TestGetCertificateData(t *testing.T) {
	testCases := []struct {
		name          string
		opts          *ArchiveOptions
		expectedError bool
		expectedCert  []byte
		setupTempFile bool
	}{
		{
			name: "No certificate path set",
			opts: &ArchiveOptions{
				CertificatePath: "",
			},
			expectedError: false,
			expectedCert:  nil,
		},
		{
			name: "Certificate path set, file exists",
			opts: &ArchiveOptions{
				CertificatePath: "/tmp/test-ca.crt",
			},
			expectedError: false,
			expectedCert:  []byte("-----BEGIN CERTIFICATE-----\nTEST CERT DATA\n-----END CERTIFICATE-----"),
			setupTempFile: true,
		},
		{
			name: "Certificate path set, file doesn't exist",
			opts: &ArchiveOptions{
				CertificatePath: "/nonexistent/ca.crt",
			},
			expectedError: true,
			expectedCert:  nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Initialize KubeFlags if not already set
			if tc.opts.KubeFlags == nil {
				tc.opts.KubeFlags = genericclioptions.NewConfigFlags(true)
			}

			// Setup temporary file if needed
			if tc.setupTempFile {
				tmpFile, tmpErr := os.CreateTemp("", "test-ca.crt")
				require.NoError(t, tmpErr)
				t.Cleanup(func() {
					os.Remove(tmpFile.Name())
				})

				err := os.WriteFile(tmpFile.Name(), tc.expectedCert, 0600)
				require.NoError(t, err)

				// Update the certificate path to use the temporary file
				tc.opts.CertificatePath = tmpFile.Name()
			}

			certData, err := tc.opts.GetCertificateData()

			if tc.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tc.expectedCert, certData)
		})
	}
}

func TestAddArchiveFlags(t *testing.T) {
	flags := pflag.NewFlagSet("test", pflag.ExitOnError)
	opts := &ArchiveOptions{
		Host:            "https://test.com",
		CertificatePath: "/test/cert.crt",
		TLSInsecure:     true,
		KubeFlags:       genericclioptions.NewConfigFlags(true),
	}

	opts.AddFlags(flags)

	// Check that all expected flags are present
	expectedFlags := []string{
		"host",
		"kubearchive-insecure-skip-tls-verify",
		"kubearchive-certificate-authority",
	}

	for _, flagName := range expectedFlags {
		flag := flags.Lookup(flagName)
		assert.NotNil(t, flag, "Flag %s should be present", flagName)
	}
}

func TestComplete(t *testing.T) {
	restConfig := &rest.Config{
		BearerToken: "kubeconfig-token",
	}

	// Test constants
	testKubectlTokenValue := "test-kubectl-token" // #nosec G101 - test token, not a real password

	testCases := []struct {
		name          string
		setup         func(opts *ArchiveOptions)
		env           map[string]string
		expectedToken string
		expectError   bool
		errorContains string
	}{
		{
			name: "Token precedence - kubectl --token flag",
			setup: func(opts *ArchiveOptions) {
				opts.KubeFlags.BearerToken = &testKubectlTokenValue
			},
			env:           map[string]string{},
			expectedToken: "test-kubectl-token",
			expectError:   false,
		},
		{
			name:  "Token precedence - environment variable",
			setup: func(opts *ArchiveOptions) {},
			env: map[string]string{
				"KUBECTL_PLUGIN_ARCHIVE_TOKEN": "env-token",
			},
			expectedToken: "env-token",
			expectError:   false,
		},
		{
			name:          "Token precedence - kubeconfig token",
			setup:         func(opts *ArchiveOptions) {},
			env:           map[string]string{},
			expectedToken: "kubeconfig-token",
			expectError:   false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Set up environment variables
			originalEnv := make(map[string]string)
			for k, v := range tc.env {
				originalEnv[k] = os.Getenv(k)
				os.Setenv(k, v)
			}
			t.Cleanup(func() {
				for k, v := range originalEnv {
					if v == "" {
						os.Unsetenv(k)
					} else {
						os.Setenv(k, v)
					}
				}
			})

			opts := &ArchiveOptions{
				KubeFlags: genericclioptions.NewConfigFlags(true),
			}
			if tc.setup != nil {
				tc.setup(opts)
			}

			err := opts.Complete(restConfig)
			if tc.expectError {
				assert.Error(t, err)
				if tc.errorContains != "" {
					assert.Contains(t, err.Error(), tc.errorContains)
				}
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tc.expectedToken, opts.Token)
		})
	}
}
