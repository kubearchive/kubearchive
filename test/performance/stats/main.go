// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"

	"github.com/kubearchive/kubearchive/test/performance/pkg"
)

func main() {
	file := flag.String("file", "", "CSV to process")
	metricType := flag.String("type", "", "'cpu' or 'memory', influences the output")
	outputType := flag.String("output", "text", "'text' or 'json', influences the output type")
	flag.Parse()

	if *file == "" {
		fmt.Println("'--file' parameter is required")
		os.Exit(1)
	}

	if *metricType == "" {
		fmt.Println("'--type' parameter is required")
		os.Exit(1)
	} else {
		if *metricType != "cpu" && *metricType != "memory" {
			fmt.Printf("'--type' '%s' is not valid, should be 'cpu' or 'memory'\n", *metricType)
			os.Exit(1)
		}
	}

	if *outputType != "text" && *outputType != "json" {
		fmt.Printf("'--output' '%s' is not valid, should be 'text' or 'json'\n", *outputType)
		os.Exit(1)
	}

	var b bytes.Buffer
	pkg.ExtractStats(*file, *metricType, *outputType, &b)
	fmt.Println(b.String())
}
