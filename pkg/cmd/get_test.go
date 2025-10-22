// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0
package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/kubearchive/kubearchive/pkg/cmd/config"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	"sigs.k8s.io/yaml"
)

// normalizeJSON normalizes JSON for comparison
func normalizeJSON(t *testing.T, jsonStr string) string {
	t.Helper()
	var obj interface{}
	err := json.Unmarshal([]byte(jsonStr), &obj)
	require.NoError(t, err)
	normalized, err := json.MarshalIndent(obj, "", "    ")
	require.NoError(t, err)
	return string(normalized)
}

// normalizeYAML normalizes YAML for comparison
func normalizeYAML(t *testing.T, yamlStr string) string {
	t.Helper()
	var obj interface{}
	err := yaml.Unmarshal([]byte(yamlStr), &obj)
	require.NoError(t, err)
	normalized, err := yaml.Marshal(obj)
	require.NoError(t, err)
	return string(normalized)
}

// createPodListJSON creates a PodList JSON with the given pod name and timestamp
func createPodListJSON(podName, timestamp string) string {
	podList := map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "PodList",
		"metadata": map[string]interface{}{
			"resourceVersion": "",
		},
		"items": []map[string]interface{}{
			{
				"apiVersion": "v1",
				"kind":       "Pod",
				"metadata": map[string]interface{}{
					"name":              podName,
					"namespace":         "default",
					"creationTimestamp": timestamp,
				},
			},
		},
	}

	jsonBytes, _ := json.Marshal(podList)
	return string(jsonBytes)
}

// MockKACLICommand implements the KACLICommand interface for testing
type MockKACLICommand struct {
	k8sResponse      string
	k9eResponse      string
	k8sError         error
	k9eError         error
	completeError    error
	namespaceValue   string
	namespaceError   error
	mockResourceInfo *config.ResourceInfo
}

func NewMockKACLICommand(mockErr error, resourceInfo *config.ResourceInfo) *MockKACLICommand {
	return &MockKACLICommand{
		completeError:    mockErr,
		mockResourceInfo: resourceInfo,
	}
}

// ResolveResourceSpec overrides the KAOptions method to return mock ResourceInfo
func (m *MockKACLICommand) ResolveResourceSpec(resourceSpec string) (*config.ResourceInfo, error) {
	return m.mockResourceInfo, nil
}

func (m *MockKACLICommand) GetFromAPI(api config.API, _ string) ([]byte, error) {
	switch api {
	case config.Kubernetes:
		if m.k8sError != nil {
			return nil, m.k8sError
		}
		return []byte(m.k8sResponse), nil
	case config.KubeArchive:
		if m.k9eError != nil {
			return nil, m.k9eError
		}
		return []byte(m.k9eResponse), nil
	default:
		return nil, fmt.Errorf("unknown API type")
	}
}

func (m *MockKACLICommand) Complete() error {
	return m.completeError
}

func (m *MockKACLICommand) AddFlags(_ *pflag.FlagSet) {}

func (m *MockKACLICommand) GetNamespace() (string, error) {
	if m.namespaceError != nil {
		return "", m.namespaceError
	}
	if m.namespaceValue != "" {
		return m.namespaceValue, nil
	}
	return "default", nil
}

// NewTestGetOptions creates GetOptions with mocks for testing
func NewTestGetOptions(mockCLI config.KACLICommand) *GetOptions {
	outputFormat := ""

	return &GetOptions{
		KACLICommand:       mockCLI,
		OutputFormat:       &outputFormat,
		JSONYamlPrintFlags: genericclioptions.NewJSONYamlPrintFlags(),
		IOStreams: genericiooptions.IOStreams{
			In:     nil,
			Out:    nil,
			ErrOut: nil,
		},
	}
}

