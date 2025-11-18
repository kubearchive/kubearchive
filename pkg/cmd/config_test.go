// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"
	"testing"

	"github.com/kubearchive/kubearchive/pkg/cmd/config"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"k8s.io/client-go/rest"
)

func TestNewConfigOptions(t *testing.T) {
	opts := NewConfigOptions()

	assert.NotNil(t, opts)
	assert.NotNil(t, opts.KACLICommand)
	assert.NotNil(t, opts.configManager)
}

func TestNewConfigCmd(t *testing.T) {
	cmd := NewConfigCmd()

	assert.NotNil(t, cmd)
	assert.Equal(t, "config", cmd.Use)
	assert.Equal(t, "Manage KubeArchive CLI configuration", cmd.Short)
	assert.NotEmpty(t, cmd.Long)
	assert.True(t, cmd.SilenceUsage)
	assert.True(t, cmd.SilenceErrors)

	// Check that subcommands are added
	subcommands := cmd.Commands()
	expectedSubcommands := []string{"list", "set", "unset", "remove", "setup"}

	assert.Len(t, subcommands, len(expectedSubcommands))

	actualSubcommands := make([]string, len(subcommands))
	for i, subcmd := range subcommands {
		actualSubcommands[i] = subcmd.Name() // Use Name() instead of Use to get just the command name
	}

	for _, expected := range expectedSubcommands {
		assert.Contains(t, actualSubcommands, expected)
	}
}

func TestConfigOptions_Complete(t *testing.T) {
	tests := []struct {
		name        string
		setupMock   func() *MockKACLICommand
		expectError bool
		errorMsg    string
	}{
		{
			name: "successful completion",
			setupMock: func() *MockKACLICommand {
				mock := &MockKACLICommand{}
				mock.k8sRESTConfig = &rest.Config{
					Host: "https://api.test.com:6443",
				}
				return mock
			},
			expectError: false,
		},
		{
			name: "k8s config error",
			setupMock: func() *MockKACLICommand {
				mock := &MockKACLICommand{}
				mock.completeK8sConfigError = assert.AnError
				return mock
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := tt.setupMock()
			opts := &ConfigOptions{
				KACLICommand:  mock,
				configManager: NewTestConfigManager(t),
			}

			err := opts.Complete()

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// MockKACLICommand implements the KACLICommand interface for testing
type MockKACLICommand struct {
	k8sRESTConfig          *rest.Config
	completeK8sConfigError error
	namespaceValue         string
	namespaceError         error
}

func (m *MockKACLICommand) CompleteK8sConfig() error {
	return m.completeK8sConfigError
}

func (m *MockKACLICommand) GetK8sRESTConfig() *rest.Config {
	if m.k8sRESTConfig != nil {
		return m.k8sRESTConfig
	}
	return &rest.Config{
		Host: "https://api.test.com:6443",
	}
}

func (m *MockKACLICommand) AddK8sFlags(_ *pflag.FlagSet) {}

func (m *MockKACLICommand) GetNamespace() (string, error) {
	if m.namespaceError != nil {
		return "", m.namespaceError
	}
	if m.namespaceValue != "" {
		return m.namespaceValue, nil
	}
	return "default", nil
}

// MockConfigManager is a mock implementation of ConfigManager for testing
type MockConfigManager struct {
	clusters      map[string]*config.ClusterConfig
	k8sRESTConfig *rest.Config
	saveError     error
	loadError     error
}

func NewMockConfigManager() *MockConfigManager {
	return &MockConfigManager{
		clusters: make(map[string]*config.ClusterConfig),
		k8sRESTConfig: &rest.Config{
			Host: "https://api.test.com:6443",
		},
	}
}

func (m *MockConfigManager) LoadConfig(k8sRESTConfig *rest.Config) error {
	if m.loadError != nil {
		return m.loadError
	}
	m.k8sRESTConfig = k8sRESTConfig
	return nil
}

func (m *MockConfigManager) SaveConfig() error {
	return m.saveError
}

func (m *MockConfigManager) GetCurrentClusterConfig() (*config.ClusterConfig, error) {
	for _, cluster := range m.clusters {
		if cluster.Current {
			return cluster, nil
		}
	}
	return nil, fmt.Errorf("current cluster config not found")
}

func (m *MockConfigManager) GenerateClusterName() (string, error) {
	return "test-cluster", nil
}

func (m *MockConfigManager) RemoveClusterConfigByName(clusterName string) error {
	delete(m.clusters, clusterName)
	return nil
}

func (m *MockConfigManager) ListClusters() map[string]*config.ClusterConfig {
	return m.clusters
}

func (m *MockConfigManager) AddCluster(cluster *config.ClusterConfig) {
	m.clusters[cluster.ClusterName] = cluster
}

func (m *MockConfigManager) GetK8sRESTConfig() *rest.Config {
	return m.k8sRESTConfig
}

// NewTestConfigManager creates a ConfigManager for testing
func NewTestConfigManager(t *testing.T) config.ConfigManager {
	t.Helper()
	return NewMockConfigManager()
}
