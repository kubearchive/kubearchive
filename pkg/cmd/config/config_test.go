// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0
package config

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/rest"
)

// MockDiscoveryClient implements DiscoveryInterface for testing
type MockDiscoveryClient struct {
	serverGroups       *metav1.APIGroupList
	serverResources    map[string]*metav1.APIResourceList
	serverGroupsErr    error
	serverResourcesErr error
}

func (m *MockDiscoveryClient) ServerGroups() (*metav1.APIGroupList, error) {
	if m.serverGroupsErr != nil {
		return nil, m.serverGroupsErr
	}
	return m.serverGroups, nil
}

func (m *MockDiscoveryClient) ServerGroupsAndResources() ([]*metav1.APIGroup, []*metav1.APIResourceList, error) {
	if m.serverResourcesErr != nil {
		return nil, nil, m.serverResourcesErr
	}

	var groups []*metav1.APIGroup
	var resourceLists []*metav1.APIResourceList

	for _, resourceList := range m.serverResources {
		resourceLists = append(resourceLists, resourceList)
	}

	if m.serverGroups != nil {
		for _, group := range m.serverGroups.Groups {
			groups = append(groups, &group)
		}
	}

	return groups, resourceLists, nil
}

// newMockDiscoveryClient creates a mock discovery client with common test resources
func newMockDiscoveryClient() *MockDiscoveryClient {
	return &MockDiscoveryClient{
		serverGroups: &metav1.APIGroupList{
			Groups: []metav1.APIGroup{
				{
					Name: "apps",
					Versions: []metav1.GroupVersionForDiscovery{
						{GroupVersion: "apps/v1beta1", Version: "v1beta1"},
						{GroupVersion: "apps/v1beta2", Version: "v1beta2"},
						{GroupVersion: "apps/v1", Version: "v1"},
					},
					PreferredVersion: metav1.GroupVersionForDiscovery{
						GroupVersion: "apps/v1",
						Version:      "v1",
					},
				},
				{
					Name: "batch",
					Versions: []metav1.GroupVersionForDiscovery{
						{GroupVersion: "batch/v1", Version: "v1"},
					},
					PreferredVersion: metav1.GroupVersionForDiscovery{
						GroupVersion: "batch/v1",
						Version:      "v1",
					},
				},
			},
		},
		serverResources: map[string]*metav1.APIResourceList{
			"v1": {
				GroupVersion: "v1",
				APIResources: []metav1.APIResource{
					{
						Name:         "pods",
						SingularName: "pod",
						Namespaced:   true,
						Kind:         "Pod",
						ShortNames:   []string{"po"},
					},
					{
						Name:         "services",
						SingularName: "service",
						Namespaced:   true,
						Kind:         "Service",
						ShortNames:   []string{"svc"},
					},
				},
			},
			"apps/v1beta1": {
				GroupVersion: "apps/v1beta1",
				APIResources: []metav1.APIResource{
					{
						Name:         "deployments",
						SingularName: "deployment",
						Namespaced:   true,
						Kind:         "Deployment",
						ShortNames:   []string{"deploy"},
					},
				},
			},
			"apps/v1beta2": {
				GroupVersion: "apps/v1beta2",
				APIResources: []metav1.APIResource{
					{
						Name:         "deployments",
						SingularName: "deployment",
						Namespaced:   true,
						Kind:         "Deployment",
						ShortNames:   []string{"deploy"},
					},
				},
			},
			"apps/v1": {
				GroupVersion: "apps/v1",
				APIResources: []metav1.APIResource{
					{
						Name:         "deployments",
						SingularName: "deployment",
						Namespaced:   true,
						Kind:         "Deployment",
						ShortNames:   []string{"deploy"},
					},
				},
			},
			"batch/v1": {
				GroupVersion: "batch/v1",
				APIResources: []metav1.APIResource{
					{
						Name:         "jobs",
						SingularName: "job",
						Namespaced:   true,
						Kind:         "Job",
					},
				},
			},
		},
	}
}

// withMultipleEnv sets multiple environment variables for the duration of the test.
func withMultipleEnv(t *testing.T, envVars map[string]string, testFunc func()) {
	t.Helper()
	for k, v := range envVars {
		t.Setenv(k, v)
	}
	testFunc()
}

