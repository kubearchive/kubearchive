// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"

	"github.com/kubearchive/kubearchive/pkg/cmd/config"
	"github.com/spf13/cobra"
)

// ConfigOptions holds the options for the config command
type ConfigOptions struct {
	KACLICommand
	configManager config.ConfigManager
}

// NewConfigOptions creates new config options
func NewConfigOptions() *ConfigOptions {
	return &ConfigOptions{
		KACLICommand:  NewKARetrieverOptionsNoEnv(),
		configManager: config.NewFileConfigManager(),
	}
}

// NewConfigCmd creates the config command
func NewConfigCmd() *cobra.Command {
	o := NewConfigOptions()

	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage KubeArchive CLI configuration",
		Long: `Manage KubeArchive CLI configuration for different Kubernetes clusters.

Configuration is stored per-cluster (not per-namespace), so switching namespaces
within the same cluster will use the same KubeArchive host configuration.

The config command allows you to:
- List configured clusters
- Set the KubeArchive host for a cluster
- Remove cluster configuration`,
		SilenceUsage:  true,
		SilenceErrors: true,
		// This method runs before Run in all subcommands
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return o.Complete()
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	// Add flags - only Kubernetes flags for config commands
	o.AddK8sFlags(cmd.PersistentFlags())

	// Add subcommands
	cmd.AddCommand(o.newListCmd())
	cmd.AddCommand(o.newSetCmd())
	cmd.AddCommand(o.newUnsetCmd())
	cmd.AddCommand(o.newRemoveCmd())
	cmd.AddCommand(o.newSetupCmd())

	return cmd
}

func (o *ConfigOptions) Complete() error {
	err := o.CompleteK8sConfig()
	if err != nil {
		return err
	}
	err = o.configManager.LoadConfig(o.GetK8sRESTConfig())
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}
	return nil
}
