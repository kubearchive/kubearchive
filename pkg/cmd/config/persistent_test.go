// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/rest"
)

func TestClusterConfig_DisplaySummary(t *testing.T) {
	tests := []struct {
		name       string
		config     *ClusterConfig
		goldenFile string
	}{
		{
			name: "current cluster with all fields",
			config: &ClusterConfig{
				ClusterName: "test-cluster",
				ServerURL:   "https://api.test.com:6443",
				Host:        "https://kubearchive.test.com",
				TLSInsecure: false,
				CertPath:    "/path/to/cert.pem",
				Token:       "test-token",
				Current:     true,
			},
			goldenFile: "display_summary_current_cluster_all_fields.txt",
		},
		{
			name: "non-current cluster with insecure TLS",
			config: &ClusterConfig{
				ClusterName: "insecure-cluster",
				ServerURL:   "https://api.insecure.com:6443",
				Host:        "http://kubearchive.insecure.com",
				TLSInsecure: true,
				Current:     false,
			},
			goldenFile: "display_summary_insecure_cluster.txt",
		},
		{
			name: "minimal configuration",
			config: &ClusterConfig{
				ClusterName: "minimal-cluster",
				ServerURL:   "https://api.minimal.com:6443",
				Host:        "https://kubearchive.minimal.com",
			},
			goldenFile: "display_summary_minimal_cluster.txt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Capture stdout
			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			// Call DisplaySummary
			tt.config.DisplaySummary()

			// Restore stdout and read captured output
			err := w.Close()
			if err != nil {
				t.Error("Unable to check stodout response")
			}
			os.Stdout = oldStdout

			var buf bytes.Buffer
			_, err = io.Copy(&buf, r)
			if err != nil {
				t.Error("Unable to read stodout response")
			}
			actualOutput := strings.TrimSpace(buf.String())

			// Read expected output from golden file
			goldenPath := filepath.Join("testdata", tt.goldenFile)
			expectedBytes, err := os.ReadFile(goldenPath)
			require.NoError(t, err, "Failed to read golden file: %s", goldenPath)
			expectedOutput := strings.TrimSpace(string(expectedBytes))

			// Compare outputs
			assert.Equal(t, expectedOutput, actualOutput, "Output doesn't match golden file: %s", tt.goldenFile)
		})
	}
}

func TestNewConfigManager(t *testing.T) {
	// Test with default config path
	fcm := NewFileConfigManager()

	assert.NotEmpty(t, fcm.configPath)
	assert.NotNil(t, fcm.config)
	assert.NotNil(t, fcm.config.Clusters)

	// Test with custom config path
	customPath := "/tmp/test-kubectl-ka.conf"
	os.Setenv("KUBECTL_KA_CONFIG_PATH", customPath)
	defer os.Unsetenv("KUBECTL_KA_CONFIG_PATH")

	fcm2 := NewFileConfigManager()
	assert.Equal(t, customPath, fcm2.configPath)
}

func TestConfigManager_LoadConfig(t *testing.T) {
	// Use test config file from testdata
	configPath := filepath.Join("testdata", "test-config.yaml")

	cm := &FileConfigManager{
		configPath: configPath,
		config: &PersistentConfig{
			Clusters: make(map[string]*ClusterConfig),
		},
	}

	k8sConfig := &rest.Config{
		Host: "https://api.test.com:6443",
	}

	err := cm.LoadConfig(k8sConfig)
	require.NoError(t, err)

	// Verify config was loaded correctly
	assert.Len(t, cm.config.Clusters, 1)
	cluster, exists := cm.config.Clusters["test-cluster"]
	require.True(t, exists)
	assert.Equal(t, "test-cluster", cluster.ClusterName)
	assert.Equal(t, "https://api.test.com:6443", cluster.ServerURL)
	assert.Equal(t, "https://kubearchive.test.com", cluster.Host)
	assert.False(t, cluster.TLSInsecure)
	assert.Equal(t, "/path/to/cert.pem", cluster.CertPath)
	assert.Equal(t, "test-token", cluster.Token)
	assert.True(t, cluster.Current) // Should be marked as current since server URLs match
}

