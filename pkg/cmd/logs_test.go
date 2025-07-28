// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0
package cmd

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLogsComplete(t *testing.T) {
	testCases := []struct {
		name                     string
		args                     []string
		expectedResource         string
		labelSelectorResultsPath string
		expectedLog              string
		shouldError              bool
	}{
		{
			name:             "one arg",
			args:             []string{"pod-name"},
			shouldError:      false,
			expectedResource: "pods",
			expectedLog:      "I'm a log line\n",
		},
		{
			name:             "one arg and container",
			args:             []string{"pod-name", "-c", "container-name"},
			shouldError:      false,
			expectedResource: "pods",
			expectedLog:      "I'm a log line\n",
		},
		{
			name:                     "two args with selector",
			args:                     []string{"batch/v1", "jobs", "-l", "app=test"},
			labelSelectorResultsPath: "testdata/jobs.json",
			shouldError:              false,
			expectedResource:         "jobs",
			expectedLog:              "I'm a log line\n",
		},
		{
			name:             "three args",
			args:             []string{"batch/v1", "jobs", "job-name"},
			shouldError:      false,
			expectedResource: "jobs",
			expectedLog:      "I'm a log line\n",
		},
		{
			name:                     "three args and container",
			args:                     []string{"batch/v1", "jobs", "job-name", "-c", "container-name"},
			labelSelectorResultsPath: "testdata/jobs.json",
			shouldError:              false,
			expectedResource:         "jobs",
			expectedLog:              "I'm a log line\n",
		},
		{
			name:                     "two args with selector",
			args:                     []string{"batch/v1", "jobs", "-l", "test=abc"},
			labelSelectorResultsPath: "testdata/jobs.json",
			shouldError:              false,
			expectedResource:         "jobs",
			expectedLog:              "I'm a log line\n",
		},
		{
			name:                     "no args with selector",
			args:                     []string{"-l", "test=abc"},
			labelSelectorResultsPath: "testdata/pods.json",
			shouldError:              false,
			expectedResource:         "pods",
			expectedLog:              "I'm a log line\nI'm a log line\n",
		},
		{
			name:        "no args",
			args:        []string{},
			shouldError: true,
		},
		{
			name:        "one arg with selector",
			args:        []string{"pod-name", "-l", "app=test"},
			shouldError: true,
		},
		{
			name:        "two args without selector",
			args:        []string{"batch/v1", "jobs"},
			shouldError: true,
		},
		{
			name:        "three args with selector",
			args:        []string{"batch/v1", "jobs", "my-job", "-l", "app=test"},
			shouldError: true,
		},
		{
			name:        "four args with selector",
			args:        []string{"batch/v1", "jobs", "my-job", "my-container", "-l", "app=test"},
			shouldError: true,
		},
		{
			name:        "four args",
			args:        []string{"batch/v1", "jobs", "job-name", "container-name"},
			shouldError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if !strings.Contains(r.URL.Path, tc.expectedResource) {
					w.WriteHeader(http.StatusNotFound)
					return
				}

				if strings.Contains(r.URL.RawQuery, "labelSelector") {
					fh, err := os.Open(tc.labelSelectorResultsPath)
					if err != nil {
						t.Fatal(err)
					}
					t.Cleanup(func() { fh.Close() })
					content, readErr := io.ReadAll(fh)
					if readErr != nil {
						t.Fatal(readErr)
					}
					_, errWrite := w.Write(content)
					if errWrite != nil {
						t.Fatal(errWrite)
					}
				} else {
					_, errWrite := w.Write([]byte("I'm a log line"))
					if errWrite != nil {
						t.Fatal(errWrite)
					}
				}
			}))
			defer srv.Close()

			args := append(tc.args, "--kubearchive-host")
			args = append(args, srv.URL)
			args = append(args, "--kubearchive-ca")
			args = append(args, "")
			command := NewLogCmd()

			var outBuf strings.Builder
			var errBuf strings.Builder
			command.SetOut(&outBuf)
			command.SetErr(&errBuf)
			command.SetArgs(args)
			err := command.Execute()

			if tc.shouldError {
				assert.Error(t, err)
				assert.NotEqual(t, "", errBuf.String())
			} else {
				assert.Equal(t, "", errBuf.String())
				assert.NoError(t, err)
				assert.Equal(t, tc.expectedLog, outBuf.String())
			}
		})
	}
}

func TestErrorLogs(t *testing.T) {
	testCases := []struct {
		name      string
		args      []string
		errorCode int
		errorMsg  string
	}{
		{
			name:      "unauthorized error",
			args:      []string{"pod-name"},
			errorMsg:  "unauthorized",
			errorCode: 401,
		},
		{
			name:      "not found error",
			args:      []string{"pod-name"},
			errorMsg:  "not found",
			errorCode: 404,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.errorCode)
			}))
			defer srv.Close()

			args := append(tc.args, "--kubearchive-host")
			args = append(args, srv.URL)
			args = append(args, "--kubearchive-ca")
			args = append(args, "")
			command := NewLogCmd()

			var outBuf strings.Builder
			var errBuf strings.Builder
			command.SetOut(&outBuf)
			command.SetErr(&errBuf)
			command.SetArgs(args)
			err := command.Execute()

			assert.Error(t, err)
			assert.NotEqual(t, "", errBuf.String())
			assert.Contains(t, errBuf.String(), tc.errorMsg)
		})
	}
}