func TestNewKAOptions(t *testing.T) {
	testCases := []struct {
		name             string
		envVars          map[string]string
		expectedHost     string
		expectedCert     string
		expectedInsecure bool
	}{
		{
			name:             "defaults",
			envVars:          map[string]string{},
			expectedHost:     "https://localhost:8081",
			expectedCert:     "",
			expectedInsecure: false,
		},
		{
			name: "environment variables",
			envVars: map[string]string{
				"KUBECTL_PLUGIN_KA_HOST":         "https://env-host:7070",
				"KUBECTL_PLUGIN_KA_CERT_PATH":    "/path/to/env/cert.crt",
				"KUBECTL_PLUGIN_KA_TLS_INSECURE": "true",
			},
			expectedHost:     "https://env-host:7070",
			expectedCert:     "/path/to/env/cert.crt",
			expectedInsecure: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			withMultipleEnv(t, tc.envVars, func() {
				opts := NewKAOptions()
				assert.Equal(t, tc.expectedHost, opts.host)
				assert.Equal(t, tc.expectedCert, opts.certificatePath)
				assert.Equal(t, tc.expectedInsecure, opts.tlsInsecure)
			})
		})
	}
}

func TestAddFlagsAndPrecedence(t *testing.T) {
	testCases := []struct {
		name             string
		envVars          map[string]string
		flagValues       map[string]string
		expectedHost     string
		expectedCert     string
		expectedInsecure bool
	}{
		{
			name:             "flags only",
			envVars:          map[string]string{},
			flagValues:       map[string]string{"host": "https://flag-host:9090"},
			expectedHost:     "https://flag-host:9090",
			expectedCert:     "",
			expectedInsecure: false,
		},
		{
			name:             "env vars only",
			envVars:          map[string]string{"KUBECTL_PLUGIN_KA_HOST": "https://env-host:7070"},
			flagValues:       map[string]string{},
			expectedHost:     "https://env-host:7070",
			expectedCert:     "",
			expectedInsecure: false,
		},
		{
			name:             "flags override env vars",
			envVars:          map[string]string{"KUBECTL_PLUGIN_KA_HOST": "https://env-host:7070"},
			flagValues:       map[string]string{"host": "https://flag-host:9090"},
			expectedHost:     "https://flag-host:9090",
			expectedCert:     "",
			expectedInsecure: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			withMultipleEnv(t, tc.envVars, func() {
				opts := NewKAOptions()

				if len(tc.flagValues) > 0 {
					flags := pflag.NewFlagSet("test", pflag.ExitOnError)
					opts.AddFlags(flags)

					for flagName, flagValue := range tc.flagValues {
						flag := flags.Lookup(flagName)
						require.NotNil(t, flag)
						require.NoError(t, flag.Value.Set(flagValue))
					}
				}

				assert.Equal(t, tc.expectedHost, opts.host)
				assert.Equal(t, tc.expectedCert, opts.certificatePath)
				assert.Equal(t, tc.expectedInsecure, opts.tlsInsecure)
			})
		})
	}
}

