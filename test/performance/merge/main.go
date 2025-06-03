// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/kubearchive/kubearchive/test/performance/pkg"
)

type Artifact struct {
	ArchiveDownloadURL string `json:"archive_download_url"`
}

type Artifacts struct {
	Artifacts []*Artifact `json:"artifacts"`
	Count     int         `json:"total_count"`
}

type WorkflowRun struct {
	Id           int64  `json:"id"`
	ArtifactsURL string `json:"artifacts_url"`
	HeadBranch   string `json:"head_branch"`
	HeadSha      string `json:"head_sha"`
	Event        string `json:"event"`
	CreatedAt    string `json:"created_at"`
}

type WorkflowRuns struct {
	WorkflowRuns []*WorkflowRun `json:"workflow_runs"`
}

type Stats struct {
	CreatedAt string
	HeadSha   string
	Id        int64
	Api       Stat `json:"kubearchive.api"`
	Sink      Stat `json:"kubearchive.sink"`
	Operator  Stat `json:"kubearchive.operator"`
}

type Stat struct {
	Min               float64
	Max               float64
	Median            float64
	Mean              float64
	RequestsPerSecond float64
}

const GITHUB_WORKFLOW_RUNS_ENDPOINT = "https://api.github.com/repos/kubearchive/kubearchive/actions/workflows/performance.yml/runs"
const DATA_DIR = "./merge"

func readOrPullWorkflowRuns() WorkflowRuns {
	client := http.Client{}
	var wr WorkflowRuns

	fh, openErr := os.Open(filepath.Join(DATA_DIR, "workflowruns.json"))
	if errors.Is(openErr, os.ErrNotExist) {
		req, err := http.NewRequest(http.MethodGet, GITHUB_WORKFLOW_RUNS_ENDPOINT, nil)
		if err != nil {
			panic(fmt.Sprintf("error creating the request: %s", err.Error()))
		}

		res, err := client.Do(req)
		if err != nil {
			panic(fmt.Sprintf("error doing the request: %s", err.Error()))
		}
		defer res.Body.Close()

		bodyBytes, err := io.ReadAll(res.Body)
		if err != nil {
			panic(fmt.Sprintf("error reading the body: %s", err.Error()))
		}
		err = json.Unmarshal(bodyBytes, &wr)
		if err != nil {
			panic(fmt.Sprintf("error deserializing the body: %s", err.Error()))
		}

		fh, err = os.Create(filepath.Join(DATA_DIR, "workflowruns.json"))
		if err != nil {
			panic(fmt.Sprintf("error creating workflowruns.json: %s", err.Error()))
		}

		_, writeErr := fh.Write(bodyBytes)
		if writeErr != nil {
			panic(fmt.Sprintf("error writing workflowruns.json: %s", writeErr.Error()))
		}
		fh.Close()
	} else {
		bodyBytes, err := io.ReadAll(fh)
		if err != nil {
			panic(fmt.Sprintf("error reading workflowruns.json: %s", err.Error()))
		}
		err = json.Unmarshal(bodyBytes, &wr)
		if err != nil {
			panic(fmt.Sprintf("error deserializing the body: %s", err.Error()))
		}
		fh.Close()
	}

	return wr
}

