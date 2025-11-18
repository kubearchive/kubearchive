// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
	"k8s.io/client-go/rest"
)

// ClusterConfig represents the configuration for a specific Kubernetes cluster
type ClusterConfig struct {
	ClusterName string `yaml:"-"`
	ServerURL   string `yaml:"server_url"`
	Host        string `yaml:"host"`
	TLSInsecure bool   `yaml:"tls_insecure,omitempty"`
	CertPath    string `yaml:"cert_path,omitempty"`
	Token       string `yaml:"token,omitempty"`
	Current     bool   `yaml:"-"`
}

// DisplaySummary displays the cluster configuration in a consistent format
func (c *ClusterConfig) DisplaySummary() {
	// Display cluster name with current indicator
	if c.Current {
		fmt.Printf("📋 %s * (current)\n", c.ClusterName)
	} else {
		fmt.Printf("📋 %s\n", c.ClusterName)
	}

	fmt.Printf("   Server:  %s\n", c.ServerURL)
	fmt.Printf("   Host:    %s\n", c.Host)

	// Format security info on one line
	var securityInfo []string
	if c.TLSInsecure {
		securityInfo = append(securityInfo, "TLS: Insecure")
	} else {
		securityInfo = append(securityInfo, "TLS: Secure")
	}

	if c.CertPath != "" {
		securityInfo = append(securityInfo, "Cert: Custom")
	}

	if c.Token != "" {
		securityInfo = append(securityInfo, "Token: Stored")
	} else {
		securityInfo = append(securityInfo, "Token: kubectl")
	}

	fmt.Printf("   Config:  %s\n", strings.Join(securityInfo, " • "))
}

// PersistentConfig represents the entire configuration file
type PersistentConfig struct {
	Clusters map[string]*ClusterConfig `yaml:"clusters"`
	current  string
}

// ConfigManager defines the interface for managing persistent configuration operations.
// Configuration is stored per-cluster (identified by the Kubernetes API server URL),
// not per-namespace. This means that switching namespaces within the same cluster
// will use the same KubeArchive host configuration.
type ConfigManager interface {
	LoadConfig(k8sRESTConfig *rest.Config) error
	SaveConfig() error
	GetCurrentClusterConfig() (*ClusterConfig, error)
	GenerateClusterName() (string, error)
	RemoveClusterConfigByName(clusterName string) error
	ListClusters() map[string]*ClusterConfig
	AddCluster(cluster *ClusterConfig)
	GetK8sRESTConfig() *rest.Config
}

// FileConfigManager implements ConfigManager using file-based persistence
type FileConfigManager struct {
	configPath    string
	config        *PersistentConfig
	k8sRESTConfig *rest.Config
}

// NewFileConfigManager creates a new file-based configuration manager
func NewFileConfigManager() *FileConfigManager {
	configPath := os.Getenv("KUBECTL_KA_CONFIG_PATH")
	if configPath == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			configPath = filepath.Join(".", "kubectl-ka.conf")
		} else {
			configPath = filepath.Join(homeDir, ".config", "kubectl-ka.conf")
		}
	}

	return &FileConfigManager{
		configPath: configPath,
		config: &PersistentConfig{
			Clusters: make(map[string]*ClusterConfig),
		},
	}
}

// LoadConfig loads the configuration from disk
func (cm *FileConfigManager) LoadConfig(k8sRESTConfig *rest.Config) error {
	cm.k8sRESTConfig = k8sRESTConfig
	if _, err := os.Stat(cm.configPath); os.IsNotExist(err) {
		// Config file doesn't exist, use empty config
		return nil
	}

	data, err := os.ReadFile(cm.configPath)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	if err := yaml.Unmarshal(data, cm.config); err != nil {
		return fmt.Errorf("failed to parse config file: %w", err)
	}

	for name, cluster := range cm.config.Clusters {
		cluster.ClusterName = name
		if cluster.ServerURL == cm.k8sRESTConfig.Host {
			cm.config.current = name
			cluster.Current = true
		}
	}
	return nil
}

// SaveConfig saves the configuration to disk
func (cm *FileConfigManager) SaveConfig() error {
	// Ensure the directory exists
	dir := filepath.Dir(cm.configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := yaml.Marshal(cm.config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(cm.configPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// GetCurrentClusterConfig retrieves configuration for the current cluster
func (cm *FileConfigManager) GetCurrentClusterConfig() (*ClusterConfig, error) {

	if config, exists := cm.config.Clusters[cm.config.current]; exists {
		return config, nil
	}

	return nil, fmt.Errorf("current cluster config not found")
}

// GenerateClusterName generates a user-friendly key for the current cluster
func (cm *FileConfigManager) GenerateClusterName() (string, error) {
	serverURL := cm.k8sRESTConfig.Host
	if serverURL == "" {
		return "", fmt.Errorf("no server URL found in kubeconfig")
	}

	// Parse URL to extract a meaningful cluster name
	parsedURL, err := url.Parse(serverURL)
	if err != nil {
		return "", fmt.Errorf("invalid server URL: %w", err)
	}

	hostname := parsedURL.Hostname()
	if hostname == "" {
		return "", fmt.Errorf("no hostname found in server URL")
	}

	generatedName := hostname
	// Handle localhost case
	if hostname == "localhost" || hostname == "127.0.0.1" {
		generatedName = "localhost"
	}

	// Split by dots to analyze the hostname structure
	parts := strings.Split(hostname, ".")

	// For OpenShift-style URLs like "api.cluster-name.apps.example.com"
	if len(parts) >= 3 && parts[0] == "api" && parts[len(parts)-2] == "openshiftapps" {
		generatedName = parts[1]
	}

	// Check naming conflicts with already configured clusters
	names := []string{}
	for _, cluster := range cm.config.Clusters {
		names = append(names, cluster.ClusterName)
	}
	for slices.Contains(names, generatedName) {
		generatedName += "-alt"
	}

	return generatedName, nil
}

// RemoveClusterConfigByName removes configuration for a cluster by its name
func (cm *FileConfigManager) RemoveClusterConfigByName(clusterName string) error {
	if clusterName == "" {
		return fmt.Errorf("no cluster name given for removal")
	}

	var exists bool
	if _, exists = cm.config.Clusters[clusterName]; exists {
		delete(cm.config.Clusters, clusterName)
	}
	if !exists {
		return fmt.Errorf("cluster '%s' not found in configuration", clusterName)
	}
	return nil
}

// ListClusters returns all configured clusters
func (cm *FileConfigManager) ListClusters() map[string]*ClusterConfig {
	return cm.config.Clusters
}

func (cm *FileConfigManager) AddCluster(cluster *ClusterConfig) {
	cm.config.Clusters[cluster.ClusterName] = cluster
}

// GetK8sRESTConfig returns the Kubernetes REST config
func (cm *FileConfigManager) GetK8sRESTConfig() *rest.Config {
	return cm.k8sRESTConfig
}
