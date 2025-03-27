// Copyright Kronicler Authors
// SPDX-License-Identifier: Apache-2.0
package pkg

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"slices"
	"strconv"
)

func ExtractStats(file, metricType, outputType string, writer io.Writer) error {
	fh, err := os.Open(file)
	if err != nil {
		return err
	}
	defer fh.Close()

	records := [][]string{}
	csvReader := csv.NewReader(fh)
	for {
		record, err := csvReader.Read()
		if err == io.EOF {
			break
		}

		if err == csv.ErrFieldCount {
			fmt.Fprintln(os.Stderr, "line wrong number of fields")
			continue
		}

		records = append(records, record)
	}

	header := records[0]
	rawData := map[string][]float64{}
	for _, record := range records[1:] {
		for ix, valueString := range record {
			if ix == 0 {
				continue // We skip the timestamp column
			}
			value, err := strconv.ParseFloat(valueString, 64)
			if err != nil {
				return fmt.Errorf("error converting '%s' to float64: %s", valueString, err)
			}
			rawData[header[ix]] = append(rawData[header[ix]], value)
		}
	}

	data := map[string]map[string]float64{}
	for key, values := range rawData {
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
		data[key] = map[string]float64{
			"min":    minimum,
			"mean":   mean,
			"median": median,
			"max":    maximum,
		}
	}

	if outputType == "text" {
		fmt.Fprintln(writer, "| Component | Min | Mean | Median | Max |")
		fmt.Fprintln(writer, "| --- | --- | --- | --- | --- |")
		for key, values := range data {
			if metricType == "cpu" {
				fmt.Fprintf(writer, "| %s | %.5f | %.5f | %.5f | %.5f |\n", key, values["min"], values["mean"], values["median"], values["max"])
			} else if metricType == "memory" {
				// Transform memory into MB
				fmt.Fprintf(writer, "| %s | %.0f | %.0f | %.0f | %.0f |\n", key, values["min"]/1e6, values["mean"]/1e6, values["median"]/1e6, values["max"]/1e6)
			}
		}
	} else if outputType == "json" {
		b, err := json.Marshal(data)
		if err != nil {
			return fmt.Errorf("error converting data to JSON: %s", err.Error())
		}
		fmt.Fprintln(writer, string(b))
	}

	return nil
}