func downloadZips(wr WorkflowRuns, token string) {
	client := http.Client{}
	for _, run := range wr.WorkflowRuns {
		if run.Event == "schedule" {
			fmt.Fprintln(os.Stderr, run.Event, run.HeadBranch, run.ArtifactsURL)
			zipName := filepath.Join(DATA_DIR, fmt.Sprintf("%d.zip", run.Id))
			zipHandler, err := os.Open(zipName)
			if err == nil {
				zipHandler.Close()
				continue
			}

			artifactsReq, err := http.NewRequest(http.MethodGet, run.ArtifactsURL, nil)
			if err != nil {
				panic(fmt.Sprintf("error creating artifacts request: %s", err))
			}

			res, err := client.Do(artifactsReq)
			if err != nil {
				panic(fmt.Sprintf("error doing the request: %s", err.Error()))
			}
			defer res.Body.Close()

			bodyBytes, err := io.ReadAll(res.Body)
			if err != nil {
				panic(fmt.Sprintf("error reading the body: %s", err.Error()))
			}

			var artifacts Artifacts
			err = json.Unmarshal(bodyBytes, &artifacts)
			if err != nil {
				panic(fmt.Sprintf("error deserializing the body: %s", err.Error()))
			}

			if artifacts.Count == 0 {
				fmt.Printf("WARNING: no artifacts for run '%d'\n", run.Id)
				continue
			}

			for _, artifact := range artifacts.Artifacts {
				fmt.Println(artifact.ArchiveDownloadURL)
				zipReq, err := http.NewRequest(http.MethodGet, artifact.ArchiveDownloadURL, nil)
				if err != nil {
					panic(fmt.Sprintf("error creating request for zip: %s", err.Error()))
				}

				zipReq.Header.Add("Authorization", fmt.Sprintf("Bearer %s", token))
				res, err := client.Do(zipReq)
				if err != nil {
					panic(fmt.Sprintf("error doing the request: %s", err.Error()))
				}
				defer res.Body.Close()

				if res.StatusCode != http.StatusOK {
					panic(fmt.Sprintf("request to download not OK: %d", res.StatusCode))
				}

				zipDest, err := os.Create(zipName)
				if err != nil {
					panic(fmt.Sprintf("error creating zip destination: %s", zipName))
				}

				_, err = io.Copy(zipDest, res.Body)
				if err != nil {
					panic(fmt.Sprintf("error reading the body: %s", err.Error()))
				}
			}
		}
	}
}

func unzipZips(wr WorkflowRuns) {
	for _, run := range wr.WorkflowRuns {
		if run.Event == "schedule" {
			zipName := filepath.Join(DATA_DIR, fmt.Sprintf("%d.zip", run.Id))
			fmt.Fprintf(os.Stderr, "Unzipping %s\n", zipName)
			reader, err := zip.OpenReader(zipName)
			if err != nil {
				if errors.Is(err, os.ErrNotExist) {
					fmt.Printf("no zip for '%d', probably a failed run.\n", run.Id)
					continue
				}
				panic(fmt.Sprintf("error unzipping %s: %s", zipName, err.Error()))
			}

			for _, f := range reader.File {
				filePath, err := SanitizeArchivePath(filepath.Join(DATA_DIR, fmt.Sprintf("%d", run.Id)), f.Name)
				if err != nil {
					panic(err)
				}
				if f.FileInfo().IsDir() {
					err = os.MkdirAll(filepath.Dir(filePath), os.ModePerm)
					if err != nil {
						panic(fmt.Sprintf("error creating directory %s: %s", filepath.Dir(filePath), err.Error()))
					}
					continue
				}

				err = os.MkdirAll(filepath.Dir(filePath), os.ModePerm)
				if err != nil {
					panic(fmt.Sprintf("error creating directory %s: %s", filePath, err.Error()))
				}

				destFile, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
				if err != nil {
					panic(fmt.Sprintf("error opening destination file %s: %s", filePath, err.Error()))
				}

				file, err := f.Open()
				if err != nil {
					panic(fmt.Sprintf("error opening file %s: %s", f.Name, err.Error()))
				}

				for {
					_, err := io.CopyN(destFile, file, 1024)
					if err != nil {
						if err == io.EOF {
							break
						}
						panic(fmt.Sprintf("error copying data to destination file: %s", err.Error()))
					}
				}

				destFile.Close()
				file.Close()
			}
		}
	}
}

