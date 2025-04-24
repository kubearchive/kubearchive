// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"flag"
)

func main() {
	typePtr := flag.String("type", "namespace", "Type of vacuum, either 'namespace' or 'cluster'.")
	configPtr := flag.String("config", "", "Name of configuration file.")

	flag.Parse()

	if *typePtr == "cluster" {
		clusterVacuum(*configPtr)
	} else {
		namespaceVacuum(*configPtr)
	}
}
