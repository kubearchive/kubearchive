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

	"github.com/kubearchive/kubearchive/pkg/cmd/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/yaml"
)

func TestGetComplete(t *testing.T) {
	testCases := []struct {
		name            string
		namespace       string
		allNamespaces   bool
		args            []string
		expectedApiPath string
		output          string
	}{
		{
			name:            "core",
			args:            []string{"v1", "pods"},
			expectedApiPath: "/api/v1/namespaces/default/pods",
		},
		{
			name:            "non-core",
			args:            []string{"batch/v1", "jobs"},
			expectedApiPath: "/apis/batch/v1/namespaces/default/jobs",
		},
		{
			name:            "core namespaced",
			namespace:       "test",
			allNamespaces:   false,
			args:            []string{"v1", "pods"},
			expectedApiPath: "/api/v1/namespaces/test/pods",
		},
		{
			name:            "non-core namespaced",
			namespace:       "test",
			allNamespaces:   false,
			args:            []string{"batch/v1", "jobs"},
			expectedApiPath: "/apis/batch/v1/namespaces/test/jobs",
		},
		{
			name:            "core with all-namespaces flag",
			namespace:       "test",
			allNamespaces:   true,
			args:            []string{"v1", "pods"},
			expectedApiPath: "/api/v1/pods",
		},
		{
			name:            "non-core with all-namespaces flag",
			namespace:       "test",
			allNamespaces:   true,
			args:            []string{"batch/v1", "jobs"},
			expectedApiPath: "/apis/batch/v1/jobs",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			options := NewGetOptions()

			if tc.namespace != "" {
				options.KubeFlags.Namespace = &tc.namespace
			}

			options.AllNamespaces = tc.allNamespaces

			err := options.Complete(tc.args)

			assert.NoError(t, err)
			assert.Equal(t, tc.expectedApiPath, options.APIPath)
			assert.Equal(t, tc.args[0], options.GroupVersion)
			assert.Equal(t, tc.args[1], options.Resource)
			assert.NotNil(t, options.RESTConfig)
		})
	}
}

func TestCompleteAPINamespaceFallback(t *testing.T) {
	testCases := []struct {
		name            string
		kubeconfigNs    string
		allNamespaces   bool
		args            []string
		expectedApiPath string
		isCore          bool
	}{
		{
			name:            "core with kubeconfig namespace",
			kubeconfigNs:    "default",
			allNamespaces:   false,
			args:            []string{"v1", "pods"},
			expectedApiPath: "/api/v1/namespaces/default/pods",
			isCore:          true,
		},
		{
			name:            "non-core with kubeconfig namespace",
			kubeconfigNs:    "kube-system",
			allNamespaces:   false,
			args:            []string{"batch/v1", "jobs"},
			expectedApiPath: "/apis/batch/v1/namespaces/kube-system/jobs",
			isCore:          false,
		},
		{
			name:            "with all-namespaces flag ignores kubeconfig namespace",
			kubeconfigNs:    "default",
			allNamespaces:   true,
			args:            []string{"v1", "pods"},
			expectedApiPath: "/api/v1/pods",
			isCore:          true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			options := NewGetOptions()
			options.AllNamespaces = tc.allNamespaces

			if tc.kubeconfigNs != "" {
				options.KubeFlags.Namespace = &tc.kubeconfigNs
			}

			err := options.Complete(tc.args)

			assert.NoError(t, err)
			assert.Equal(t, tc.expectedApiPath, options.APIPath)
			assert.Equal(t, tc.args[0], options.GroupVersion)
			assert.Equal(t, tc.args[1], options.Resource)
			assert.NotNil(t, options.RESTConfig)
		})
	}
}

func loadExpectedOutput(t *testing.T, filename string) string {
	t.Helper()
	path := filepath.Join("testdata", filename)
	content, err := os.ReadFile(path)
	require.NoError(t, err, "Failed to read expected output file: %s", path)
	return string(content)
}

