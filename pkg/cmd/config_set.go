// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/kubearchive/kubearchive/pkg/cmd/config"
	"github.com/spf13/cobra"
)

// SetOptions holds the options specific to the set subcommand
type SetOptions struct {
	*ConfigOptions
	key    string
	value  string
	tester config.ConnectivityTester
}

// completeSetArgs provides shell completion for the set command arguments
func completeSetArgs(_ *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
	if len(args) == 0 {
		// CompleteK8sConfig configuration keys
		return []string{
			"host\tKubeArchive API server URL",
			"ca\tPath to certificate authority file",
			"insecure\tBoolean (true/false) to skip TLS verification",
			"token\tBearer token for authentication",
		}, cobra.ShellCompDirectiveNoFileComp
	} else if len(args) == 1 {
		// CompleteK8sConfig values based on the key
		switch args[0] {
		case "ca", "certificate-authority":
			// Enable file completion for certificate paths
			return nil, cobra.ShellCompDirectiveDefault
		case "insecure", "insecure-skip-tls-verify":
			return []string{"true", "false"}, cobra.ShellCompDirectiveNoFileComp
		case "host":
			return []string{"https://"}, cobra.ShellCompDirectiveNoSpace | cobra.ShellCompDirectiveNoFileComp
		default:
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
	}
	return nil, cobra.ShellCompDirectiveNoFileComp
}

// newSetCmd creates the set subcommand
func (o *ConfigOptions) newSetCmd() *cobra.Command {
	tester := &config.DefaultConnectivityTester{}
	return &cobra.Command{
		Use:               "set <key> [value]",
		Short:             "Set a configuration option",
		Long:              "Set a configuration option for the current cluster",
		SilenceUsage:      true,
		Args:              cobra.MinimumNArgs(1),
		ValidArgsFunction: completeSetArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			setOpts := &SetOptions{ConfigOptions: o, tester: tester}
			if err := setOpts.completeSet(args); err != nil {
				return err
			}
			return setOpts.runSet()
		},
	}
}

// completeSet validates and processes the arguments for the set command
func (o *SetOptions) completeSet(args []string) error {
	// Handle argument parsing manually for better error messages
	if len(args) < 1 {
		return fmt.Errorf("missing configuration key")
	}
	if len(args) > 2 {
		return fmt.Errorf("too many arguments")
	}

	o.key = args[0]

	if len(args) == 2 {
		o.value = args[1]
	} else {
		// Handle special case for insecure - default to true if no value provided
		if o.key == "insecure" || o.key == "insecure-skip-tls-verify" {
			o.value = "true"
		} else {
			return fmt.Errorf("missing value for key '%s'", o.key)
		}
	}

	return nil
}

// runSet executes the set command for individual configuration options
func (o *SetOptions) runSet() error {
	clusterConfig, err := o.configManager.GetCurrentClusterConfig()

	if err != nil && o.key != "host" {
		return fmt.Errorf("cannot set configuration '%s' to a cluster that isn't configured: %w", o.key, err)
	}
	// If no configuration exists for this cluster, create a new one
	if clusterConfig == nil {
		fmt.Println("Current cluster not found, generating a new one")
		clusterKey, err := o.configManager.GenerateClusterName()
		if err != nil {
			return fmt.Errorf("failed to generate a cluster name: %w", err)
		}

		clusterConfig = &config.ClusterConfig{
			ClusterName: clusterKey,
			ServerURL:   o.GetK8sRESTConfig().Host,
			Current:     true,
		}

		o.configManager.AddCluster(clusterConfig)
	}

	// Handle different configuration keys
	switch o.key {
	case "host":
		return o.setHost(clusterConfig, o.value)
	case "ca", "certificate-authority":
		return o.setCertificateAuthority(clusterConfig, o.value)
	case "insecure", "insecure-skip-tls-verify":
		return o.setInsecure(clusterConfig, o.value)
	case "token":
		return o.setToken(clusterConfig, o.value)
	default:
		return fmt.Errorf("unknown configuration key: %s", o.key)
	}
}