func main() {
	token := os.Getenv("GH_TOKEN")
	if token == "" {
		fmt.Println("The environment variable 'GH_TOKEN' should be set.")
		os.Exit(1)
	}

	wr := readOrPullWorkflowRuns()
	downloadZips(wr, token)
	unzipZips(wr)

	GetCpuStats := getStats(wr, "get-cpu.csv", "cpu")
	fh, err := os.Create(filepath.Join(DATA_DIR, "get-cpu.csv"))
	if err != nil {
		panic(fmt.Sprintf("error opening merged 'get-cpu.csv': %s", err.Error()))
	}
	fmt.Fprintln(fh, "created.at,api.max,api.min,api.mean,api.median,sink.max,sink.min,sink.mean,sink.median,operator.max,operator.min,operator.mean,operator.median")
	for _, value := range GetCpuStats {
		fmt.Fprintf(fh, "%s,%f,%f,%f,%f,%f,%f,%f,%f,%f,%f,%f,%f\n", value.CreatedAt, value.Api.Max, value.Api.Min, value.Api.Mean, value.Api.Median, value.Sink.Max, value.Sink.Min, value.Sink.Mean, value.Sink.Median, value.Operator.Max, value.Operator.Min, value.Operator.Mean, value.Operator.Median)
	}
	fh.Close()

	CreateCpuStats := getStats(wr, "create-cpu.csv", "cpu")
	fh, err = os.Create(filepath.Join(DATA_DIR, "create-cpu.csv"))
	if err != nil {
		panic(fmt.Sprintf("error opening merged 'create-cpu.csv': %s", err.Error()))
	}
	fmt.Fprintln(fh, "created.at,api.max,api.min,api.mean,api.median,sink.max,sink.min,sink.mean,sink.median,operator.max,operator.min,operator.mean,operator.median")
	for _, value := range CreateCpuStats {
		fmt.Fprintf(fh, "%s,%f,%f,%f,%f,%f,%f,%f,%f,%f,%f,%f,%f\n", value.CreatedAt, value.Api.Max, value.Api.Min, value.Api.Mean, value.Api.Median, value.Sink.Max, value.Sink.Min, value.Sink.Mean, value.Sink.Median, value.Operator.Max, value.Operator.Min, value.Operator.Mean, value.Operator.Median)
	}
	fh.Close()

	GetMemoryStats := getStats(wr, "get-memory.csv", "memory")
	fh, err = os.Create(filepath.Join(DATA_DIR, "get-memory.csv"))
	if err != nil {
		panic(fmt.Sprintf("error opening merged 'get-memory.csv': %s", err.Error()))
	}
	fmt.Fprintln(fh, "created.at,api.max,api.min,api.mean,api.median,sink.max,sink.min,sink.mean,sink.median,operator.max,operator.min,operator.mean,operator.median")
	for _, value := range GetMemoryStats {
		fmt.Fprintf(fh, "%s,%.0f,%.0f,%.0f,%.0f,%.0f,%.0f,%.0f,%.0f,%.0f,%.0f,%.0f,%.0f\n", value.CreatedAt, value.Api.Max, value.Api.Min, value.Api.Mean, value.Api.Median, value.Sink.Max, value.Sink.Min, value.Sink.Mean, value.Sink.Median, value.Operator.Max, value.Operator.Min, value.Operator.Mean, value.Operator.Median)
	}
	fh.Close()

	CreateMemoryStats := getStats(wr, "create-memory.csv", "memory")
	fh, err = os.Create(filepath.Join(DATA_DIR, "create-memory.csv"))
	if err != nil {
		panic(fmt.Sprintf("error opening merged 'create-memory.csv': %s", err.Error()))
	}
	fmt.Fprintln(fh, "created.at,api.max,api.min,api.mean,api.median,sink.max,sink.min,sink.mean,sink.median,operator.max,operator.min,operator.mean,operator.median")
	for _, value := range CreateMemoryStats {
		fmt.Fprintf(fh, "%s,%.0f,%.0f,%.0f,%.0f,%.0f,%.0f,%.0f,%.0f,%.0f,%.0f,%.0f,%.0f\n", value.CreatedAt, value.Api.Max, value.Api.Min, value.Api.Mean, value.Api.Median, value.Sink.Max, value.Sink.Min, value.Sink.Mean, value.Sink.Median, value.Operator.Max, value.Operator.Min, value.Operator.Mean, value.Operator.Median)
	}
	fh.Close()

	CreateReqStats := getLocustStats(wr, "create_stats.csv")
	fh, err = os.Create(filepath.Join(DATA_DIR, "create-req.csv"))
	if err != nil {
		panic(fmt.Sprintf("error opening merged 'create-memory.csv': %s", err.Error()))
	}
	fmt.Fprintln(fh, "created.at,max,min,mean,median,req/s")
	for _, value := range CreateReqStats {
		fmt.Fprintf(fh, "%s,%.0f,%.0f,%.0f,%.0f,%.0f\n", value.CreatedAt, value.Sink.Max, value.Sink.Min, value.Sink.Mean, value.Sink.Median, value.Sink.RequestsPerSecond)
	}
	fh.Close()

	GetReqStats := getLocustStats(wr, "get_stats.csv")
	fh, err = os.Create(filepath.Join(DATA_DIR, "get-req.csv"))
	if err != nil {
		panic(fmt.Sprintf("error opening merged 'create-memory.csv': %s", err.Error()))
	}
	fmt.Fprintln(fh, "created.at,max,min,mean,median,req/s")
	for _, value := range GetReqStats {
		fmt.Fprintf(fh, "%s,%.0f,%.0f,%.0f,%.0f,%.0f\n", value.CreatedAt, value.Sink.Max, value.Sink.Min, value.Sink.Mean, value.Sink.Median, value.Sink.RequestsPerSecond)
	}
	fh.Close()
}

