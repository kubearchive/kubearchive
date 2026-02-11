// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/avast/retry-go/v5"
)

type PromResult struct {
	Metric struct{ Job string }
	Values [][]any
}

type PromData struct {
	Result []PromResult
}

type PrometheusResponse struct {
	Status string
	Data   PromData
}

var metrics = []struct {
	Name  string
	Query string
	Step  string
}{
	{
		Name:  "memory",
		Query: `sum by (job) (go_memory_used_bytes{job=~"kubearchive.*"})`,
		Step:  "15s",
	},
	{
		Name:  "cpu",
		Query: `sum by (job) (irate(process_cpu_time_seconds_total{job=~"kubearchive.*"}[5m]))`,
		Step:  "15s",
	},
}

func main() {
	start := flag.String("start", "", "a string")
	end := flag.String("end", "", "a string")
	prefix := flag.String("prefix", "./perf-results/", "a string")
	flag.Parse()

	if *start == "" {
		fmt.Println("flag '--start=' is required")
		os.Exit(1)
	}

	if *end == "" {
		fmt.Println("flag '--end=' is required")
		os.Exit(1)
	}

	client := http.Client{}
	for _, metric := range metrics {
		req, err := http.NewRequest(http.MethodGet, "http://localhost:9090/api/v1/query_range", nil)
		if err != nil {
			panic(err)
		}

		q := req.URL.Query()
		q.Add("query", metric.Query)
		q.Add("step", metric.Step)
		q.Add("start", *start)
		q.Add("end", *end)
		req.URL.RawQuery = q.Encode()

		var responseBytes []byte
		retryErr := retry.Do(
			func() error {
				response, errDo := client.Do(req)
				if errDo != nil {
					return fmt.Errorf("error doing the request: %s", err)
				}
				defer response.Body.Close()

				var errRead error
				responseBytes, errRead = io.ReadAll(response.Body)
				if errRead != nil {
					return fmt.Errorf("error reading all: %s", err)
				}

				if response.StatusCode != http.StatusOK {
					fmt.Println(string(responseBytes))
					return errors.New("status code not OK")
				}

				return nil
			}, retry.OnRetry(func(n uint, err error) {
				fmt.Printf("Retry #%d: %s\n", n, err)
			}))

		if retryErr != nil {
			panic(fmt.Sprintf("error calling Prometheus: %s", err))
		}

		var responseData PrometheusResponse
		err = json.Unmarshal(responseBytes, &responseData)
		if err != nil {
			fmt.Println(string(responseBytes))
			panic(fmt.Sprintf("error unmarshaling: %s", err))
		}

		if responseData.Status != "success" {
			panic("status not 'success'!")
		}

		if len(responseData.Data.Result) == 0 {
			panic("no results returned!")
		}

		data := map[string][]string{}
		csvHeader := []string{"timestamp"}
		for _, result := range responseData.Data.Result {
			resultJob := result.Metric.Job
			csvHeader = append(csvHeader, resultJob)
			for _, value := range result.Values {
				timestamp := fmt.Sprintf("%.0f", value[0].(float64))
				val := value[1].(string)

				data[timestamp] = append(data[timestamp], val)
			}
		}

		var b bytes.Buffer
		buf := bufio.NewWriter(&b)
		csvWriter := csv.NewWriter(buf)
		err = csvWriter.Write(csvHeader)
		if err != nil {
			panic(fmt.Sprintf("error writing to CSV: %s", err))
		}
		for key, values := range data {
			err = csvWriter.Write(append([]string{key}, values...))
			if err != nil {
				panic(fmt.Sprintf("error writing to CSV: %s", err))
			}
		}
		csvWriter.Flush()

		filePath := fmt.Sprintf("%s%s.csv", *prefix, metric.Name)
		err = os.WriteFile(filePath, b.Bytes(), 0600)
		if err != nil {
			panic(fmt.Sprintf("error writing to file: %s", err))
		}
	}

}