func TestConfigManager_SaveConfig(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "test-config.yaml")

	cm := &FileConfigManager{
		configPath: configPath,
		config: &PersistentConfig{
			Clusters: map[string]*ClusterConfig{
				"test-cluster": {
					ClusterName: "test-cluster",
					ServerURL:   "https://api.test.com:6443",
					Host:        "https://kubearchive.test.com",
					TLSInsecure: false,
					CertPath:    "/path/to/cert.pem",
					Token:       "test-token",
					Current:     true,
				},
			},
		},
	}

	err := cm.SaveConfig()
	require.NoError(t, err)

	// Verify file was created
	assert.FileExists(t, configPath)

	// Read the saved file content
	actualContent, err := os.ReadFile(configPath)
	require.NoError(t, err)

	// Read expected content from testdata
	expectedPath := filepath.Join("testdata", "test-config.yaml")
	expectedContent, err := os.ReadFile(expectedPath)
	require.NoError(t, err)

	// Compare the contents, ignoring license headers
	actualYAML := stripLicenseHeader(string(actualContent))
	expectedYAML := stripLicenseHeader(string(expectedContent))
	assert.Equal(t, expectedYAML, actualYAML, "Saved config doesn't match expected testdata file")
}

func TestConfigManager_GetCurrentClusterConfig(t *testing.T) {
	tests := []struct {
		name            string
		clusters        map[string]*ClusterConfig
		currentCluster  string
		expectedCluster string
		expectError     bool
		errorContains   string
	}{
		{
			name: "successfully get current cluster",
			clusters: map[string]*ClusterConfig{
				"test-cluster": {
					ClusterName: "test-cluster",
					ServerURL:   "https://api.test.com:6443",
					Host:        "https://kubearchive.test.com",
					Current:     true,
				},
				"other-cluster": {
					ClusterName: "other-cluster",
					ServerURL:   "https://api.other.com:6443",
					Host:        "https://kubearchive.other.com",
					Current:     false,
				},
			},
			currentCluster:  "test-cluster",
			expectedCluster: "test-cluster",
			expectError:     false,
		},
		{
			name: "no current cluster set",
			clusters: map[string]*ClusterConfig{
				"test-cluster": {
					ClusterName: "test-cluster",
					ServerURL:   "https://api.test.com:6443",
					Host:        "https://kubearchive.test.com",
					Current:     false,
				},
			},
			currentCluster: "",
			expectError:    true,
			errorContains:  "current cluster config not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cm := &FileConfigManager{
				config: &PersistentConfig{
					Clusters: tt.clusters,
					current:  tt.currentCluster,
				},
			}

			cluster, err := cm.GetCurrentClusterConfig()

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
				assert.Nil(t, cluster)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, cluster)
				assert.Equal(t, tt.expectedCluster, cluster.ClusterName)
				assert.True(t, cluster.Current)
			}
		})
	}
}

