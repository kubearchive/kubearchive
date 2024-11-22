// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"math"
	"os"
	"slices"
	"strconv"
)

func main() {
	file := flag.String("file", "", "CSV to process")
	metricType := flag.String("type", "", "'cpu' or 'memory', influences the output")
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

	fh, err := os.Open(*file)
	if err != nil {
		panic(fmt.Sprintf("error opening file: %s", err))
	}
	defer fh.Close()

	csvReader := csv.NewReader(fh)
	records, err := csvReader.ReadAll()
	if err != nil {
		panic(fmt.Sprintf("error reading CSV: %s", err))
	}
	header := records[0]
	data := map[string][]float64{}
	for _, record := range records[1:] {
		for ix, valueString := range record {
			if ix == 0 {
				continue // We skip the timestamp column
			}
			value, err := strconv.ParseFloat(valueString, 64)
			if err != nil {
				panic(fmt.Sprintf("error converting '%s' to float64: %s", valueString, err))
			}
			data[header[ix]] = append(data[header[ix]], value)
		}
	}

	fmt.Println("| Component | Min | Mean | Median | Max |")
	fmt.Println("| --- | --- | --- | --- | --- |")
	for key, values := range data {
		minimum := slices.Min(values)
		maximum := slices.Max(values)

		meanSum := 0.0
		for _, val := range values {
			meanSum += val
		}
		length := len(values)
		mean := meanSum / float64(length)

		slices.Sort(values) // This modifies the slice
		ixMiddleValue := length/2 - 1
		median := values[ixMiddleValue]
		// if length is even, use the average of the two "middle" values
		if math.Mod(float64(length)/2.0, 2) == 0 {
			median = (values[ixMiddleValue] + values[ixMiddleValue+1]) / 2.0
		}

		if *metricType == "cpu" {
			fmt.Printf("| %s | %.5f | %.5f | %.5f | %.5f |\n", key, minimum, mean, median, maximum)
		} else if *metricType == "memory" {
			// Transform memory into MB
			fmt.Printf("| %s | %.0f | %.0f | %.0f | %.0f |\n", key, minimum/1e6, mean/1e6, median/1e6, maximum/1e6)
		}
	}
}
