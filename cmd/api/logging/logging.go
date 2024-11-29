// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package logging

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/kubearchive/kubearchive/cmd/api/abort"
	"github.com/ohler55/ojg/jp"
	"github.com/ohler55/ojg/oj"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

const (
	userKey     string = "USER"
	passwordKey string = "PASSWORD"
	jsonPathKey string = "JSON_PATH"
)

// LogRetrieval retrieves a middleware function that checks the logging config and when set
// expects to find a log url in the context from where retrieve and parse logs
func LogRetrieval(loggingConfig map[string]string) gin.HandlerFunc {
	// FIXME For now the queries to the logging backend server are done insecurely
	client := http.Client{
		Transport: otelhttp.NewTransport(&http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}})}
	user := loggingConfig[userKey]
	password := loggingConfig[passwordKey]
	jsonPathParser, err := jp.ParseString(loggingConfig[jsonPathKey])
	if user == "" || password == "" || err != nil {
		return func(c *gin.Context) {
			abort.Abort(c, fmt.Errorf("invalid jsonPath expression in logging config: %s", err.Error()),
				http.StatusBadRequest)
		}
	}
	return func(c *gin.Context) {
		logUrl := c.GetString("logURL")

		request, errReq := http.NewRequest("GET", logUrl, nil)
		if errReq != nil {
			abort.Abort(c, errReq, http.StatusBadRequest)
		}

		request.SetBasicAuth(user, password)

		response, errReq := client.Do(request)
		if errReq != nil {
			abort.Abort(c, errReq, http.StatusInternalServerError)
			return
		}
		defer func(Body io.ReadCloser) {
			readErr := Body.Close()
			if readErr != nil {
				abort.Abort(c, readErr, http.StatusInternalServerError)
				return
			}
		}(response.Body)

		reader := bufio.NewReader(response.Body)
		line, readErr := readLine(reader)
		var jsonLine any
		var errJson error
		var logsReturned bool
		for readErr == nil {
			var parsedLines []string
			jsonLine, errJson = oj.Parse(line)
			if errJson != nil {
				abort.Abort(c, fmt.Errorf("retrieved log line isn't in the expected JSON format: %s",
					errJson.Error()), http.StatusInternalServerError)
				return
			}
			results := jsonPathParser.Get(jsonLine)
			for _, result := range results {
				parsedLines = append(parsedLines, result.(string))
			}
			if len(parsedLines) > 0 {
				logsReturned = true
				c.JSON(http.StatusOK, strings.Join(parsedLines, "\n"))
			}
			line, readErr = readLine(reader)
		}

		if !logsReturned {
			abort.Abort(c, fmt.Errorf("no parsed logs for the json path: %s", jsonPathParser.String()),
				http.StatusNotFound)
		}
	}
}

func readLine(reader *bufio.Reader) ([]byte, error) {
	var (
		isPrefix      bool  = true
		err           error = nil
		tmpLine, line []byte
	)
	for isPrefix && err == nil {
		tmpLine, isPrefix, err = reader.ReadLine()
		line = append(line, tmpLine...)
	}
	return line, err
}
