// Copyright Kronicler Authors
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"fmt"
	"os"

	"github.com/kronicler/kronicler/pkg/cmd"
	"github.com/spf13/cobra"
)

func main() {
	rootCmd := NewRootCmd()
	rootCmd.AddCommand(cmd.NewGetCmd())
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func NewRootCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "kubectl-archive",
		Short: "kubectl plugin to interact with Kronicler",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("Hello from the root cmd")
			return nil
		},
	}
}
