// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"github.com/kubearchive/kubearchive/pkg/cmd/config"
	"github.com/spf13/cobra"
)

// newSetupCmd creates the setup subcommand
func (o *ConfigOptions) newSetupCmd() *cobra.Command {
	return &cobra.Command{
		Use:          "setup",
		Short:        "Interactive setup for KubeArchive configuration",
		Long:         "Interactively configure KubeArchive for the current Kubernetes cluster",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return o.runSetup()
		},
	}
}

// runSetup executes the setup command
func (o *ConfigOptions) runSetup() error {

	tester := &config.DefaultConnectivityTester{}
	ns, err := o.GetNamespace()
	if err != nil {
		ns = "default"
	}
	interactiveSetup := config.NewInteractiveSetup(o.configManager, ns, tester)

	return interactiveSetup.RunSetup()
}
