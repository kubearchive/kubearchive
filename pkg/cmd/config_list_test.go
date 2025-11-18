// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/kubearchive/kubearchive/pkg/cmd/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/rest"
)

func TestConfigOptions_newListCmd(t *testing.T) {
	opts := NewConfigOptions()
	cmd := opts.newListCmd()

	assert.NotNil(t, cmd)
	assert.Equal(t, "list", cmd.Use)
	assert.Equal(t, "List all configured clusters", cmd.Short)
	assert.True(t, cmd.SilenceUsage)
}

func TestConfigOptions_runList(t *testing.T) {
	tests := []struct {
		name                 string
		clusters             map[string]*config.ClusterConfig
		expectNoClusterMsg   bool
		expectedClusterNames []string
	}{
		{
			name:               "no clusters configured",
			clusters:           map[string]*config.ClusterConfig{},
			expectNoClusterMsg: true,
		},
		{
			name: "single cluster configured",
			clusters: map[string]*config.ClusterConfig{
				"test-cluster": {
					ClusterName: "test-cluster",
					ServerURL:   "https://api.test.com:6443",
					Host:        "https://kubearchive.test.com",
					Current:     true,
				},
			},
			expectNoClusterMsg:   false,
			expectedClusterNames: []string{"test-cluster"},
		},
		{
			name: "multiple clusters configured",
			clusters: map[string]*config.ClusterConfig{
				"cluster1": {
					ClusterName: "cluster1",
					ServerURL:   "https://api.cluster1.com:6443",
					Host:        "https://kubearchive.cluster1.com",
					Current:     false,
				},
				"cluster2": {
					ClusterName: "cluster2",
					ServerURL:   "https://api.cluster2.com:6443",
					Host:        "https://kubearchive.cluster2.com",
					Current:     true,
				},
			},
			expectNoClusterMsg:   false,
			expectedClusterNames: []string{"cluster1", "cluster2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock config manager
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

			// Capture stdout
			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			// Run the list command
			err := opts.runList()
			if err != nil {
				t.Fatal(err)
			}

			// Restore stdout and read captured output
			err = w.Close()
			if err != nil {
				t.Error("error closing stdout:", err)
			}
			os.Stdout = oldStdout

			var buf bytes.Buffer
			_, err = io.Copy(&buf, r)
			if err != nil {
				t.Error("error reading stdout:", err)
			}
			actualOutput := buf.String()

			// Verify no error occurred
			require.NoError(t, err)

			// Check for "No clusters configured" message
			if tt.expectNoClusterMsg {
				assert.Contains(t, actualOutput, "No clusters configured.", "Should show 'No clusters configured' message when no clusters exist")
			} else {
				assert.NotContains(t, actualOutput, "No clusters configured.", "Should not show 'No clusters configured' message when clusters exist")
			}

			// Check that cluster names are present in output
			for _, clusterName := range tt.expectedClusterNames {
				assert.Contains(t, actualOutput, clusterName, "Output should contain cluster name: %s", clusterName)
			}
		})
	}
}

// NewMockKACLICommand creates a mock KACLICommand for testing
func NewMockKACLICommand() KACLICommand {
	return &MockKACLICommand{
		k8sRESTConfig: &rest.Config{
			Host: "https://api.test.com:6443",
		},
	}
}
