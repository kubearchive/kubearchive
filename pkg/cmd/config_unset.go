// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// completeUnsetArgs provides shell completion for the unset command arguments
func completeUnsetArgs(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) == 0 {
		// CompleteK8sConfig configuration keys that can be unset
		return []string{
			"ca\tClear certificate authority file path",
			"token\tClear bearer token for authentication",
		}, cobra.ShellCompDirectiveNoFileComp
	}
	return nil, cobra.ShellCompDirectiveNoFileComp
}

// newUnsetCmd creates the unset subcommand
func (o *ConfigOptions) newUnsetCmd() *cobra.Command {
	return &cobra.Command{
		Use:               "unset <key>",
		Short:             "Unset a configuration option",
		Long:              "Clear/reset a configuration option for the current cluster",
		SilenceUsage:      true,
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeUnsetArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return o.runUnset(args[0])
		},
	}
}

// runUnset executes the unset command
func (o *ConfigOptions) runUnset(key string) error {
	clusterConfig, err := o.configManager.GetCurrentClusterConfig()
	if err != nil {
		return fmt.Errorf("failed to get current cluster configuration: %w", err)
	}

	// Handle different configuration keys
	switch key {
	case "ca", "certificate-authority":
		clusterConfig.CertPath = ""
		fmt.Println("✓ Certificate authority cleared")
	case "token":
		clusterConfig.Token = ""
		fmt.Println("✓ Token cleared")
	case "host":
		return fmt.Errorf("cannot unset host - use 'kubectl ka config remove' to remove the entire cluster configuration")
	default:
		return fmt.Errorf("unknown configuration key: %s", key)
	}

	// Save the configuration
	if err := o.configManager.SaveConfig(); err != nil {
		return fmt.Errorf("failed to save configuration: %w", err)
	}

	return nil
}
