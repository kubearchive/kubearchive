// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"fmt"
	"os"

	"github.com/kubearchive/kubearchive/pkg/cmd"
	"github.com/spf13/cobra"
)

func main() {
	rootCmd := NewRootCmd()
	rootCmd.AddCommand(cmd.NewGetCmd())
	rootCmd.AddCommand(cmd.NewLogCmd())
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func NewRootCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "kubectl-archive",
		Short: "kubectl plugin to interact with KubeArchive",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("Hello from the root cmd")
			return nil
		},
	}
}
