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
)

// MockKACLICommandForLogs implements the KACLICommand interface for logs testing
type MockKACLICommandForLogs struct {
	responses        map[string]string           // Path -> response
	errors           map[string]*config.APIError // Path -> error
	completeError    error
	namespaceValue   string
	namespaceError   error
	mockResourceInfo *config.ResourceInfo
}

func NewMockKACLICommandForLogs(completeErr error, resourceInfo *config.ResourceInfo) *MockKACLICommandForLogs {
	return &MockKACLICommandForLogs{
		completeError:    completeErr,
		mockResourceInfo: resourceInfo,
		responses:        make(map[string]string),
		errors:           make(map[string]*config.APIError),
	}
}

func (m *MockKACLICommandForLogs) GetFromAPI(_ config.API, path string) ([]byte, *config.APIError) {
	// Check for errors first
	if err, exists := m.errors[path]; exists {
		return nil, err
	}

	// Return response if it exists
	if response, exists := m.responses[path]; exists {
		return []byte(response), nil
	}

	// If no response is configured, return an error
	return nil, &config.APIError{
		StatusCode: 500,
		URL:        path,
		Message:    fmt.Sprintf("unexpected API call to path: %s", path),
		Body:       "",
	}
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

func (m *MockKACLICommandForLogs) ResolveResourceSpec(resourceSpec string) (*config.ResourceInfo, error) {
	return m.mockResourceInfo, nil
}

// NewTestLogsOptions creates LogsOptions with a mock for testing
func NewTestLogsOptions(mockCLI *MockKACLICommandForLogs) *LogsOptions {
	return &LogsOptions{
		KACLICommand: mockCLI,
	}
}

func TestLogsComplete(t *testing.T) {
	testCases := []struct {
		name          string
		args          []string
		labelSelector string
		resourceInfo  *config.ResourceInfo
		expectedName  string
		mockError     error
		expectError   bool
		errorContains string
	}{
		{
			name: "resource/name format",
			args: []string{"pod/test-pod"},
			resourceInfo: &config.ResourceInfo{
				Resource: "pods", Version: "v1", Group: "", GroupVersion: "v1", Kind: "Pod", Namespaced: true,
			},
			expectedName: "test-pod", // Fixed implementation: parts[1] becomes Name
		},
		{
			name:          "resource only with labelSelector",
			args:          []string{"pods"},
			labelSelector: "app=test",
			resourceInfo: &config.ResourceInfo{
				Resource: "pods", Version: "v1", Group: "", GroupVersion: "v1", Kind: "Pod", Namespaced: true,
			},
			expectedName: "", // With labelSelector, single arg becomes resourceSpec, Name stays empty
		},
		{
			name: "pod name only (no labelSelector)",
			args: []string{"pods"},
			resourceInfo: &config.ResourceInfo{
				Resource: "pods", Version: "v1", Group: "", GroupVersion: "v1", Kind: "Pod", Namespaced: true,
			},
			expectedName: "pods", // Without labelSelector, single arg becomes Name, resourceSpec defaults to "pods"
		},
		{
			name: "pod name only",
			args: []string{"test-pod"},
			resourceInfo: &config.ResourceInfo{
				Resource: "pods", Version: "v1", Group: "", GroupVersion: "v1", Kind: "Pod", Namespaced: true,
			},
			expectedName: "test-pod", // Single arg becomes Name when no labelSelector
		},
		{
			name: "non-core resource",
			args: []string{"job/test-job"},
			resourceInfo: &config.ResourceInfo{
				Resource: "jobs", Version: "v1", Group: "batch", GroupVersion: "batch/v1", Kind: "Job", Namespaced: true,
			},
			expectedName: "test-job", // Fixed implementation: parts[1] becomes Name
		},
		{
			name:          "name and label selector together",
			args:          []string{"pod/my-pod"},
			labelSelector: "app=nginx",
			expectError:   true,
			errorContains: "cannot specify both a resource name and a label selector",
		},
		{
			name:          "complete error",
			args:          []string{"pods"},
			mockError:     fmt.Errorf("mock complete failed"),
			expectError:   true,
			errorContains: "mock complete failed",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockCLI := NewMockKACLICommandForLogs(tc.mockError, tc.resourceInfo)
			mockCLI.namespaceValue = "default"

			options := NewTestLogsOptions(mockCLI)
			options.LabelSelector = tc.labelSelector

			err := options.Complete(tc.args)

			if tc.expectError {
				assert.Error(t, err)
				if tc.errorContains != "" {
					assert.Contains(t, err.Error(), tc.errorContains)
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, options.ResourceInfo)
				assert.Equal(t, tc.resourceInfo.Resource, options.ResourceInfo.Resource)
				assert.Equal(t, tc.resourceInfo.Version, options.ResourceInfo.Version)
				assert.Equal(t, tc.resourceInfo.Group, options.ResourceInfo.Group)
				assert.Equal(t, tc.resourceInfo.GroupVersion, options.ResourceInfo.GroupVersion)
				assert.Equal(t, tc.resourceInfo.Kind, options.ResourceInfo.Kind)
				assert.Equal(t, tc.expectedName, options.Name)
			}
		})
	}
}

