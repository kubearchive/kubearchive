// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"testing"

	"github.com/kubearchive/kubearchive/pkg/cmd/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigOptions_newRemoveCmd(t *testing.T) {
	opts := NewConfigOptions()
	cmd := opts.newRemoveCmd()

	assert.NotNil(t, cmd)
	assert.Equal(t, "remove [cluster-name]", cmd.Use)
	assert.Equal(t, "Remove configuration for a cluster", cmd.Short)
	assert.NotEmpty(t, cmd.Long)
	assert.True(t, cmd.SilenceUsage)

	// Test argument validation (MaximumNArgs(1))
	assert.Nil(t, cmd.Args(cmd, []string{}))              // 0 args should be allowed
	assert.Nil(t, cmd.Args(cmd, []string{"cluster1"}))    // 1 arg should be allowed
	assert.NotNil(t, cmd.Args(cmd, []string{"c1", "c2"})) // 2 args should be rejected
}

func TestConfigOptions_runRemove_ErrorCases(t *testing.T) {
	tests := []struct {
		name          string
		clusterName   string
		clusters      map[string]*config.ClusterConfig
		errorContains string
	}{
		{
			name:        "remove current cluster when no current cluster exists",
			clusterName: "", // Empty means remove current cluster
			clusters: map[string]*config.ClusterConfig{
				"test-cluster": {
					ClusterName: "test-cluster",
					ServerURL:   "https://api.test.com:6443",
					Host:        "https://kubearchive.test.com",
					Current:     false,
				},
			},
			errorContains: "current cluster config not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock config manager using shared implementation
			mockConfigManager := NewMockConfigManager()

			// Add test clusters to the mock
			for _, cluster := range tt.clusters {
				mockConfigManager.AddCluster(cluster)
			}

			// Create ConfigOptions with the mock config manager
			opts := &ConfigOptions{
				KACLICommand:  NewMockKACLICommand(),
				configManager: mockConfigManager,
			}

			// Run the remove command (this will fail at the confirmation prompt for valid clusters)
			err := opts.runRemove(tt.clusterName)

			// Check error expectations
			require.Error(t, err)
			if tt.errorContains != "" {
				assert.Contains(t, err.Error(), tt.errorContains)
			}
		})
	}
}

func TestConfigOptions_runRemove_ClusterLookup(t *testing.T) {
	tests := []struct {
		name         string
		clusterName  string
		clusters     map[string]*config.ClusterConfig
		expectFound  bool
		expectedName string
	}{
		{
			name:        "find cluster by name",
			clusterName: "test-cluster",
			clusters: map[string]*config.ClusterConfig{
				"test-cluster": {
					ClusterName: "test-cluster",
					ServerURL:   "https://api.test.com:6443",
					Host:        "https://kubearchive.test.com",
					Current:     true,
				},
			},
			expectFound:  true,
			expectedName: "test-cluster",
		},
		{
			name:        "find current cluster when no name provided",
			clusterName: "", // Empty means current cluster
			clusters: map[string]*config.ClusterConfig{
				"current-cluster": {
					ClusterName: "current-cluster",
					ServerURL:   "https://api.current.com:6443",
					Host:        "https://kubearchive.current.com",
					Current:     true,
				},
			},
			expectFound:  true,
			expectedName: "current-cluster",
		},
		{
			name:        "cluster not found by name",
			clusterName: "missing-cluster",
			clusters: map[string]*config.ClusterConfig{
				"test-cluster": {
					ClusterName: "test-cluster",
					ServerURL:   "https://api.test.com:6443",
					Host:        "https://kubearchive.test.com",
					Current:     true,
				},
			},
			expectFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockConfigManager := NewMockConfigManager()

			for _, cluster := range tt.clusters {
				mockConfigManager.AddCluster(cluster)
			}

			var cluster *config.ClusterConfig
			var err error

			if tt.clusterName == "" {
				cluster, err = mockConfigManager.GetCurrentClusterConfig()
			} else {
				clusters := mockConfigManager.ListClusters()
				cluster = clusters[tt.clusterName]
			}

			if tt.expectFound {
				if tt.clusterName == "" {
					require.NoError(t, err, "Should find current cluster")
				}
				require.NotNil(t, cluster, "Should find cluster")
				assert.Equal(t, tt.expectedName, cluster.ClusterName, "Should find correct cluster")
			} else {
				if tt.clusterName == "" {
					assert.Error(t, err, "Should not find current cluster")
				} else {
					assert.Nil(t, cluster, "Should not find cluster")
				}
			}
		})
	}
}