func getLocustStats(wr WorkflowRuns, path string) []Stats {
	stats := []Stats{}
	for _, run := range wr.WorkflowRuns {
		if run.Event == "schedule" {
			folder := filepath.Join(DATA_DIR, fmt.Sprintf("%d", run.Id))
			filePath := filepath.Join(folder, path)

			fh, err := os.Open(filePath)
			if err != nil {
				if errors.Is(err, fs.ErrNotExist) {
					fmt.Printf("'%s' does not exist, continue...\n", filePath)
					continue
				}
				panic(err)
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
					fmt.Println("line has the wrong number of fields")
					continue
				}

				records = append(records, record)
			}

			header := records[0]
			rawData := map[string]float64{}

			// We are only interested in the second row, where the aggregated values are
			record := records[2]
			for ix, valueString := range record {
				if ix == 0 || ix == 1 {
					continue // First two columns are Type and Name
				}

				value, err := strconv.ParseFloat(valueString, 64)
				if err != nil {
					fmt.Printf("error converting '%s' to float64: %s\n", valueString, err)
					panic(err)
				}
				rawData[header[ix]] = value
			}

			stat := Stats{
				Id:        run.Id,
				CreatedAt: run.CreatedAt,
				HeadSha:   run.HeadSha,
				Sink: Stat{
					Min:               rawData["Min Response Time"],
					Max:               rawData["Max Response Time"],
					Median:            rawData["Median Response Time"],
					Mean:              rawData["Mean Response Time"],
					RequestsPerSecond: rawData["Requests/s"],
				},
			}

			stats = append(stats, stat)
		}

	}
	return stats
}

func getStats(wr WorkflowRuns, path, metricType string) []Stats {
	stats := []Stats{}
	for _, run := range wr.WorkflowRuns {
		if run.Event == "schedule" {
			folder := filepath.Join(DATA_DIR, fmt.Sprintf("%d", run.Id))
			filePath := filepath.Join(folder, path)

			var outBuff bytes.Buffer
			if err := pkg.ExtractStats(filePath, metricType, "json", &outBuff); err != nil {
				if errors.Is(err, fs.ErrNotExist) {
					fmt.Printf("'%s' does not exist, continue...\n", filePath)
					continue
				}

				panic(err)
			}

			var stat Stats
			err := json.Unmarshal(outBuff.Bytes(), &stat)
			if err != nil {
				panic(fmt.Sprintf("error deserializing stats output for '%s': %s, %s", filePath, err.Error(), outBuff.String()))
			}

			stat.Id = run.Id
			stat.CreatedAt = run.CreatedAt
			stat.HeadSha = run.HeadSha
			stats = append(stats, stat)
		}
	}

	sort.Slice(stats, func(i, j int) bool {
		return stats[i].CreatedAt < stats[j].CreatedAt
	})

	return stats
}

// From https://github.com/securego/gosec/issues/324
func SanitizeArchivePath(d, t string) (v string, err error) {
	v = filepath.Join(d, t)
	if strings.HasPrefix(v, filepath.Clean(d)) {
		return v, nil
	}

	return "", fmt.Errorf("%s: %s", "content filepath is tainted", t)
}
