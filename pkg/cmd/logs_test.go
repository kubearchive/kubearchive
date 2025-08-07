// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0
package cmd

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/kubearchive/kubearchive/pkg/cmd/config"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockKACLICommandForLogs implements the KACLICommand interface for logs testing
type MockKACLICommandForLogs struct {
	responses      map[string]string // Path -> response
	errors         map[string]error  // Path -> error
	completeError  error
	namespaceValue string
	namespaceError error
}

func (m *MockKACLICommandForLogs) GetFromAPI(_ config.API, path string) ([]byte, error) {
	// Check for errors first
	if err, exists := m.errors[path]; exists {
		return nil, err
	}

	// Return response if it exists
	if response, exists := m.responses[path]; exists {
		return []byte(response), nil
	}

	// If no response is configured, return an error
	return nil, fmt.Errorf("unexpected API call to path: %s", path)
}

func (m *MockKACLICommandForLogs) Complete() error {
	return m.completeError
}

func (m *MockKACLICommandForLogs) AddFlags(_ *pflag.FlagSet) {}

func (m *MockKACLICommandForLogs) GetNamespace() (string, error) {
	if m.namespaceError != nil {
		return "", m.namespaceError
	}
	if m.namespaceValue != "" {
		return m.namespaceValue, nil
	}
	return "default", nil
}

// NewTestLogsOptions creates LogsOptions with a mock for testing
func NewTestLogsOptions(mockCLI *MockKACLICommandForLogs) *LogsOptions {
	return &LogsOptions{
		KACLICommand: mockCLI,
	}
}

func TestLogsOptionsComplete(t *testing.T) {
	testCases := []struct {
		name           string
		args           []string
		labelSelector  string
		expectedError  bool
		expectedResult *LogsOptions
	}{
		{
			name:          "no args with selector",
			args:          []string{},
			labelSelector: "app=test",
			expectedError: false,
			expectedResult: &LogsOptions{
				GroupVersion: "v1",
				Resource:     "pods",
			},
		},
		{
			name:          "one arg",
			args:          []string{"pod-name"},
			expectedError: false,
			expectedResult: &LogsOptions{
				GroupVersion: "v1",
				Resource:     "pods",
				Name:         "pod-name",
			},
		},
		{
			name:          "two args with selector",
			args:          []string{"batch/v1", "jobs"},
			labelSelector: "app=test",
			expectedError: false,
			expectedResult: &LogsOptions{
				GroupVersion: "batch/v1",
				Resource:     "jobs",
			},
		},
		{
			name:          "three args",
			args:          []string{"batch/v1", "jobs", "job-name"},
			expectedError: false,
			expectedResult: &LogsOptions{
				GroupVersion: "batch/v1",
				Resource:     "jobs",
				Name:         "job-name",
			},
		},
		{
			name:          "no args without selector",
			args:          []string{},
			expectedError: true,
		},
		{
			name:          "one arg with selector",
			args:          []string{"pod-name"},
			labelSelector: "app=test",
			expectedError: true,
		},
		{
			name:          "two args without selector",
			args:          []string{"batch/v1", "jobs"},
			expectedError: true,
		},
		{
			name:          "three args with selector",
			args:          []string{"batch/v1", "jobs", "job-name"},
			labelSelector: "app=test",
			expectedError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			options := NewLogsOptions()
			options.LabelSelector = tc.labelSelector

			err := options.Complete(tc.args)

			if tc.expectedError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.expectedResult.GroupVersion, options.GroupVersion)
			assert.Equal(t, tc.expectedResult.Resource, options.Resource)
			assert.Equal(t, tc.expectedResult.Name, options.Name)
		})
	}
}