func TestLogsRun(t *testing.T) {
	podsData := loadGoldenFile(t, "pods.json")
	jobsData := loadGoldenFile(t, "jobs.json")

	testCases := []struct {
		name           string
		resourceInfo   *config.ResourceInfo
		resourceName   string
		containerName  string
		labelSelector  string
		namespace      string
		responses      map[string]string
		errors         map[string]*config.APIError
		namespaceError error
		expectError    bool
		errorContains  string
		expectedOutput string
	}{
		{
			name: "single pod logs from pods.json",
			resourceInfo: &config.ResourceInfo{
				Resource: "pods", Version: "v1", Group: "", GroupVersion: "v1", Kind: "Pod", Namespaced: true,
			},
			resourceName: "generate-log-1-29141722-k7s8m",
			namespace:    "generate-logs-cronjobs",
			responses: map[string]string{
				"/api/v1/namespaces/generate-logs-cronjobs/pods/generate-log-1-29141722-k7s8m/log": "Log output from pod generate-log-1-29141722-k7s8m",
			},
			expectedOutput: "Log output from pod generate-log-1-29141722-k7s8m\n",
		},
		{
			name: "pod logs with container from pods.json",
			resourceInfo: &config.ResourceInfo{
				Resource: "pods", Version: "v1", Group: "", GroupVersion: "v1", Kind: "Pod", Namespaced: true,
			},
			resourceName:  "generate-log-1-29141722-k7s8m",
			containerName: "generate3",
			namespace:     "generate-logs-cronjobs",
			responses: map[string]string{
				"/api/v1/namespaces/generate-logs-cronjobs/pods/generate-log-1-29141722-k7s8m/log?container=generate3": "Log output from generate3 container",
			},
			expectedOutput: "Log output from generate3 container\n",
		},
		{
			name: "job logs from jobs.json",
			resourceInfo: &config.ResourceInfo{
				Resource: "jobs", Version: "v1", Group: "batch", GroupVersion: "batch/v1", Kind: "Job", Namespaced: true,
			},
			resourceName: "generate-log-1-29141722",
			namespace:    "generate-logs-cronjobs",
			responses: map[string]string{
				"/apis/batch/v1/namespaces/generate-logs-cronjobs/jobs/generate-log-1-29141722/log": "Log output from job generate-log-1-29141722",
			},
			expectedOutput: "Log output from job generate-log-1-29141722\n",
		},
		{
			name: "pods with label selector from pods.json",
			resourceInfo: &config.ResourceInfo{
				Resource: "pods", Version: "v1", Group: "", GroupVersion: "v1", Kind: "Pod", Namespaced: true,
			},
			labelSelector: "batch.kubernetes.io/job-name=generate-log-1-29141722",
			namespace:     "generate-logs-cronjobs",
			responses: map[string]string{
				"/api/v1/namespaces/generate-logs-cronjobs/pods?labelSelector=batch.kubernetes.io%2Fjob-name%3Dgenerate-log-1-29141722": podsData,
				"/api/v1/namespaces/generate-logs-cronjobs/pods/generate-log-1-29141722-k7s8m/log":                                      "Log output from pod generate-log-1-29141722-k7s8m",
				"/api/v1/namespaces/generate-logs-cronjobs/pods/generate-log-1-29141723-vvvds/log":                                      "Log output from pod generate-log-1-29141723-vvvds",
			},
			expectedOutput: "Log output from pod generate-log-1-29141722-k7s8m\nLog output from pod generate-log-1-29141723-vvvds\n",
		},
		{
			name: "API error",
			resourceInfo: &config.ResourceInfo{
				Resource: "pods", Version: "v1", Group: "", GroupVersion: "v1", Kind: "Pod", Namespaced: true,
			},
			resourceName: "generate-log-1-29141722-k7s8m",
			namespace:    "generate-logs-cronjobs",
			errors: map[string]*config.APIError{
				"/api/v1/namespaces/generate-logs-cronjobs/pods/generate-log-1-29141722-k7s8m/log": &config.APIError{
					StatusCode: 500,
					URL:        "/api/v1/namespaces/generate-logs-cronjobs/pods/generate-log-1-29141722-k7s8m/log",
					Message:    "connection failed",
					Body:       "",
				},
			},
			expectError:   true,
			errorContains: "connection failed",
		},
		{
			name: "namespace error",
			resourceInfo: &config.ResourceInfo{
				Resource: "pods", Version: "v1", Group: "", GroupVersion: "v1", Kind: "Pod", Namespaced: true,
			},
			resourceName:   "generate-log-1-29141722-k7s8m",
			namespaceError: fmt.Errorf("namespace error"),
			expectError:    true,
			errorContains:  "error getting namespace",
		},
		{
			name: "no resources found with label selector",
			resourceInfo: &config.ResourceInfo{
				Resource: "pods", Version: "v1", Group: "", GroupVersion: "v1", Kind: "Pod", Namespaced: true,
			},
			labelSelector: "app=nonexistent",
			namespace:     "generate-logs-cronjobs",
			responses: map[string]string{
				"/api/v1/namespaces/generate-logs-cronjobs/pods?labelSelector=app%3Dnonexistent": `{"apiVersion":"v1","kind":"List","metadata":{},"items":[]}`,
			},
			expectError:   true,
			errorContains: "no resources found in generate-logs-cronjobs namespace",
		},
		{
			name: "cronjob to jobs to pods to container logs flow",
			resourceInfo: &config.ResourceInfo{
				Resource: "jobs", Version: "v1", Group: "batch", GroupVersion: "batch/v1", Kind: "Job", Namespaced: true,
			},
			labelSelector: "job-name=generate-log-1-29141722",
			containerName: "generate1",
			namespace:     "generate-logs-cronjobs",
			responses: map[string]string{
				"/apis/batch/v1/namespaces/generate-logs-cronjobs/jobs?labelSelector=job-name%3Dgenerate-log-1-29141722": jobsData,
				"/apis/batch/v1/namespaces/generate-logs-cronjobs/jobs/generate-log-1-29141722/log?container=generate1":  "Log output from job generate-log-1-29141722 container generate1",
			},
			expectedOutput: "Log output from job generate-log-1-29141722 container generate1\n",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockCLI := NewMockKACLICommandForLogs(nil, tc.resourceInfo)
			mockCLI.namespaceValue = tc.namespace
			mockCLI.namespaceError = tc.namespaceError

			// Set up responses and errors
			if tc.responses != nil {
				for path, response := range tc.responses {
					mockCLI.responses[path] = response
				}
			}
			if tc.errors != nil {
				for path, err := range tc.errors {
					mockCLI.errors[path] = err
				}
			}

			opts := NewTestLogsOptions(mockCLI)
			opts.ResourceInfo = tc.resourceInfo
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