func TestGetComplete(t *testing.T) {
	testCases := []struct {
		name            string
		allNamespaces   bool
		args            []string
		resourceInfo    *config.ResourceInfo
		expectedApiPath string
		mockError       error
		expectError     bool
		errorContains   string
	}{
		{
			name:          "core resource",
			allNamespaces: false,
			args:          []string{"pods"},
			resourceInfo: &config.ResourceInfo{
				Resource: "pods", Version: "v1", Group: "", GroupVersion: "v1", Kind: "Pod",
			},
			expectedApiPath: "/api/v1/namespaces/default/pods",
		},
		{
			name:          "non-core resource with version and group",
			allNamespaces: false,
			args:          []string{"jobs.v1.batch"},
			resourceInfo: &config.ResourceInfo{
				Resource: "jobs", Version: "v1", Group: "batch", GroupVersion: "batch/v1", Kind: "Job",
			},
			expectedApiPath: "/apis/batch/v1/namespaces/default/jobs",
		},
		{
			name:          "non-core resource with group only",
			allNamespaces: false,
			args:          []string{"deployments.apps"},
			resourceInfo: &config.ResourceInfo{
				Resource: "deployments", Version: "v1", Group: "apps", GroupVersion: "apps/v1", Kind: "Deployment",
			},
			expectedApiPath: "/apis/apps/v1/namespaces/default/deployments",
		},
		{
			name:          "short name resource",
			allNamespaces: false,
			args:          []string{"deploy"},
			resourceInfo: &config.ResourceInfo{
				Resource: "deployments", Version: "v1", Group: "apps", GroupVersion: "apps/v1", Kind: "Deployment",
			},
			expectedApiPath: "/apis/apps/v1/namespaces/default/deployments", // Should use actual resource name
		},
		{
			name:          "singular name resource",
			allNamespaces: false,
			args:          []string{"pod"},
			resourceInfo: &config.ResourceInfo{
				Resource: "pods", Version: "v1", Group: "", GroupVersion: "v1", Kind: "Pod",
			},
			expectedApiPath: "/api/v1/namespaces/default/pods", // Should use actual resource name
		},
		{
			name:          "all namespaces",
			allNamespaces: true,
			args:          []string{"pods"},
			resourceInfo: &config.ResourceInfo{
				Resource: "pods", Version: "v1", Group: "", GroupVersion: "v1", Kind: "Pod",
			},
			expectedApiPath: "/api/v1/pods",
		},
		{
			name:          "complete error",
			args:          []string{"pods"},
			mockError:     fmt.Errorf("mock complete failed"),
			expectError:   true,
			errorContains: "error completing the args: mock complete failed",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var options *GetOptions
			mockCli := NewMockKACLICommand(tc.mockError, tc.resourceInfo)
			options = NewTestGetOptions(mockCli)
			options.AllNamespaces = tc.allNamespaces

			err := options.Complete(tc.args)

			if tc.expectError {
				assert.Error(t, err)
				if tc.errorContains != "" {
					assert.Contains(t, err.Error(), tc.errorContains)
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expectedApiPath, options.APIPath)
				assert.NotNil(t, options.ResourceInfo)
			}
		})
	}
}

func TestRun(t *testing.T) {
	testCases := []struct {
		name               string
		k8sError           error
		k9eError           error
		outputFormat       string
		expectError        bool
		errorContains      string
		expectedOutputFile string
		needsNormalization bool
		useDynamicTime     bool
	}{
		{
			name:               "table output",
			outputFormat:       "",
			expectedOutputFile: "expected_table_output.txt",
			useDynamicTime:     true,
		},
		{
			name:               "JSON output",
			outputFormat:       "json",
			expectedOutputFile: "expected_json_output.json",
			needsNormalization: true,
		},
		{
			name:               "YAML output",
			outputFormat:       "yaml",
			expectedOutputFile: "expected_yaml_output.yaml",
			needsNormalization: true,
		},
		{
			name:          "API error",
			k8sError:      fmt.Errorf("connection failed"),
			expectError:   true,
			errorContains: "error retrieving the resources from Kubernetes API server",
		},
		{
			name:          "invalid output format",
			outputFormat:  "invalid",
			expectError:   true,
			errorContains: "error getting printer",
		},
		{
			name:          "invalid JSON response",
			outputFormat:  "",
			expectError:   true,
			errorContains: "error retrieving resources from the cluster",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var k8sResponse, k9eResponse string
			if !tc.expectError || tc.name == "invalid JSON response" || tc.name == "invalid output format" {
				var timestamp string
				if tc.useDynamicTime {
					timestamp = time.Now().Add(-5 * time.Minute).Format(time.RFC3339)
				} else {
					timestamp = "2025-07-08T09:54:00Z"
				}

				if tc.name == "invalid JSON response" {
					k8sResponse = `invalid json`
					k9eResponse = createPodListJSON("archived-pod-1", timestamp)
				} else {
					k8sResponse = createPodListJSON("test-pod-1", timestamp)
					k9eResponse = createPodListJSON("archived-pod-1", timestamp)
				}
			}

			mockCLI := &MockKACLICommand{
				k8sResponse:    k8sResponse,
				k9eResponse:    k9eResponse,
				k8sError:       tc.k8sError,
				k9eError:       tc.k9eError,
				namespaceValue: "default",
			}

			opts := NewTestGetOptions(mockCLI)
			opts.APIPath = "/api/v1/pods"
			opts.OutputFormat = &tc.outputFormat

			var outBuf bytes.Buffer
			opts.IOStreams = genericiooptions.IOStreams{Out: &outBuf}

			err := opts.Run()

			if tc.expectError {
				assert.Error(t, err)
				if tc.errorContains != "" {
					assert.Contains(t, err.Error(), tc.errorContains)
				}
			} else {
				assert.NoError(t, err)
				expectedOutput := loadGoldenFile(t, tc.expectedOutputFile)
				actualOutput := outBuf.String()

				if tc.needsNormalization {
					if tc.outputFormat == "json" {
						expectedNormalized := normalizeJSON(t, expectedOutput)
						actualNormalized := normalizeJSON(t, strings.TrimSpace(actualOutput))
						assert.Equal(t, expectedNormalized, actualNormalized)
					} else if tc.outputFormat == "yaml" {
						expectedNormalized := normalizeYAML(t, expectedOutput)
						actualNormalized := normalizeYAML(t, strings.TrimSpace(actualOutput))
						assert.Equal(t, expectedNormalized, actualNormalized)
					}
				} else {
					assert.Equal(t, expectedOutput, actualOutput)
				}
			}
		})
	}
}