// setHost sets the KubeArchive host URL
func (o *SetOptions) setHost(clusterConfig *config.ClusterConfig, hostURL string) error {
	// Validate URL format
	parsedURL, err := url.Parse(hostURL)
	if err != nil {
		return fmt.Errorf("invalid URL format: %w", err)
	}

	if parsedURL.Scheme == "" {
		return fmt.Errorf("URL must include scheme (http:// or https://)")
	}

	if parsedURL.Host == "" {
		return fmt.Errorf("URL must include host")
	}

	// Test connection before saving
	if err := o.tester.TestKubeArchiveLivezEndpoint(hostURL, true, nil); err != nil {
		return fmt.Errorf("cannot connect to %s: %w", hostURL, err)
	}

	// Save the configuration
	clusterConfig.Host = hostURL
	if err := o.configManager.SaveConfig(); err != nil {
		return fmt.Errorf("failed to save configuration: %w", err)
	}

	fmt.Printf("✓ Host set to %s\n", hostURL)
	return nil
}

// setCertificateAuthority sets the certificate authority file path
func (o *SetOptions) setCertificateAuthority(clusterConfig *config.ClusterConfig, certPath string) error {
	if certPath == "" {
		return fmt.Errorf("certificate path cannot be empty")
	}

	// Load certificate data for testing
	expandedCertPath, certData, err := config.LoadCertData(certPath)
	if err != nil {
		return fmt.Errorf("failed to read certificate file for testing: %w", err)
	}

	if err := o.tester.TestKubeArchiveLivezEndpoint(clusterConfig.Host, false, certData); err != nil {
		return fmt.Errorf("cannot connect using certificate %s: %w", expandedCertPath, err)
	}

	// Save the configuration
	clusterConfig.CertPath = expandedCertPath
	clusterConfig.TLSInsecure = false // Setting a certificate implies secure TLS
	if err := o.configManager.SaveConfig(); err != nil {
		return fmt.Errorf("failed to save configuration: %w", err)
	}

	fmt.Printf("✓ Certificate authority set to %s (TLS verification enabled)\n", expandedCertPath)
	return nil
}

// setInsecure sets the TLS insecure flag
func (o *SetOptions) setInsecure(clusterConfig *config.ClusterConfig, value string) error {
	lowerValue := strings.ToLower(value)
	var insecure bool

	switch lowerValue {
	case "true", "yes", "1":
		insecure = true
	case "false", "no", "0":
		insecure = false
	default:
		return fmt.Errorf("invalid boolean value for insecure: %s (use true/false)", value)
	}

	// Load certificate data if available
	var certData []byte
	if insecure {
		if clusterConfig.CertPath != "" {
			var err error
			certData, err = os.ReadFile(clusterConfig.CertPath)
			if err != nil {
				return fmt.Errorf("failed to read certificate file for testing: %w", err)
			}
		}
	}

	if err := o.tester.TestKubeArchiveLivezEndpoint(clusterConfig.Host, insecure, certData); err != nil {
		if insecure {
			return fmt.Errorf("cannot connect with TLS verification disabled: %w", err)
		} else {
			return fmt.Errorf("cannot connect with TLS verification enabled: %w (try setting a certificate authority with 'kubectl ka config set ca /path/to/ca.crt' or use 'kubectl ka config set insecure true')", err)
		}
	}

	// Save the configuration
	clusterConfig.TLSInsecure = insecure
	if err := o.configManager.SaveConfig(); err != nil {
		return fmt.Errorf("failed to save configuration: %w", err)
	}

	if insecure {
		fmt.Println("✓ TLS verification disabled (insecure)")
	} else {
		fmt.Println("✓ TLS verification enabled (secure)")
	}
	return nil
}

// setToken sets the bearer token
func (o *SetOptions) setToken(clusterConfig *config.ClusterConfig, token string) error {
	if token == "" {
		return fmt.Errorf("token cannot be empty")
	}

	// Validate JWT format (should have 3 parts separated by dots)
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return fmt.Errorf("invalid JWT format: token must have 3 parts separated by dots")
	}

	// Test token against KubeArchive API if host is configured
	if clusterConfig.Host != "" {
		if err := o.tester.TestKubeArchiveConnectivity(clusterConfig.Host, true, token, nil); err != nil {
			return fmt.Errorf("token validation failed: %w", err)
		}
	}

	// Save the configuration
	clusterConfig.Token = token
	if err := o.configManager.SaveConfig(); err != nil {
		return fmt.Errorf("failed to save configuration: %w", err)
	}

	fmt.Println("✓ Token set successfully")
	return nil
}
