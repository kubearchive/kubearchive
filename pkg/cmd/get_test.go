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

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	"k8s.io/client-go/rest"
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

// createPodListJSON creates a PodList JSON with the given pod name, timestamp, and optional status
// If status is empty, no status field is added. If uid is empty, a unique UID is generated based on pod name.
func createPodListJSON(podName, timestamp, status, uid string) string {
	// Generate a unique UID if not provided
	if uid == "" {
		uid = fmt.Sprintf("%s-uid-12345678-1234-1234-1234-123456789abc", podName)
	}

	pod := map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata": map[string]interface{}{
			"name":              podName,
			"namespace":         "default",
			"uid":               uid,
			"creationTimestamp": timestamp,
		},
	}

	// Add status if provided
	if status != "" {
		pod["status"] = map[string]interface{}{
			"phase": status,
		}
	}

	podList := map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "PodList",
		"metadata": map[string]interface{}{
			"resourceVersion": "",
		},
		"items": []map[string]interface{}{pod},
	}

	jsonBytes, _ := json.Marshal(podList)
	return string(jsonBytes)
}

// MockKARetrieverCommandForGet implements the KARetrieverCommand interface for testing get command
type MockKARetrieverCommandForGet struct {
	k8sResponse      string
	k9eResponse      string
	k8sError         *APIError
	k9eError         *APIError
	completeError    error
	namespaceValue   string
	namespaceError   error
	mockResourceInfo *ResourceInfo
}

func NewMockKARetrieverCommandForGet(mockErr error, resourceInfo *ResourceInfo) *MockKARetrieverCommandForGet {
	return &MockKARetrieverCommandForGet{
		completeError:    mockErr,
		mockResourceInfo: resourceInfo,
	}
}

// ResolveResourceSpec overrides the KAOptions method to return mock ResourceInfo
func (m *MockKARetrieverCommandForGet) ResolveResourceSpec(resourceSpec string) (*ResourceInfo, error) {
	return m.mockResourceInfo, nil
}

func (m *MockKARetrieverCommandForGet) GetFromAPI(api API, _ string) ([]byte, *APIError) {
	switch api {
	case Kubernetes:
		if m.k8sError != nil {
			return nil, m.k8sError
		}
		return []byte(m.k8sResponse), nil
	case KubeArchive:
		if m.k9eError != nil {
			return nil, m.k9eError
		}
		return []byte(m.k9eResponse), nil
	default:
		return nil, &APIError{StatusCode: 500, URL: "unknown-api", Message: "unknown API type", Body: ""}
	}
}

func (m *MockKARetrieverCommandForGet) CompleteK8sConfig() error {
	return m.completeError
}

func (m *MockKARetrieverCommandForGet) AddK8sFlags(_ *pflag.FlagSet)       {}
func (m *MockKARetrieverCommandForGet) AddRetrieverFlags(_ *pflag.FlagSet) {}

func (m *MockKARetrieverCommandForGet) GetNamespace() (string, error) {
	if m.namespaceError != nil {
		return "", m.namespaceError
	}
	if m.namespaceValue != "" {
		return m.namespaceValue, nil
	}
	return "default", nil
}

func (m *MockKARetrieverCommandForGet) GetK8sRESTConfig() *rest.Config {
	// Return a mock REST config for testing
	return &rest.Config{
		Host: "https://test-cluster.example.com:6443",
	}
}

func (m *MockKARetrieverCommandForGet) CompleteRetriever() error {
	return m.completeError
}

// NewTestGetOptions creates GetOptions with mocks for testing
func NewTestGetOptions(mockCLI KARetrieverCommand) *GetOptions {
	outputFormat := ""

	return &GetOptions{
		KARetrieverCommand: mockCLI,
		OutputFormat:       &outputFormat,
		JSONYamlPrintFlags: genericclioptions.NewJSONYamlPrintFlags(),
		IOStreams: genericiooptions.IOStreams{
			In:     nil,
			Out:    nil,
			ErrOut: nil,
		},
		InCluster: true,
		Archived:  true,
	}
}

