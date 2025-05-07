// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package logging

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kubearchive/kubearchive/pkg/abort"
	"github.com/kubearchive/kubearchive/pkg/files"
	"github.com/ohler55/ojg/jp"
	"github.com/ohler55/ojg/oj"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

const (
	loggingKey string = "headers"
)

// getKubeArchiveLoggingHeaders retrieves the kubearchive-logging secret data if it's available
func getKubeArchiveLoggingHeaders() map[string]string {
	loggingDir, exists := os.LookupEnv(files.LoggingDirEnvVar)
	if !exists {
		errMsg := fmt.Sprintf("environment variable not set: %s", files.LoggingDirEnvVar)
		slog.Warn(errMsg)
		return nil
	}
	configFiles, err := files.FilesInDir(loggingDir)
	if err != nil {
		errMsg := fmt.Sprintf("could not read kubearchive-logging secret files: %s", err.Error())
		slog.Warn(errMsg)
		return nil
	}
	if len(configFiles) == 0 {
		errMsg := "logging configuration not specified"
		slog.Warn(errMsg)
		return nil
	}

	loggingConf, err := files.LoggingConfigFromFiles(configFiles)
	if err != nil {
		errMsg := fmt.Sprintf("could not read kubearchive-logging secret files: %s", err.Error())
		slog.Warn(errMsg)
		return nil
	}
	return loggingConf
}

// SetLoggingHeaders sets the headers to be sent to the logging backend
func SetLoggingHeaders() gin.HandlerFunc {
	loggingHeaders := getKubeArchiveLoggingHeaders()
	return func(c *gin.Context) {
		c.Set(loggingKey, loggingHeaders)
	}
}

// LogRetrieval retrieves a middleware function that checks the logging config and when set
// expects to find a log url in the context from where retrieve and parse logs
// It should be called when user, password and logURL are set in the context.
func LogRetrieval() gin.HandlerFunc {
	// FIXME For now the queries to the logging backend server are done insecurely. Needed for the current test env.
	client := http.Client{
		Transport: otelhttp.NewTransport(&http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}), // #nosec G402
		Timeout: 60 * time.Second,
	}
	return func(c *gin.Context) {

		headers := c.GetStringMapString(loggingKey)
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

		request, errReq := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, logUrl, nil)
		if errReq != nil {
			abort.Abort(c, errReq, http.StatusInternalServerError)
			return
		}

		for key, value := range headers {
			request.Header.Set(key, value)
		}

		response, errReq := client.Do(request)
		if errReq != nil {
			abort.Abort(c, errReq, http.StatusInternalServerError)
			return
		}

		defer response.Body.Close()

		reader := bufio.NewReader(response.Body)
		var readErr error
		var logsReturned bool
		var line []byte

		for readErr != io.EOF {
			line, readErr = reader.ReadBytes('\n')
			if response.StatusCode != http.StatusOK {
				abort.Abort(c, fmt.Errorf("error response: %d - %s", response.StatusCode, line), response.StatusCode)
				return
			}
			if readErr != nil && readErr != io.EOF {
				abort.Abort(c, readErr, http.StatusInternalServerError)
				return
			}
			parsedLine, errParseLine := parseLine(line, jsonPathParser)
			if errParseLine != nil {
				abort.Abort(c, errParseLine, http.StatusInternalServerError)
				return
			}
			if parsedLine != "" {
				logsReturned = true
				c.String(http.StatusOK, parsedLine+"\n")
			}
		}

		if !logsReturned {
			abort.Abort(c, fmt.Errorf("no parsed logs for the json path: %s from logs in %s",
				jsonPathParser.String(), logUrl),
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
		return "", errJson
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
