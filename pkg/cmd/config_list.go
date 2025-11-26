// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// newListCmd creates the list subcommand
func (o *ConfigOptions) newListCmd() *cobra.Command {
	return &cobra.Command{
		Use:          "list",
		Short:        "List all configured clusters",
		Long:         "List KubeArchive configuration for the different Kubernetes clusters",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return o.runList()
		},
	}
}

// runList executes the list command
func (o *ConfigOptions) runList() error {

	clusters := o.configManager.ListClusters()
	if len(clusters) == 0 {
		fmt.Println("No clusters configured.")
		fmt.Println()
		fmt.Println("💡 Tip: Run 'kubectl-ka config setup' to automatically configure the current cluster")
		return nil
	}

	for _, clusterConfig := range clusters {
		clusterConfig.DisplaySummary()
		fmt.Println()
	}

	return nil
}
