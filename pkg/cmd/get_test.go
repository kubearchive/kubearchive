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

// MockKACLICommand implements the KACLICommand interface for testing
type MockKACLICommand struct {
	k8sResponse      string
	k9eResponse      string
	k8sError         *config.APIError
	k9eError         *config.APIError
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

func (m *MockKACLICommand) GetFromAPI(api config.API, _ string) ([]byte, *config.APIError) {
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
		return nil, &config.APIError{StatusCode: 500, URL: "unknown-api", Message: "unknown API type", Body: ""}
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
		resourceInfo    *config.ResourceInfo
		expectedApiPath string
		mockError       error
		expectError     bool
		errorContains   string
		inCluster       bool
		archived        bool
		setInCluster    bool
		setArchived     bool
	}{
		{
			name:          "core resource",
			allNamespaces: false,
			args:          []string{"pods"},
			resourceInfo: &config.ResourceInfo{
				Resource: "pods", Version: "v1", Group: "", GroupVersion: "v1", Kind: "Pod", Namespaced: true,
			},
			expectedApiPath: "/api/v1/namespaces/default/pods",
		},
		{
			name:          "non-core resource with version and group",
			allNamespaces: false,
			args:          []string{"jobs.v1.batch"},
			resourceInfo: &config.ResourceInfo{
				Resource: "jobs", Version: "v1", Group: "batch", GroupVersion: "batch/v1", Kind: "Job", Namespaced: true,
			},
			expectedApiPath: "/apis/batch/v1/namespaces/default/jobs",
		},
		{
			name:          "non-core resource with group only",
			allNamespaces: false,
			args:          []string{"deployments.apps"},
			resourceInfo: &config.ResourceInfo{
				Resource: "deployments", Version: "v1", Group: "apps", GroupVersion: "apps/v1", Kind: "Deployment", Namespaced: true,
			},
			expectedApiPath: "/apis/apps/v1/namespaces/default/deployments",
		},
		{
			name:          "short name resource",
			allNamespaces: false,
			args:          []string{"deploy"},
			resourceInfo: &config.ResourceInfo{
				Resource: "deployments", Version: "v1", Group: "apps", GroupVersion: "apps/v1", Kind: "Deployment", Namespaced: true,
			},
			expectedApiPath: "/apis/apps/v1/namespaces/default/deployments", // Should use actual resource name
		},
		{
			name:          "singular name resource",
			allNamespaces: false,
			args:          []string{"pod"},
			resourceInfo: &config.ResourceInfo{
				Resource: "pods", Version: "v1", Group: "", GroupVersion: "v1", Kind: "Pod", Namespaced: true,
			},
			expectedApiPath: "/api/v1/namespaces/default/pods", // Should use actual resource name
		},
		{
			name:          "all namespaces",
			allNamespaces: true,
			args:          []string{"pods"},
			resourceInfo: &config.ResourceInfo{
				Resource: "pods", Version: "v1", Group: "", GroupVersion: "v1", Kind: "Pod", Namespaced: true,
			},
			expectedApiPath: "/api/v1/pods",
		},
		{
			name:          "core resource with name",
			allNamespaces: false,
			args:          []string{"pods", "my-pod"},
			resourceInfo: &config.ResourceInfo{
				Resource: "pods", Version: "v1", Group: "", GroupVersion: "v1", Kind: "Pod", Namespaced: true,
			},
			expectedApiPath: "/api/v1/namespaces/default/pods/my-pod",
		},
		{
			name:          "non-core resource with name",
			allNamespaces: false,
			args:          []string{"deployments.apps", "my-deployment"},
			resourceInfo: &config.ResourceInfo{
				Resource: "deployments", Version: "v1", Group: "apps", GroupVersion: "apps/v1", Kind: "Deployment", Namespaced: true,
			},
			expectedApiPath: "/apis/apps/v1/namespaces/default/deployments/my-deployment",
		},
		{
			name:          "all namespaces with name (should still include name)",
			allNamespaces: true,
			args:          []string{"pods", "my-pod"},
			resourceInfo: &config.ResourceInfo{
				Resource: "pods", Version: "v1", Group: "", GroupVersion: "v1", Kind: "Pod", Namespaced: true,
			},
			expectedApiPath: "/api/v1/pods/my-pod",
		},
		{
			name:          "with label selector",
			allNamespaces: false,
			labelSelector: "app=nginx",
			args:          []string{"pods"},
			resourceInfo: &config.ResourceInfo{
				Resource: "pods", Version: "v1", Group: "", GroupVersion: "v1", Kind: "Pod", Namespaced: true,
			},
			expectedApiPath: "/api/v1/namespaces/default/pods?labelSelector=app%3Dnginx",
		},
		{
			name:          "name and label selector together",
			labelSelector: "app=nginx",
			args:          []string{"pods", "my-pod"},
			expectError:   true,
			errorContains: "cannot specify both a resource name and a label selector",
		},
		{
			name: "both flags true - valid",
			args: []string{"pods"},
			resourceInfo: &config.ResourceInfo{
				Resource: "pods", Version: "v1", Group: "", GroupVersion: "v1", Kind: "Pod", Namespaced: true,
			},
			inCluster:       true,
			archived:        true,
			setInCluster:    true,
			setArchived:     true,
			expectedApiPath: "/api/v1/namespaces/default/pods",
		},
		{
			name: "in-cluster true, archived false - valid",
			args: []string{"pods"},
			resourceInfo: &config.ResourceInfo{
				Resource: "pods", Version: "v1", Group: "", GroupVersion: "v1", Kind: "Pod", Namespaced: true,
			},
			inCluster:       true,
			archived:        false,
			setInCluster:    true,
			setArchived:     true,
			expectedApiPath: "/api/v1/namespaces/default/pods",
		},
		{
			name: "in-cluster false, archived true - valid",
			args: []string{"pods"},
			resourceInfo: &config.ResourceInfo{
				Resource: "pods", Version: "v1", Group: "", GroupVersion: "v1", Kind: "Pod", Namespaced: true,
			},
			inCluster:       false,
			archived:        true,
			setInCluster:    true,
			setArchived:     true,
			expectedApiPath: "/api/v1/namespaces/default/pods",
		},
		{
			name:          "both flags false - invalid",
			args:          []string{"pods"},
			inCluster:     false,
			archived:      false,
			setInCluster:  true,
			setArchived:   true,
			expectError:   true,
			errorContains: "at least one of --in-cluster or --archived must be true",
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
			var options *GetOptions
			mockCli := NewMockKACLICommand(tc.mockError, tc.resourceInfo)
			options = NewTestGetOptions(mockCli)
			options.AllNamespaces = tc.allNamespaces
			options.LabelSelector = tc.labelSelector

			// Set flags if specified in test case
			if tc.setInCluster {
				options.InCluster = tc.inCluster
			}
			if tc.setArchived {
				options.Archived = tc.archived
			}

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
		k8sError           *config.APIError
		k9eError           *config.APIError
		outputFormat       string
		expectError        bool
		errorContains      string
		expectedOutputFile string
		needsNormalization bool
		inCluster          bool
		archived           bool
		setInCluster       bool
		setArchived        bool
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
			inCluster:          true,
			archived:           false,
			setInCluster:       true,
			setArchived:        true,
		},
		{
			name:               "archived flag only",
			k8sResponse:        emptyPodList,
			k9eResponse:        archiveOnlyPod,
			outputFormat:       "",
			expectedOutputFile: "expected_only_in_archive.txt",
			inCluster:          false,
			archived:           true,
			setInCluster:       true,
			setArchived:        true,
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
			k8sError: &config.APIError{
				StatusCode: 500,
				URL:        "mock-k8s-url",
				Message:    "connection failed",
				Body:       "",
			},
			k9eError: &config.APIError{
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
			k8sError: &config.APIError{
				StatusCode: 404,
				URL:        "https://k8s/api/v1/pods",
				Message:    "unable to get 'https://k8s/api/v1/pods': not found",
				Body:       "",
			},
			k9eError: &config.APIError{
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
			k8sError: &config.APIError{
				StatusCode: 403,
				URL:        "https://k8s/api/v1/clusterrolebindings",
				Message:    "clusterrolebindings.rbac.authorization.k8s.io is forbidden: User \"manon\" cannot list resource \"clusterrolebindings\" in API group \"rbac.authorization.k8s.io\" at the cluster scope",
				Body:       "",
			},
			k9eError: &config.APIError{
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
			mockCLI := &MockKACLICommand{
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
			if tc.setInCluster {
				opts.InCluster = tc.inCluster
			}
			if tc.setArchived {
				opts.Archived = tc.archived
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
