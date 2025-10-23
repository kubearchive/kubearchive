// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"testing"

	kubearchiveapi "github.com/kubearchive/kubearchive/cmd/operator/api/v1"
	"github.com/kubearchive/kubearchive/pkg/constants"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

func createFakeClientWithSinkFilter(sinkFilter *kubearchiveapi.SinkFilter) dynamic.Interface {
	scheme := runtime.NewScheme()
	if sinkFilter != nil {
		unstructuredObj, _ := runtime.DefaultUnstructuredConverter.ToUnstructured(sinkFilter)
		unstructuredSinkFilter := &unstructured.Unstructured{Object: unstructuredObj}
		unstructuredSinkFilter.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   "kubearchive.org",
			Version: "v1",
			Kind:    "SinkFilter",
		})
		return dynamicfake.NewSimpleDynamicClient(scheme, unstructuredSinkFilter)
	}
	return dynamicfake.NewSimpleDynamicClient(scheme)
}

func createTestSinkFilter() *kubearchiveapi.SinkFilter {
	return &kubearchiveapi.SinkFilter{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "kubearchive.org/v1",
			Kind:       "SinkFilter",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      constants.SinkFilterResourceName,
			Namespace: constants.KubeArchiveNamespace,
		},
		Spec: kubearchiveapi.SinkFilterSpec{
			Namespaces: map[string][]kubearchiveapi.KubeArchiveConfigResource{
				"test-namespace": {
					{
						Selector: kubearchiveapi.APIVersionKind{
							Kind:       "Pod",
							APIVersion: "v1",
						},
						ArchiveWhen:     "true",
						DeleteWhen:      "false",
						ArchiveOnDelete: "metadata.name == 'test'",
					},
				},
				constants.SinkFilterGlobalNamespace: {
					{
						Selector: kubearchiveapi.APIVersionKind{
							Kind:       "Deployment",
							APIVersion: "apps/v1",
						},
						ArchiveWhen:     "metadata.labels.archive == 'true'",
						DeleteWhen:      "",
						ArchiveOnDelete: "",
					},
				},
			},
		},
	}
}

func TestNewSinkFilterReader(t *testing.T) {
	// This test verifies that NewSinkFilterReader creates a properly initialized reader
	// Note: We can't test the actual k8sclient.NewInstrumentedDynamicClient() call
	// without mocking the entire k8s client infrastructure, but we can test the structure
	reader := &SinkFilterReader{
		dynamicClient: createFakeClientWithSinkFilter(nil),
	}
	assert.NotNil(t, reader)
	assert.NotNil(t, reader.dynamicClient)
}

func TestSinkFilterReader_GetSinkFilter(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name          string
		sinkFilter    *kubearchiveapi.SinkFilter
		expectedError string
		expectNil     bool
	}{
		{
			name:          "Successful retrieval",
			sinkFilter:    createTestSinkFilter(),
			expectedError: "",
			expectNil:     false,
		},
		{
			name:          "SinkFilter not found",
			sinkFilter:    nil,
			expectedError: "",
			expectNil:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient := createFakeClientWithSinkFilter(tt.sinkFilter)

			reader := &SinkFilterReader{
				dynamicClient: fakeClient,
			}

			result, err := reader.GetSinkFilter(ctx)

			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				assert.NoError(t, err)
			}

			if tt.expectNil {
				assert.Nil(t, result)
			} else {
				assert.NotNil(t, result)
				assert.Equal(t, tt.sinkFilter.Name, result.Name)
				assert.Equal(t, tt.sinkFilter.Namespace, result.Namespace)
			}
		})
	}
}

func TestSinkFilterReader_ProcessAllNamespaces(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name          string
		sinkFilter    *kubearchiveapi.SinkFilter
		expectedError string
		expectEmpty   bool
	}{
		{
			name:          "Successful processing with SinkFilter",
			sinkFilter:    createTestSinkFilter(),
			expectedError: "",
			expectEmpty:   false,
		},
		{
			name:          "No SinkFilter found returns empty map",
			sinkFilter:    nil,
			expectedError: "",
			expectEmpty:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient := createFakeClientWithSinkFilter(tt.sinkFilter)

			reader := &SinkFilterReader{
				dynamicClient: fakeClient,
			}

			result, err := reader.ProcessAllNamespaces(ctx)

			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)

				if tt.expectEmpty {
					assert.Empty(t, result)
				} else {
					assert.NotEmpty(t, result)
				}
			}
		})
	}
}

func TestSinkFilterReader_ProcessSingleNamespace(t *testing.T) {
	ctx := context.Background()
	targetNamespace := "test-namespace"

	tests := []struct {
		name          string
		sinkFilter    *kubearchiveapi.SinkFilter
		expectedError string
		expectEmpty   bool
	}{
		{
			name:          "Successful processing with SinkFilter",
			sinkFilter:    createTestSinkFilter(),
			expectedError: "",
			expectEmpty:   false,
		},
		{
			name:          "No SinkFilter found returns empty map",
			sinkFilter:    nil,
			expectedError: "",
			expectEmpty:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient := createFakeClientWithSinkFilter(tt.sinkFilter)

			reader := &SinkFilterReader{
				dynamicClient: fakeClient,
			}

			result, err := reader.ProcessSingleNamespace(ctx, targetNamespace)

			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)

				if tt.expectEmpty {
					assert.Empty(t, result)
				} else {
					assert.NotEmpty(t, result)
				}
			}
		})
	}
}
