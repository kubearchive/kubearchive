// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0
package cmd

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/yaml"
)

func TestCompleteAPI(t *testing.T) {
	testCases := []struct {
		name            string
		namespace       string
		args            []string
		expectedApiPath string
		isCore          bool
		output          string
	}{
		{
			name:            "core",
			args:            []string{"v1", "pods"},
			expectedApiPath: "/api/v1/pods",
			isCore:          true,
		},
		{
			name:            "non-core",
			args:            []string{"batch/v1", "jobs"},
			expectedApiPath: "/apis/batch/v1/jobs",
			isCore:          false,
		},
		{
			name:            "core namespaced",
			namespace:       "test",
			args:            []string{"v1", "pods"},
			expectedApiPath: "/api/v1/namespaces/test/pods",
			isCore:          true,
		},
		{
			name:            "non-core namespaced",
			namespace:       "test",
			args:            []string{"batch/v1", "jobs"},
			expectedApiPath: "/apis/batch/v1/namespaces/test/jobs",
			isCore:          false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			options := NewGetOptions()
			options.kubeFlags.Namespace = &tc.namespace

			err := options.Complete(tc.args)

			assert.NoError(t, err)
			assert.Equal(t, tc.expectedApiPath, options.APIPath)
			assert.Equal(t, tc.isCore, options.IsCoreResource)
			assert.Equal(t, tc.args[0], options.GroupVersion)
			assert.Equal(t, tc.args[1], options.Resource)
			assert.NotNil(t, options.RESTConfig)
		})
	}

}

// Helper function to load expected content from testdata files
func loadExpectedOutput(t *testing.T, filename string) string {
	t.Helper()
	path := filepath.Join("testdata", filename)
	content, err := os.ReadFile(path)
	require.NoError(t, err, "Failed to read expected output file: %s", path)
	return string(content)
}

// Helper function to normalize JSON for comparison
func normalizeJSON(t *testing.T, jsonStr string) string {
	t.Helper()
	var obj interface{}
	err := json.Unmarshal([]byte(jsonStr), &obj)
	require.NoError(t, err)
	normalized, err := json.MarshalIndent(obj, "", "    ")
	require.NoError(t, err)
	return string(normalized)
}

// Helper function to normalize YAML for comparison
func normalizeYAML(t *testing.T, yamlStr string) string {
	t.Helper()
	var obj interface{}
	err := yaml.Unmarshal([]byte(yamlStr), &obj)
	require.NoError(t, err)
	normalized, err := yaml.Marshal(obj)
	require.NoError(t, err)
	return string(normalized)
}

// Helper function to create a mock server that returns a pod with the specified name and timestamp
// If podName is empty, returns an empty list
// If timestamp is empty, no timestamp is set (for empty lists)
func createMockServer(t *testing.T, podName string, timestamp string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"kind":       "List",
			"apiVersion": "v1",
			"items":      []map[string]interface{}{},
		}

		// Add a pod only if podName is provided
		if podName != "" {
			pod := map[string]interface{}{
				"kind":       "Pod",
				"apiVersion": "v1",
				"metadata": map[string]interface{}{
					"name":      podName,
					"namespace": "default",
				},
			}

			// Add timestamp if provided
			if timestamp != "" {
				pod["metadata"].(map[string]interface{})["creationTimestamp"] = timestamp
			}

			response["items"] = []map[string]interface{}{pod}
		}

		err := json.NewEncoder(w).Encode(response)
		if err != nil {
			return
		}
	}))
}

// Helper function to create a mock server that returns an error
func createMockErrorServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, err := w.Write([]byte("server error"))
		if err != nil {
			return
		}
	}))
}

// Helper function to create test options with two separate mock servers
func createTestOptionsWithTwoServers(t *testing.T, kubernetesServerURL, kubeArchiveServerURL string) *GetOptions {
	t.Helper()
	options := NewGetOptions()
	options.RESTConfig = &rest.Config{
		Host: kubernetesServerURL,
	}
	options.KubeArchiveHost = kubeArchiveServerURL
	options.AllNamespaces = true
	options.Resource = "pods"
	options.GroupVersion = "v1"
	options.IsCoreResource = true
	options.APIPath = "/api/v1/pods"

	// Set up bearer token to avoid a nil pointer in getKubeArchiveResources
	token := "test-token"
	options.kubeFlags.BearerToken = &token

	return options
}

// setupTestEnvironmentCommon handles common setup: creates CA file, creates test options, returns cleanup
func setupTestEnvironmentCommon(t *testing.T, mockKubernetesServer, mockKubeArchiveServer *httptest.Server) (*GetOptions, func()) {
	t.Helper()

	// Create CA certificate file
	err := os.WriteFile("ca.crt", []byte(""), 0600)
	require.NoError(t, err)

	// Create test options
	options := createTestOptionsWithTwoServers(t, mockKubernetesServer.URL, mockKubeArchiveServer.URL)

	// Return cleanup function
	cleanup := func() {
		os.Remove("ca.crt")
		mockKubernetesServer.Close()
		mockKubeArchiveServer.Close()
	}

	return options, cleanup
}

