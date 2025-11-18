// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"

	"github.com/kubearchive/kubearchive/pkg/cmd/config"
	"github.com/spf13/cobra"
)

// newRemoveCmd creates the remove subcommand
func (o *ConfigOptions) newRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:          "remove [cluster-name]",
		Short:        "Remove configuration for a cluster",
		Long:         "Remove KubeArchive configuration for a specific cluster by name, or the current cluster if no name is provided",
		SilenceUsage: true,
		Args:         cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var clusterName string
			if len(args) > 0 {
				clusterName = args[0]
			}
			return o.runRemove(clusterName)
		},
	}
}

// runRemove executes the remove command
func (o *ConfigOptions) runRemove(clusterName string) error {

	var cluster *config.ClusterConfig
	var err error
	if clusterName == "" {
		cluster, err = o.configManager.GetCurrentClusterConfig()
		if err != nil {
			return err
		}
	} else {
		cluster = o.configManager.ListClusters()[clusterName]
	}
	if cluster == nil {
		return fmt.Errorf("cluster %s not found", clusterName)
	}
	cluster.DisplaySummary()

	confirmed, err := config.PromptForConfirmation(
		fmt.Sprintf("Remove configuration for cluster '%s'?", cluster.ClusterName),
		config.DefaultNo,
	)
	if err != nil {
		return fmt.Errorf("failed to get confirmation: %w", err)
	}

	if !confirmed {
		fmt.Println("Configuration removal cancelled")
		return nil
	}

	// Remove the cluster configuration by name
	err = o.configManager.RemoveClusterConfigByName(cluster.ClusterName)
	if err != nil {
		return fmt.Errorf("failed to remove cluster configuration: %w", err)
	}
	err = o.configManager.SaveConfig()
	if err != nil {
		return fmt.Errorf("failed to save cluster configuration: %w", err)
	}
	fmt.Printf("✓ Configuration for cluster '%s' removed\n", cluster.ClusterName)
	return nil
}