func TestLogsRun(t *testing.T) {
	podsData := loadGoldenFile(t, "pods.json")
	jobsData := loadGoldenFile(t, "jobs.json")

	testCases := []struct {
		name           string
		groupVersion   string
		resource       string
		resourceName   string
		containerName  string
		labelSelector  string
		namespace      string
		mock           *MockKACLICommandForLogs
		expectError    bool
		errorContains  string
		expectedOutput string
	}{
		{
			name:         "single pod logs from pods.json",
			groupVersion: "v1",
			resource:     "pods",
			resourceName: "generate-log-1-29141722-k7s8m",
			namespace:    "generate-logs-cronjobs",
			mock: &MockKACLICommandForLogs{
				namespaceValue: "generate-logs-cronjobs",
				responses: map[string]string{
					"/api/v1/namespaces/generate-logs-cronjobs/pods/generate-log-1-29141722-k7s8m/log": "Log output from pod generate-log-1-29141722-k7s8m",
				},
			},
			expectedOutput: "Log output from pod generate-log-1-29141722-k7s8m\n",
		},
		{
			name:          "pod logs with container from pods.json",
			groupVersion:  "v1",
			resource:      "pods",
			resourceName:  "generate-log-1-29141722-k7s8m",
			containerName: "generate3",
			namespace:     "generate-logs-cronjobs",
			mock: &MockKACLICommandForLogs{
				namespaceValue: "generate-logs-cronjobs",
				responses: map[string]string{
					"/api/v1/namespaces/generate-logs-cronjobs/pods/generate-log-1-29141722-k7s8m/log?container=generate3": "Log output from generate3 container",
				},
			},
			expectedOutput: "Log output from generate3 container\n",
		},
		{
			name:         "job logs from jobs.json",
			groupVersion: "batch/v1",
			resource:     "jobs",
			resourceName: "generate-log-1-29141722",
			namespace:    "generate-logs-cronjobs",
			mock: &MockKACLICommandForLogs{
				namespaceValue: "generate-logs-cronjobs",
				responses: map[string]string{
					"/apis/batch/v1/namespaces/generate-logs-cronjobs/jobs/generate-log-1-29141722/log": "Log output from job generate-log-1-29141722",
				},
			},
			expectedOutput: "Log output from job generate-log-1-29141722\n",
		},
		{
			name:          "pods with label selector from pods.json",
			groupVersion:  "v1",
			resource:      "pods",
			labelSelector: "batch.kubernetes.io/job-name=generate-log-1-29141722",
			namespace:     "generate-logs-cronjobs",
			mock: &MockKACLICommandForLogs{
				namespaceValue: "generate-logs-cronjobs",
				responses: map[string]string{
					"/api/v1/namespaces/generate-logs-cronjobs/pods?labelSelector=batch.kubernetes.io%2Fjob-name%3Dgenerate-log-1-29141722": podsData,
					"/api/v1/namespaces/generate-logs-cronjobs/pods/generate-log-1-29141722-k7s8m/log":                                      "Log output from pod generate-log-1-29141722-k7s8m",
					"/api/v1/namespaces/generate-logs-cronjobs/pods/generate-log-1-29141723-vvvds/log":                                      "Log output from pod generate-log-1-29141723-vvvds",
				},
			},
			expectedOutput: "Log output from pod generate-log-1-29141722-k7s8m\nLog output from pod generate-log-1-29141723-vvvds\n",
		},
		{
			name:         "API error",
			groupVersion: "v1",
			resource:     "pods",
			resourceName: "generate-log-1-29141722-k7s8m",
			namespace:    "generate-logs-cronjobs",
			mock: &MockKACLICommandForLogs{
				namespaceValue: "generate-logs-cronjobs",
				errors: map[string]error{
					"/api/v1/namespaces/generate-logs-cronjobs/pods/generate-log-1-29141722-k7s8m/log": fmt.Errorf("connection failed"),
				},
			},
			expectError:   true,
			errorContains: "error retrieving resources from the KubeArchive API",
		},
		{
			name:         "namespace error",
			groupVersion: "v1",
			resource:     "pods",
			resourceName: "generate-log-1-29141722-k7s8m",
			mock: &MockKACLICommandForLogs{
				namespaceError: fmt.Errorf("namespace error"),
			},
			expectError:   true,
			errorContains: "error getting namespace",
		},
		{
			name:          "no resources found with label selector",
			groupVersion:  "v1",
			resource:      "pods",
			labelSelector: "app=nonexistent",
			namespace:     "generate-logs-cronjobs",
			mock: &MockKACLICommandForLogs{
				namespaceValue: "generate-logs-cronjobs",
				responses: map[string]string{
					"/api/v1/namespaces/generate-logs-cronjobs/pods?labelSelector=app%3Dnonexistent": `{"apiVersion":"v1","kind":"List","metadata":{},"items":[]}`,
				},
			},
			expectError:   true,
			errorContains: "no resources found in the generate-logs-cronjobs namespace",
		},
		{
			name:          "cronjob to jobs to pods to container logs flow",
			groupVersion:  "batch/v1",
			resource:      "jobs",
			labelSelector: "job-name=generate-log-1-29141722",
			containerName: "generate1",
			namespace:     "generate-logs-cronjobs",
			mock: &MockKACLICommandForLogs{
				namespaceValue: "generate-logs-cronjobs",
				responses: map[string]string{
					"/apis/batch/v1/namespaces/generate-logs-cronjobs/jobs?labelSelector=job-name%3Dgenerate-log-1-29141722": jobsData,
					"/apis/batch/v1/namespaces/generate-logs-cronjobs/jobs/generate-log-1-29141722/log?container=generate1":  "Log output from job generate-log-1-29141722 container generate1",
				},
			},
			expectedOutput: "Log output from job generate-log-1-29141722 container generate1\n",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			opts := NewTestLogsOptions(tc.mock)
			opts.GroupVersion = tc.groupVersion
			opts.Resource = tc.resource
			opts.Name = tc.resourceName
			opts.ContainerName = tc.containerName
			opts.LabelSelector = tc.labelSelector

			var outBuf bytes.Buffer
			cmd := &cobra.Command{}
			cmd.SetOut(&outBuf)

			err := opts.Run(cmd)

			if tc.expectError {
				assert.Error(t, err)
				if tc.errorContains != "" {
					assert.Contains(t, err.Error(), tc.errorContains)
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expectedOutput, outBuf.String())
			}
		})
	}
}