func TestGetComplete(t *testing.T) {
	testCases := []struct {
		name            string
		allNamespaces   bool
		labelSelector   string
		args            []string
		flags           []string // Command line flags to set
		resourceInfo    *ResourceInfo
		expectedApiPath string
		mockError       error
		expectError     bool
		errorContains   string
		expectInCluster bool
		expectArchived  bool
	}{
		{
			name:          "core resource",
			allNamespaces: false,
			args:          []string{"pods"},
			resourceInfo: &ResourceInfo{
				Resource: "pods", Version: "v1", Group: "", GroupVersion: "v1", Kind: "Pod", Namespaced: true,
			},
			expectedApiPath: "/api/v1/namespaces/default/pods",
			expectInCluster: true,
			expectArchived:  true,
		},
		{
			name:          "non-core resource with version and group",
			allNamespaces: false,
			args:          []string{"jobs.v1.batch"},
			resourceInfo: &ResourceInfo{
				Resource: "jobs", Version: "v1", Group: "batch", GroupVersion: "batch/v1", Kind: "Job", Namespaced: true,
			},
			expectedApiPath: "/apis/batch/v1/namespaces/default/jobs",
			expectInCluster: true,
			expectArchived:  true,
		},
		{
			name:          "non-core resource with group only",
			allNamespaces: false,
			args:          []string{"deployments.apps"},
			resourceInfo: &ResourceInfo{
				Resource: "deployments", Version: "v1", Group: "apps", GroupVersion: "apps/v1", Kind: "Deployment", Namespaced: true,
			},
			expectedApiPath: "/apis/apps/v1/namespaces/default/deployments",
			expectInCluster: true,
			expectArchived:  true,
		},
		{
			name:          "short name resource",
			allNamespaces: false,
			args:          []string{"deploy"},
			resourceInfo: &ResourceInfo{
				Resource: "deployments", Version: "v1", Group: "apps", GroupVersion: "apps/v1", Kind: "Deployment", Namespaced: true,
			},
			expectedApiPath: "/apis/apps/v1/namespaces/default/deployments", // Should use actual resource name
			expectInCluster: true,
			expectArchived:  true,
		},
		{
			name:          "singular name resource",
			allNamespaces: false,
			args:          []string{"pod"},
			resourceInfo: &ResourceInfo{
				Resource: "pods", Version: "v1", Group: "", GroupVersion: "v1", Kind: "Pod", Namespaced: true,
			},
			expectedApiPath: "/api/v1/namespaces/default/pods", // Should use actual resource name
			expectInCluster: true,
			expectArchived:  true,
		},
		{
			name:          "all namespaces",
			allNamespaces: true,
			args:          []string{"pods"},
			resourceInfo: &ResourceInfo{
				Resource: "pods", Version: "v1", Group: "", GroupVersion: "v1", Kind: "Pod", Namespaced: true,
			},
			expectedApiPath: "/api/v1/pods",
			expectInCluster: true,
			expectArchived:  true,
		},
		{
			name:          "core resource with name",
			allNamespaces: false,
			args:          []string{"pods", "my-pod"},
			resourceInfo: &ResourceInfo{
				Resource: "pods", Version: "v1", Group: "", GroupVersion: "v1", Kind: "Pod", Namespaced: true,
			},
			expectedApiPath: "/api/v1/namespaces/default/pods/my-pod",
			expectInCluster: true,
			expectArchived:  true,
		},
		{
			name:          "non-core resource with name",
			allNamespaces: false,
			args:          []string{"deployments.apps", "my-deployment"},
			resourceInfo: &ResourceInfo{
				Resource: "deployments", Version: "v1", Group: "apps", GroupVersion: "apps/v1", Kind: "Deployment", Namespaced: true,
			},
			expectedApiPath: "/apis/apps/v1/namespaces/default/deployments/my-deployment",
			expectInCluster: true,
			expectArchived:  true,
		},
		{
			name:          "all namespaces with name (should still include name)",
			allNamespaces: true,
			args:          []string{"pods", "my-pod"},
			resourceInfo: &ResourceInfo{
				Resource: "pods", Version: "v1", Group: "", GroupVersion: "v1", Kind: "Pod", Namespaced: true,
			},
			expectedApiPath: "/api/v1/pods/my-pod",
			expectInCluster: true,
			expectArchived:  true,
		},
		{
			name:          "with label selector",
			allNamespaces: false,
			labelSelector: "app=nginx",
			args:          []string{"pods"},
			resourceInfo: &ResourceInfo{
				Resource: "pods", Version: "v1", Group: "", GroupVersion: "v1", Kind: "Pod", Namespaced: true,
			},
			expectedApiPath: "/api/v1/namespaces/default/pods?labelSelector=app%3Dnginx",
			expectInCluster: true,
			expectArchived:  true,
		},
		{
			name:            "name and label selector together",
			labelSelector:   "app=nginx",
			args:            []string{"pods", "my-pod"},
			expectError:     true,
			errorContains:   "cannot specify both a resource name and a label selector",
			expectInCluster: true,
			expectArchived:  true,
		},
		{
			name:  "in-cluster true, archived false - valid",
			args:  []string{"pods"},
			flags: []string{"--in-cluster"},
			resourceInfo: &ResourceInfo{
				Resource: "pods", Version: "v1", Group: "", GroupVersion: "v1", Kind: "Pod", Namespaced: true,
			},
			expectedApiPath: "/api/v1/namespaces/default/pods",
			expectInCluster: true,
			expectArchived:  false,
		},
		{
			name:  "in-cluster false, archived true - valid",
			args:  []string{"pods"},
			flags: []string{"--archived"},
			resourceInfo: &ResourceInfo{
				Resource: "pods", Version: "v1", Group: "", GroupVersion: "v1", Kind: "Pod", Namespaced: true,
			},
			expectedApiPath: "/api/v1/namespaces/default/pods",
			expectInCluster: false,
			expectArchived:  true,
		},
		{
			name:            "both flags false - invalid",
			args:            []string{"pods"},
			flags:           []string{"--in-cluster=false", "--archived=false"},
			expectError:     true,
			errorContains:   "at least one of --in-cluster or --archived must be true",
			expectInCluster: false,
			expectArchived:  false,
		},
		{
			name:            "complete error",
			args:            []string{"pods"},
			mockError:       fmt.Errorf("mock complete failed"),
			expectError:     true,
			errorContains:   "mock complete failed",
			expectInCluster: true,
			expectArchived:  true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockCli := NewMockKARetrieverCommandForGet(tc.mockError, tc.resourceInfo)
			options := NewTestGetOptions(mockCli)
			options.AllNamespaces = tc.allNamespaces
			options.LabelSelector = tc.labelSelector

			// Create a cobra command to properly simulate flag parsing
			cmd := &cobra.Command{}
			cmd.Flags().BoolVar(&options.InCluster, "in-cluster", true, "")
			cmd.Flags().BoolVar(&options.Archived, "archived", true, "")

			// Parse the flags if any are provided
			if len(tc.flags) > 0 {
				err := cmd.Flags().Parse(tc.flags)
				require.NoError(t, err, "Failed to parse test flags")
			}

			err := options.Complete(cmd.Flags(), tc.args)

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

			// Check flag values after Complete is called (whether error or not)
			assert.Equal(t, tc.expectInCluster, options.InCluster, "InCluster flag mismatch")
			assert.Equal(t, tc.expectArchived, options.Archived, "Archived flag mismatch")
		})
	}
}

