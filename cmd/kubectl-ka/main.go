// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"fmt"
	"os"

	"github.com/kubearchive/kubearchive/pkg/cmd"
	"github.com/spf13/cobra"
)

var (
	// These variables are set during build time via ldflags
	version   = "dev"
	buildDate = "unknown"
	gitCommit = "unknown"
)

func main() {
	rootCmd := NewRootCmd()
	rootCmd.AddCommand(cmd.NewGetCmd())
	rootCmd.AddCommand(cmd.NewLogCmd())
	rootCmd.AddCommand(NewVersionCmd())
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func NewRootCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "kubectl-ka",
		Short: "kubectl plugin to interact with KubeArchive",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("Hello from the root cmd")
			return nil
		},
	}
}

func NewVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("kubectl-archive version %s\n", version)
			fmt.Printf("Build date: %s\n", buildDate)
			fmt.Printf("Git commit: %s\n", gitCommit)
		},
	}
}
