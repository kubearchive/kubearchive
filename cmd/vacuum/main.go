// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	typePtr := flag.String("type", "namespace", "Type of vacuum, either 'namespace' or 'cluster'.")
	configPtr := flag.String("config", "", "Name of configuration file.")

	flag.Parse()

	var err error
	if *typePtr == "cluster" {
		err = clusterVacuum(*configPtr)
	} else {
		err = namespaceVacuum(*configPtr)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	os.Exit(0)
}