// setupTestEnvironment handles test setup: creates CA file, mock servers, and test options
// Returns cleanup function that should be deferred
func setupTestEnvironment(t *testing.T, kubernetesPodName, kubeArchivePodName, kubernetesTimestamp, kubeArchiveTimestamp string) (*GetOptions, func()) {
	t.Helper()

	// Create mock servers
	mockKubernetesServer := createMockServer(t, kubernetesPodName, kubernetesTimestamp)
	mockKubeArchiveServer := createMockServer(t, kubeArchivePodName, kubeArchiveTimestamp)

	return setupTestEnvironmentCommon(t, mockKubernetesServer, mockKubeArchiveServer)
}

// setupTestEnvironmentWithErrorServers creates test environment with error servers for testing failure scenarios
func setupTestEnvironmentWithErrorServers(t *testing.T, kubernetesError, kubeArchiveError bool) (*GetOptions, func()) {
	t.Helper()

	// Create appropriate servers based on error flags
	var mockKubernetesServer *httptest.Server
	if kubernetesError {
		mockKubernetesServer = createMockErrorServer(t)
	} else {
		mockKubernetesServer = createMockServer(t, "", "") // Empty list, no timestamp
	}

	var mockKubeArchiveServer *httptest.Server
	if kubeArchiveError {
		mockKubeArchiveServer = createMockErrorServer(t)
	} else {
		mockKubeArchiveServer = createMockServer(t, "", "") // Empty list, no timestamp
	}

	return setupTestEnvironmentCommon(t, mockKubernetesServer, mockKubeArchiveServer)
}

func TestRunOutputFormats(t *testing.T) {
	// Calculate the dynamic timestamp for table test (5 minutes ago)
	dynamicTimestamp := time.Now().Add(-5 * time.Minute).Format(time.RFC3339)

	testCases := []struct {
		name                 string
		outputFormat         string
		expectedOutputFile   string
		needsNormalization   bool
		kubernetesTimestamp  string
		kubeArchiveTimestamp string
	}{
		{
			name:                 "table",
			outputFormat:         "",
			expectedOutputFile:   "expected_table_output.txt",
			needsNormalization:   false,
			kubernetesTimestamp:  dynamicTimestamp,
			kubeArchiveTimestamp: dynamicTimestamp,
		},
		{
			name:                 "json",
			outputFormat:         "json",
			expectedOutputFile:   "expected_json_output.json",
			needsNormalization:   true,
			kubernetesTimestamp:  "2025-07-08T09:54:00Z",
			kubeArchiveTimestamp: "2025-07-08T09:54:00Z",
		},
		{
			name:                 "yaml",
			outputFormat:         "yaml",
			expectedOutputFile:   "expected_yaml_output.yaml",
			needsNormalization:   true,
			kubernetesTimestamp:  "2025-07-08T09:54:00Z",
			kubeArchiveTimestamp: "2025-07-08T09:54:00Z",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			options, cleanup := setupTestEnvironment(t, "test-pod-1", "archived-pod-1", tc.kubernetesTimestamp, tc.kubeArchiveTimestamp)
			defer cleanup()

			// Set output format if specified
			if tc.outputFormat != "" {
				options.OutputFormat = &tc.outputFormat
			}

			// Capture output
			var buf bytes.Buffer
			options.Out = &buf

			// Call the Run function directly
			err := options.Run()
			require.NoError(t, err)

			// Compare with expected output
			expectedOutput := loadExpectedOutput(t, tc.expectedOutputFile)
			actualOutput := buf.String()

			if tc.needsNormalization {
				// Normalize both expected and actual output for JSON/YAML
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
				// Direct comparison for table output
				assert.Equal(t, expectedOutput, actualOutput)
			}
		})
	}
}

func TestRunErrorHandling(t *testing.T) {
	testCases := []struct {
		name                string
		kubernetesError     bool
		kubeArchiveError    bool
		expectedErrorString string
	}{
		{
			name:                "kubernetes server error",
			kubernetesError:     true,
			kubeArchiveError:    false,
			expectedErrorString: "error retrieving resources from the cluster",
		},
		{
			name:                "kubearchive server error",
			kubernetesError:     false,
			kubeArchiveError:    true,
			expectedErrorString: "error retrieving resources from the KubeArchive API",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			options, cleanup := setupTestEnvironmentWithErrorServers(t, tc.kubernetesError, tc.kubeArchiveError)
			defer cleanup()

			// Capture output
			var buf bytes.Buffer
			options.Out = &buf

			// Call the Run function directly and expect an error
			err := options.Run()
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.expectedErrorString)
		})
	}
}
