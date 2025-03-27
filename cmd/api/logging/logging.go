// Copyright Kronicler Authors
// SPDX-License-Identifier: Apache-2.0

package logging

import (
	"bufio"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kronicler/kronicler/pkg/abort"
	"github.com/kronicler/kronicler/pkg/files"
	"github.com/ohler55/ojg/jp"
	"github.com/ohler55/ojg/oj"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

const (
	userKey     string = "USER"
	passwordKey string = "PASSWORD"
)

// GetKroniclerLoggingCredentials retrieves the kronicler-logging secret data and
// an error if there isn't available
func GetKroniclerLoggingCredentials() (map[string]string, error) {
	loggingDir, exists := os.LookupEnv(files.LoggingDirEnvVar)
	errMsg := fmt.Sprintf("environment variable not set: %s", files.LoggingDirEnvVar)
	if !exists {
		return nil, errors.New(errMsg)
	}
	slog.Warn(errMsg)
	configFiles, err := files.FilesInDir(loggingDir)
	if err != nil {
		errMsg = fmt.Sprintf("could not read logging credentials: %s", err.Error())
		slog.Warn(errMsg)
		return nil, errors.New(errMsg)
	}
	if len(configFiles) == 0 {
		errMsg = "Logging secret is empty. To configure logging update the kronicler-logging Secret"
		slog.Warn(errMsg)
		return nil, errors.New(errMsg)
	}

	loggingConf, err := files.LoggingConfigFromFiles(configFiles)
	if err != nil {
		return nil, fmt.Errorf("could not get value for logging credentials: %w", err)
	}
	return loggingConf, nil
}

// SetLoggingCredentials sets the user and password for accessing logging backend in the request context
func SetLoggingCredentials(loggingCreds map[string]string, loggingCredsErr error) gin.HandlerFunc {
	user := loggingCreds[userKey]
	password := loggingCreds[passwordKey]
	if user == "" || password == "" {
		err := fmt.Errorf(
			"logging secret user or password unset")
		if loggingCredsErr != nil {
			err = fmt.Errorf("%w: %w", err, loggingCredsErr)
		}
		return func(c *gin.Context) {
			abort.Abort(c, err, http.StatusBadRequest)
		}
	}
	return func(c *gin.Context) {
		c.Set(userKey, user)
		c.Set(passwordKey, password)
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
		user := c.GetString(userKey)
		password := c.GetString(passwordKey)
		if user == "" || password == "" {
			loggingCredsErr := fmt.Errorf(
				"unexpected error. Logging credentials are unset")
			abort.Abort(c, loggingCredsErr, http.StatusInternalServerError)
			return
		}
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

		request.SetBasicAuth(user, password)

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