func TestComplete(t *testing.T) {
	kubeconfigPath := filepath.Join("testdata", "kubeconfig.yaml")
	t.Setenv("KUBECONFIG", kubeconfigPath)

	testCases := []struct {
		name                string
		setup               func(opts *KAOptions)
		env                 map[string]string
		expectedK9eHost     string
		expectedK9eToken    string
		expectedK9eInsecure bool
		expectCertData      bool
		expectError         bool
		errorContains       string
	}{
		{
			name:                "default configuration",
			setup:               func(opts *KAOptions) {},
			env:                 map[string]string{},
			expectedK9eHost:     "https://localhost:8081",
			expectedK9eToken:    "k8s-config-token",
			expectedK9eInsecure: false,
			expectCertData:      false,
			expectError:         false,
		},
		{
			name: "token precedence - kubectl flag wins",
			setup: func(opts *KAOptions) {
				testToken := "kubectl-token" // #nosec G101 - this is a test token
				opts.kubeFlags.BearerToken = &testToken
			},
			env: map[string]string{
				"KUBECTL_PLUGIN_KA_TOKEN": "env-token",
			},
			expectedK9eHost:     "https://localhost:8081",
			expectedK9eToken:    "kubectl-token",
			expectedK9eInsecure: false,
			expectCertData:      false,
			expectError:         false,
		},
		{
			name:  "token from environment variable",
			setup: func(opts *KAOptions) {},
			env: map[string]string{
				"KUBECTL_PLUGIN_KA_TOKEN": "env-token",
			},
			expectedK9eHost:     "https://localhost:8081",
			expectedK9eToken:    "env-token",
			expectedK9eInsecure: false,
			expectCertData:      false,
			expectError:         false,
		},
		{
			name: "certificate path with TLS override",
			setup: func(opts *KAOptions) {
				opts.tlsInsecure = true
				opts.certificatePath = filepath.Join("testdata", "test-cert.crt")
			},
			env:                 map[string]string{},
			expectedK9eHost:     "https://localhost:8081",
			expectedK9eToken:    "k8s-config-token",
			expectedK9eInsecure: false, // Certificate overrides insecure setting
			expectCertData:      true,
			expectError:         false,
		},
		{
			name: "certificate error",
			setup: func(opts *KAOptions) {
				opts.certificatePath = "/nonexistent/cert.crt"
			},
			env:           map[string]string{},
			expectError:   true,
			errorContains: "failed to load certificate from path",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			withMultipleEnv(t, tc.env, func() {
				opts := NewKAOptions()
				if tc.setup != nil {
					tc.setup(opts)
				}

				err := opts.Complete()
				if tc.expectError {
					assert.Error(t, err)
					if tc.errorContains != "" {
						assert.Contains(t, err.Error(), tc.errorContains)
					}
					return
				}

				require.NoError(t, err)
				assert.NotNil(t, opts.K8sRESTConfig)
				assert.NotNil(t, opts.K9eRESTConfig)
				assert.Equal(t, tc.expectedK9eHost, opts.K9eRESTConfig.Host)
				assert.Equal(t, tc.expectedK9eToken, opts.K9eRESTConfig.BearerToken)
				assert.Equal(t, tc.expectedK9eInsecure, opts.K9eRESTConfig.Insecure)

				if tc.expectCertData {
					assert.NotNil(t, opts.K9eRESTConfig.CAData)
					assert.NotEmpty(t, opts.K9eRESTConfig.CAData)
				} else {
					assert.Nil(t, opts.K9eRESTConfig.CAData)
				}
			})
		})
	}
}

func TestGetNamespace(t *testing.T) {
	testCases := []struct {
		name          string
		useKubeconfig bool
		setup         func(opts *KAOptions)
		expectedNs    string
		expectError   bool
	}{
		{
			name: "namespace from flags",
			setup: func(opts *KAOptions) {
				ns := "test-namespace"
				opts.kubeFlags.Namespace = &ns
			},
			expectedNs:  "test-namespace",
			expectError: false,
		},
		{
			name:          "namespace from kubeconfig",
			useKubeconfig: true,
			setup:         func(opts *KAOptions) {},
			expectedNs:    "kubeconfig-namespace",
			expectError:   false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.useKubeconfig {
				kubeconfigPath := filepath.Join("testdata", "kubeconfig.yaml")
				t.Setenv("KUBECONFIG", kubeconfigPath)
			}

			opts := &KAOptions{
				kubeFlags: genericclioptions.NewConfigFlags(true),
			}
			if tc.setup != nil {
				tc.setup(opts)
			}

			ns, err := opts.GetNamespace()
			if tc.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expectedNs, ns)
			}
		})
	}
}