func normalizeJSON(t *testing.T, jsonStr string) string {
	t.Helper()
	var obj interface{}
	err := json.Unmarshal([]byte(jsonStr), &obj)
	require.NoError(t, err)
	normalized, err := json.MarshalIndent(obj, "", "    ")
	require.NoError(t, err)
	return string(normalized)
}

func normalizeYAML(t *testing.T, yamlStr string) string {
	t.Helper()
	var obj interface{}
	err := yaml.Unmarshal([]byte(yamlStr), &obj)
	require.NoError(t, err)
	normalized, err := yaml.Marshal(obj)
	require.NoError(t, err)
	return string(normalized)
}

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

func createTestOptionsWithTwoServers(t *testing.T, kubernetesServerURL, kubeArchiveServerURL string) *GetOptions {
	t.Helper()
	options := NewGetOptions()
	options.RESTConfig = &rest.Config{
		Host: kubernetesServerURL,
	}
	options.Host = kubeArchiveServerURL
	options.AllNamespaces = true
	options.Resource = "pods"
	options.GroupVersion = "v1"
	options.APIPath = "/api/v1/pods"

	token := "test-token"
	options.KubeFlags.BearerToken = &token
	options.RESTConfig.BearerToken = token

	return options
}

func setupTestEnvironmentCommon(t *testing.T, mockKubernetesServer, mockKubeArchiveServer *httptest.Server) (*GetOptions, func()) {
	t.Helper()

	options := createTestOptionsWithTwoServers(t, mockKubernetesServer.URL, mockKubeArchiveServer.URL)

	cleanup := func() {
		mockKubernetesServer.Close()
		mockKubeArchiveServer.Close()
	}

	return options, cleanup
}

func setupTestEnvironment(t *testing.T, kubernetesPodName, kubeArchivePodName, kubernetesTimestamp, kubeArchiveTimestamp string) (*GetOptions, func()) {
	t.Helper()

	mockKubernetesServer := createMockServer(t, kubernetesPodName, kubernetesTimestamp)
	mockKubeArchiveServer := createMockServer(t, kubeArchivePodName, kubeArchiveTimestamp)

	return setupTestEnvironmentCommon(t, mockKubernetesServer, mockKubeArchiveServer)
}

func setupTestEnvironmentWithErrorServers(t *testing.T, kubernetesError, kubeArchiveError bool) (*GetOptions, func()) {
	t.Helper()

	var mockKubernetesServer *httptest.Server
	if kubernetesError {
		mockKubernetesServer = createMockErrorServer(t)
	} else {
		mockKubernetesServer = createMockServer(t, "", "")
	}

	var mockKubeArchiveServer *httptest.Server
	if kubeArchiveError {
		mockKubeArchiveServer = createMockErrorServer(t)
	} else {
		mockKubeArchiveServer = createMockServer(t, "", "")
	}

	return setupTestEnvironmentCommon(t, mockKubernetesServer, mockKubeArchiveServer)
}

func TestRunOutputFormats(t *testing.T) {
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

			if tc.outputFormat != "" {
				options.OutputFormat = &tc.outputFormat
			}

			var buf bytes.Buffer
			options.Out = &buf

			err := options.Run()
			require.NoError(t, err)

			expectedOutput := loadExpectedOutput(t, tc.expectedOutputFile)
			actualOutput := buf.String()

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

			var buf bytes.Buffer
			options.Out = &buf

			err := options.Run()
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.expectedErrorString)
		})
	}
}

func TestKubectlFlagsIntegration(t *testing.T) {
	opts := &GetOptions{
		ArchiveOptions: config.NewArchiveOptions(),
		RESTConfig: &rest.Config{
			Host: "https://test-cluster.com",
		},
	}

	caFile := "/tmp/test-ca.crt"
	insecure := true
	opts.KubeFlags.CAFile = &caFile
	opts.KubeFlags.Insecure = &insecure

	err := opts.ArchiveOptions.Complete(opts.RESTConfig)
	assert.NoError(t, err)

	assert.Equal(t, "", opts.CertificatePath)
	assert.Equal(t, false, opts.TLSInsecure)

	certData, err := opts.GetCertificateData()
	assert.NoError(t, err)
	assert.Nil(t, certData)
}