func TestConfigManager_GenerateClusterName(t *testing.T) {
	tests := []struct {
		name        string
		serverURL   string
		existing    []string
		expected    string
		expectError bool
	}{
		{
			name:      "localhost",
			serverURL: "https://localhost:6443",
			expected:  "localhost",
		},
		{
			name:      "127.0.0.1",
			serverURL: "https://127.0.0.1:6443",
			expected:  "localhost",
		},
		{
			name:      "openshift style",
			serverURL: "https://api.cluster-name.apps.example.openshiftapps.com:6443",
			expected:  "cluster-name",
		},
		{
			name:      "regular hostname",
			serverURL: "https://k8s.example.com:6443",
			expected:  "k8s.example.com",
		},
		{
			name:        "empty server URL",
			serverURL:   "",
			expectError: true,
		},
		{
			name:        "invalid URL",
			serverURL:   "not-a-url",
			expectError: true,
		},
		{
			name:      "conflict resolution",
			serverURL: "https://localhost:6443",
			existing:  []string{"localhost", "localhost-alt"},
			expected:  "localhost-alt-alt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cm := &FileConfigManager{
				k8sRESTConfig: &rest.Config{
					Host: tt.serverURL,
				},
				config: &PersistentConfig{
					Clusters: make(map[string]*ClusterConfig),
				},
			}

			// Add existing clusters to test conflict resolution
			for _, existing := range tt.existing {
				cm.config.Clusters[existing] = &ClusterConfig{
					ClusterName: existing,
				}
			}

			result, err := cm.GenerateClusterName()

			if tt.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestConfigManager_RemoveClusterConfigByName(t *testing.T) {
	tests := []struct {
		name                 string
		initialClusters      map[string]*ClusterConfig
		clusterToRemove      string
		expectedClustersLeft int
		expectError          bool
		errorContains        string
		shouldNotExist       string
	}{
		{
			name: "successfully remove existing cluster",
			initialClusters: map[string]*ClusterConfig{
				"test-cluster": {
					ClusterName: "test-cluster",
				},
				"other-cluster": {
					ClusterName: "other-cluster",
				},
			},
			clusterToRemove:      "test-cluster",
			expectedClustersLeft: 1,
			expectError:          false,
			shouldNotExist:       "test-cluster",
		},
		{
			name: "remove non-existent cluster",
			initialClusters: map[string]*ClusterConfig{
				"test-cluster": {
					ClusterName: "test-cluster",
				},
			},
			clusterToRemove: "non-existent",
			expectError:     true,
			errorContains:   "cluster 'non-existent' not found",
		},
		{
			name: "empty cluster name",
			initialClusters: map[string]*ClusterConfig{
				"test-cluster": {
					ClusterName: "test-cluster",
				},
			},
			clusterToRemove: "",
			expectError:     true,
			errorContains:   "no cluster name given",
		},
		{
			name: "remove from single cluster config",
			initialClusters: map[string]*ClusterConfig{
				"only-cluster": {
					ClusterName: "only-cluster",
				},
			},
			clusterToRemove:      "only-cluster",
			expectedClustersLeft: 0,
			expectError:          false,
			shouldNotExist:       "only-cluster",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cm := &FileConfigManager{
				config: &PersistentConfig{
					Clusters: make(map[string]*ClusterConfig),
				},
			}

			// Copy initial clusters to avoid test interference
			for name, cluster := range tt.initialClusters {
				cm.config.Clusters[name] = cluster
			}

			err := cm.RemoveClusterConfigByName(tt.clusterToRemove)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				require.NoError(t, err)
				assert.Len(t, cm.config.Clusters, tt.expectedClustersLeft)

				if tt.shouldNotExist != "" {
					_, exists := cm.config.Clusters[tt.shouldNotExist]
					assert.False(t, exists, "Cluster %s should have been removed", tt.shouldNotExist)
				}
			}
		})
	}
}

func TestConfigManager_ListClusters(t *testing.T) {
	clusters := map[string]*ClusterConfig{
		"cluster1": {ClusterName: "cluster1"},
		"cluster2": {ClusterName: "cluster2"},
	}

	cm := &FileConfigManager{
		config: &PersistentConfig{
			Clusters: clusters,
		},
	}

	result := cm.ListClusters()
	assert.Equal(t, clusters, result)
}

func TestConfigManager_AddCluster(t *testing.T) {
	cm := &FileConfigManager{
		config: &PersistentConfig{
			Clusters: make(map[string]*ClusterConfig),
		},
	}

	cluster := &ClusterConfig{
		ClusterName: "new-cluster",
		ServerURL:   "https://api.new.com:6443",
	}

	cm.AddCluster(cluster)

	assert.Len(t, cm.config.Clusters, 1)
	stored, exists := cm.config.Clusters["new-cluster"]
	require.True(t, exists)
	assert.Equal(t, cluster, stored)
}

// stripLicenseHeader removes license header comments from YAML content for comparison
func stripLicenseHeader(content string) string {
	lines := strings.Split(content, "\n")
	var yamlLines []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		// Skip license header lines (comments starting with #) and empty lines before YAML content
		if strings.HasPrefix(trimmed, "#") || (trimmed == "" && len(yamlLines) == 0) {
			continue
		}
		// Skip YAML document separator if it's at the beginning
		if trimmed == "---" && len(yamlLines) == 0 {
			continue
		}
		yamlLines = append(yamlLines, line)
	}

	return strings.TrimSpace(strings.Join(yamlLines, "\n"))
}