func TestGetFromAPI(t *testing.T) {
	testCases := []struct {
		name           string
		api            API
		path           string
		serverResponse func(w http.ResponseWriter, r *http.Request)
		expectedBody   string
		expectError    bool
		errorContains  string
		unreachable    bool
	}{
		{
			name: "successful request",
			api:  Kubernetes,
			path: "/api/v1/pods",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"kind":"PodList"}`))
			},
			expectedBody: `{"kind":"PodList"}`,
			expectError:  false,
		},
		{
			name: "unauthorized response",
			api:  KubeArchive,
			path: "/api/v1/secrets",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusUnauthorized)
			},
			expectError:   true,
			errorContains: "HTTP 401",
		},
		{
			name: "not found response",
			api:  Kubernetes,
			path: "/api/v1/nonexistent",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNotFound)
			},
			expectError:   true,
			errorContains: "HTTP 404",
		},
		{
			name: "server error with body",
			api:  KubeArchive,
			path: "/api/v1/error",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`Error details`))
			},
			expectError:   true,
			errorContains: "Error details (500)",
		},
		{
			name:          "network error",
			api:           Kubernetes,
			path:          "/test",
			unreachable:   true,
			expectError:   true,
			errorContains: "error on GET to",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var server *httptest.Server
			var serverURL string

			if tc.unreachable {
				serverURL = "http://unreachable:12345"
			} else {
				server = httptest.NewServer(http.HandlerFunc(tc.serverResponse))
				defer server.Close()
				serverURL = server.URL
			}

			opts := &KAOptions{
				host: serverURL,
				K8sRESTConfig: &rest.Config{
					Host: serverURL,
				},
				K9eRESTConfig: &rest.Config{
					Host: serverURL,
				},
			}

			result, apiErr := opts.GetFromAPI(tc.api, tc.path)

			if tc.expectError {
				assert.NotNil(t, apiErr)
				if tc.errorContains != "" {
					assert.Contains(t, apiErr.Error(), tc.errorContains)
				}
			} else {
				assert.Nil(t, apiErr)
				assert.Equal(t, tc.expectedBody, string(result))
			}
		})
	}
}

func TestResolveResourceSpec(t *testing.T) {
	testCases := []struct {
		name             string
		resourceSpec     string
		expectedResource string
		expectedVersion  string
		expectedGroup    string
		expectedKind     string
		expectError      bool
		errorContains    string
	}{
		{
			name:             "core resource only",
			resourceSpec:     "pods",
			expectedResource: "pods",
			expectedVersion:  "v1",
			expectedGroup:    "",
			expectedKind:     "Pod",
		},
		{
			name:             "core resource short name",
			resourceSpec:     "po",
			expectedResource: "pods",
			expectedVersion:  "v1",
			expectedGroup:    "",
			expectedKind:     "Pod",
		},
		{
			name:             "core resource singular name",
			resourceSpec:     "pod",
			expectedResource: "pods",
			expectedVersion:  "v1",
			expectedGroup:    "",
			expectedKind:     "Pod",
		},
		{
			name:             "non-core resource with group",
			resourceSpec:     "deployments.apps",
			expectedResource: "deployments",
			expectedVersion:  "v1",
			expectedGroup:    "apps",
			expectedKind:     "Deployment",
		},
		{
			name:             "non-core resource short name",
			resourceSpec:     "deploy",
			expectedResource: "deployments",
			expectedVersion:  "v1",
			expectedGroup:    "apps",
			expectedKind:     "Deployment",
		},
		{
			name:             "resource with version and group",
			resourceSpec:     "jobs.v1.batch",
			expectedResource: "jobs",
			expectedVersion:  "v1",
			expectedGroup:    "batch",
			expectedKind:     "Job",
		},
		{
			name:             "specific beta version requested",
			resourceSpec:     "deployments.v1beta2.apps",
			expectedResource: "deployments",
			expectedVersion:  "v1beta2",
			expectedGroup:    "apps",
			expectedKind:     "Deployment",
		},
		{
			name:             "older beta version requested",
			resourceSpec:     "deployments.v1beta1.apps",
			expectedResource: "deployments",
			expectedVersion:  "v1beta1",
			expectedGroup:    "apps",
			expectedKind:     "Deployment",
		},
		{
			name:             "group only - should use preferred version",
			resourceSpec:     "deployments.apps",
			expectedResource: "deployments",
			expectedVersion:  "v1",
			expectedGroup:    "apps",
			expectedKind:     "Deployment",
		},
		{
			name:          "empty resource",
			resourceSpec:  "",
			expectError:   true,
			errorContains: "resource name cannot be empty",
		},
		{
			name:          "too many parts",
			resourceSpec:  "pods.v1.core.extra",
			expectError:   true,
			errorContains: "invalid resource specification format",
		},
		{
			name:          "resource not found",
			resourceSpec:  "nonexistent",
			expectError:   true,
			errorContains: "resource \"nonexistent\" not found",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			opts := NewKAOptions()
			opts.discoveryClient = newMockDiscoveryClient()

			resourceInfo, err := opts.ResolveResourceSpec(tc.resourceSpec)

			if tc.expectError {
				assert.Error(t, err)
				if tc.errorContains != "" {
					assert.Contains(t, err.Error(), tc.errorContains)
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, resourceInfo)
				assert.Equal(t, tc.expectedResource, resourceInfo.Resource)
				assert.Equal(t, tc.expectedVersion, resourceInfo.Version)
				assert.Equal(t, tc.expectedGroup, resourceInfo.Group)
				assert.Equal(t, tc.expectedKind, resourceInfo.Kind)
			}
		})
	}
}