func TestRun(t *testing.T) {
	// Pre-calculate timestamp for consistent testing
	timestamp := time.Now().Add(-5 * time.Minute).Format(time.RFC3339)
	staticTimestamp := "2025-07-08T09:54:00Z"

	// Pre-generate common responses
	emptyPodList := `{"apiVersion": "v1", "kind": "PodList", "metadata": {"resourceVersion": ""}, "items": []}`
	testPod1 := createPodListJSON("test-pod-1", timestamp, "", "")
	archivedPod1 := createPodListJSON("archived-pod-1", timestamp, "", "")
	clusterOnlyPod := createPodListJSON("cluster-only-pod", timestamp, "", "")
	archiveOnlyPod := createPodListJSON("archive-only-pod", timestamp, "", "")
	duplicateUID := "duplicate-pod-uid-12345678-1234-1234-1234-123456789abc"
	duplicatePodK8s := createPodListJSON("duplicate-pod", timestamp, "Running", duplicateUID)
	duplicatePodArchive := createPodListJSON("duplicate-pod", timestamp, "Succeeded", duplicateUID)
	invalidJSON := `invalid json`

	testCases := []struct {
		name               string
		k8sResponse        string
		k9eResponse        string
		k8sError           *APIError
		k9eError           *APIError
		outputFormat       string
		expectError        bool
		errorContains      string
		expectedOutputFile string
		needsNormalization bool
		flags              []string
		allNamespaces      bool
	}{
		{
			name:               "table output with availability columns",
			k8sResponse:        testPod1,
			k9eResponse:        archivedPod1,
			outputFormat:       "",
			expectedOutputFile: "expected_table_output.txt",
		},
		{
			name:               "JSON output",
			k8sResponse:        createPodListJSON("test-pod-1", staticTimestamp, "", ""),
			k9eResponse:        createPodListJSON("archived-pod-1", staticTimestamp, "", ""),
			outputFormat:       "json",
			expectedOutputFile: "expected_json_output.json",
			needsNormalization: true,
		},
		{
			name:               "YAML output",
			k8sResponse:        createPodListJSON("test-pod-1", staticTimestamp, "", ""),
			k9eResponse:        createPodListJSON("archived-pod-1", staticTimestamp, "", ""),
			outputFormat:       "yaml",
			expectedOutputFile: "expected_yaml_output.yaml",
			needsNormalization: true,
		},
		{
			name:               "deduplication - live cluster priority with availability",
			k8sResponse:        duplicatePodK8s,
			k9eResponse:        duplicatePodArchive,
			outputFormat:       "",
			expectedOutputFile: "expected_deduplication_output.txt",
		},
		{
			name:               "only in cluster",
			k8sResponse:        clusterOnlyPod,
			k9eResponse:        emptyPodList,
			outputFormat:       "",
			expectedOutputFile: "expected_only_in_cluster.txt",
		},
		{
			name:               "only in archive",
			k8sResponse:        emptyPodList,
			k9eResponse:        archiveOnlyPod,
			outputFormat:       "",
			expectedOutputFile: "expected_only_in_archive.txt",
		},
		{
			name:               "in-cluster flag only",
			k8sResponse:        clusterOnlyPod,
			k9eResponse:        emptyPodList,
			outputFormat:       "",
			expectedOutputFile: "expected_only_in_cluster.txt",
			flags:              []string{"--in-cluster=true"},
		},
		{
			name:               "archived flag only",
			k8sResponse:        emptyPodList,
			k9eResponse:        archiveOnlyPod,
			outputFormat:       "",
			expectedOutputFile: "expected_only_in_archive.txt",
			flags:              []string{"--archived=true"},
		},
		{
			name:          "no resources found in namespace",
			k8sResponse:   emptyPodList,
			k9eResponse:   emptyPodList,
			expectError:   true,
			errorContains: "no resources found in default namespace",
		},
		{
			name:          "no resources found all namespaces",
			k8sResponse:   emptyPodList,
			k9eResponse:   emptyPodList,
			expectError:   true,
			errorContains: "no resources found",
			allNamespaces: true,
		},
		{
			name: "API error",
			k8sError: &APIError{
				StatusCode: 500,
				URL:        "mock-k8s-url",
				Message:    "connection failed",
				Body:       "",
			},
			k9eError: &APIError{
				StatusCode: 500,
				URL:        "mock-k9e-url",
				Message:    "network timeout",
				Body:       "",
			},
			expectError:   true,
			errorContains: "connection failed",
		},
		{
			name:          "invalid output format",
			k8sResponse:   testPod1,
			k9eResponse:   archivedPod1,
			outputFormat:  "invalid",
			expectError:   true,
			errorContains: "unable to match a printer suitable for the output format",
		},
		{
			name:          "invalid JSON response",
			k8sResponse:   invalidJSON,
			k9eResponse:   invalidJSON,
			outputFormat:  "",
			expectError:   true,
			errorContains: "error parsing resources from the cluster",
		},
		{
			name: "both APIs not found",
			k8sError: &APIError{
				StatusCode: 404,
				URL:        "https://k8s/api/v1/pods",
				Message:    "unable to get 'https://k8s/api/v1/pods': not found",
				Body:       "",
			},
			k9eError: &APIError{
				StatusCode: 404,
				URL:        "https://ka/api/v1/pods",
				Message:    "unable to get 'https://ka/api/v1/pods': not found",
				Body:       "",
			},
			expectError:   true,
			errorContains: "no resources found in Kubernetes or KubeArchive",
		},
		{
			name: "kubernetes forbidden, kubearchive not found",
			k8sError: &APIError{
				StatusCode: 403,
				URL:        "https://k8s/api/v1/clusterrolebindings",
				Message:    "clusterrolebindings.rbac.authorization.k8s.io is forbidden: User \"manon\" cannot list resource \"clusterrolebindings\" in API group \"rbac.authorization.k8s.io\" at the cluster scope",
				Body:       "",
			},
			k9eError: &APIError{
				StatusCode: 404,
				URL:        "https://ka/api/v1/clusterrolebindings",
				Message:    "unable to get 'https://ka/api/v1/clusterrolebindings': not found",
				Body:       "",
			},
			expectError:   true,
			errorContains: "clusterrolebindings.rbac.authorization.k8s.io is forbidden: User \"manon\" cannot list resource \"clusterrolebindings\"",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockCLI := &MockKARetrieverCommandForGet{
				k8sResponse:    tc.k8sResponse,
				k9eResponse:    tc.k9eResponse,
				k8sError:       tc.k8sError,
				k9eError:       tc.k9eError,
				namespaceValue: "default",
			}

			opts := NewTestGetOptions(mockCLI)
			opts.APIPath = "/api/v1/pods"
			opts.OutputFormat = &tc.outputFormat
			opts.AllNamespaces = tc.allNamespaces

			// Set flags if specified in test case
			if len(tc.flags) > 0 {
				// Create a cobra command to properly simulate flag parsing
				cmd := &cobra.Command{}
				cmd.Flags().BoolVar(&opts.InCluster, "in-cluster", true, "")
				cmd.Flags().BoolVar(&opts.Archived, "archived", true, "")

				err := cmd.Flags().Parse(tc.flags)
				require.NoError(t, err, "Failed to parse test flags")
			}

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
