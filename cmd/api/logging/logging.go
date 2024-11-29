// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package logging

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/kubearchive/kubearchive/cmd/api/abort"
	"github.com/kubearchive/kubearchive/pkg/files"
	"github.com/ohler55/ojg/jp"
	"github.com/ohler55/ojg/oj"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

const (
	userKey     string = "USER"
	passwordKey string = "PASSWORD"
)

func getKubeArchiveLoggingCredentials() (map[string]string, error) {
	loggingDir, exists := os.LookupEnv(files.LoggingDirEnvVar)
	if !exists {
		return nil, fmt.Errorf("environment variable not set: %s", files.LoggingDirEnvVar)
	}
	configFiles, err := files.FilesInDir(loggingDir)
	if err != nil {
		return nil, fmt.Errorf("could not read logging credentials: %w", err)
	}
	if len(configFiles) == 0 {
		return nil, fmt.Errorf("logging secret is empty. To configure logging update the kubearchive-logging Secret")
	}

	loggingConf, err := files.LoggingConfigFromFiles(configFiles)
	if err != nil {
		return nil, fmt.Errorf("could not get value for logging credentials: %w", err)
	}
	return loggingConf, nil
}

// LogRetrieval retrieves a middleware function that checks the logging config and when set
// expects to find a log url in the context from where retrieve and parse logs
func LogRetrieval() gin.HandlerFunc {
	// FIXME For now the queries to the logging backend server are done insecurely. Needed for the current test env.
	// #nosec G402
	client := http.Client{
		Transport: otelhttp.NewTransport(&http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}})}
	loggingCreds, err := getKubeArchiveLoggingCredentials()
	user := loggingCreds[userKey]
	password := loggingCreds[passwordKey]
	if user == "" || password == "" || err != nil {
		err = fmt.Errorf("logging secret values unset. To configure logging update the kubearchive-logging Secret")
		return func(c *gin.Context) {
			abort.Abort(c, err, http.StatusBadRequest)
		}
	}
	return func(c *gin.Context) {
		logUrl := c.GetString("logURL")
		jsonPath := c.GetString("jsonPath")

		if logUrl == "" {
			abort.Abort(c, fmt.Errorf("no log URL found"), http.StatusNotFound)
			return
		}
		slog.Info("Retrieving logs", "logURL", logUrl)

		var jsonPathParser jp.Expr
		var errJsonPath error

		if jsonPath != "" {
			jsonPathParser, errJsonPath = jp.ParseString(jsonPath)
			if errJsonPath != nil {
				abort.Abort(c, fmt.Errorf("invalid jsonPath %s for url %s", jsonPath, logUrl),
					http.StatusInternalServerError)
				return
			}
		}

		request, errReq := http.NewRequest(http.MethodGet, logUrl, nil)
		if errReq != nil {
			abort.Abort(c, errReq, http.StatusInternalServerError)
			return
		}

		request.SetBasicAuth(user, password)

		response, errReq := client.Do(request)
		if errReq != nil {
			abort.Abort(c, errReq, http.StatusInternalServerError)
			return
		}

		defer response.Body.Close()

		reader := bufio.NewReader(response.Body)
		line, readErr := readLine(reader)
		var logsReturned bool
		for readErr == nil {
			parsedLine, errParseLine := parseLine(line, jsonPathParser)
			if errParseLine != nil {
				abort.Abort(c, errParseLine, http.StatusInternalServerError)
				return
			}
			if parsedLine != "" {
				logsReturned = true
				c.JSON(http.StatusOK, parsedLine+"\n")
			}
			line, readErr = readLine(reader)
		}

		if !logsReturned {
			abort.Abort(c, fmt.Errorf("no parsed logs for the json path: %s", jsonPathParser.String()),
				http.StatusNotFound)
		}
	}
}

func parseLine(line []byte, jsonPathParser jp.Expr) (string, error) {
	if jsonPathParser == nil {
		return string(line), nil
	}

	var jsonLine any
	var errJson error
	var result string

	jsonLine, errJson = oj.Parse(line)
	if errJson != nil {
		return result, errJson
	}
	for _, res := range jsonPathParser.Get(jsonLine) {
		if result == "" {
			result = res.(string)
		} else {
			result = fmt.Sprintf("%s\n%s", result, res.(string))
		}
	}
	return result, nil
}

func readLine(reader *bufio.Reader) ([]byte, error) {
	var (
		isPrefix            = true
		err           error = nil
		tmpLine, line []byte
	)
	for isPrefix && err == nil {
		tmpLine, isPrefix, err = reader.ReadLine()
		line = append(line, tmpLine...)
	}
	return line, err
}
