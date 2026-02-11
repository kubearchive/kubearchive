// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0
//go:build integration

package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/avast/retry-go/v5"
	"github.com/kubearchive/kubearchive/test"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestWildcardNameFilteringIntegration(t *testing.T) {
	t.Parallel()

	clientset, dynamicClient := test.GetKubernetesClient(t)
	namespaceName, token := test.CreateTestNamespace(t, false)
	port := test.PortForwardApiServer(t, clientset)

	test.CreateKAC(t, "testdata/kac-with-resources.yaml", namespaceName)

	testResources := []struct {
		name string
		gvr  schema.GroupVersionResource
	}{
		{
			name: "test-123",
			gvr:  schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"},
		},
		{
			name: "test-456",
			gvr:  schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"},
		},
		{
			name: "other-resource",
			gvr:  schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"},
		},
	}

	for _, res := range testResources {
		t.Logf("Creating test resource: %s/%s", namespaceName, res.name)

		obj := &unstructured.Unstructured{}
		obj.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   res.gvr.Group,
			Version: res.gvr.Version,
			Kind:    "Pod",
		})
		obj.SetName(res.name)
		obj.SetNamespace(namespaceName)

		obj.Object["spec"] = map[string]interface{}{
			"containers": []interface{}{
				map[string]interface{}{
					"name":  "test",
					"image": "nginx:latest",
				},
			},
			"restartPolicy": "Never",
		}

		_, err := dynamicClient.Resource(res.gvr).Namespace(namespaceName).Create(context.TODO(), obj, metav1.CreateOptions{})
		if err != nil {
			t.Logf("Failed to create resource %s: %v (might already exist)", res.name, err)
		}

		defer func(gvr schema.GroupVersionResource, namespace, name string) {
			err := dynamicClient.Resource(gvr).Namespace(namespace).Delete(context.TODO(), name, metav1.DeleteOptions{})
			if err != nil {
				t.Logf("Failed to delete resource %s: %v", name, err)
			}
		}(res.gvr, namespaceName, res.name)
	}

	testCases := []struct {
		name          string
		endpoint      string
		expectedNames []string
		description   string
	}{
		{
			name:          "wildcard contains test lowercase",
			endpoint:      fmt.Sprintf("/api/v1/namespaces/%s/pods?name=*test*", namespaceName),
			expectedNames: []string{"test-123", "test-456"},
			description:   "Should match pods containing 'test'",
		},
		{
			name:          "wildcard contains test uppercase",
			endpoint:      fmt.Sprintf("/api/v1/namespaces/%s/pods?name=*TEST*", namespaceName),
			expectedNames: []string{"test-123", "test-456"},
			description:   "Should match pods containing 'TEST' (case insensitive)",
		},
		{
			name:          "wildcard suffix 123",
			endpoint:      fmt.Sprintf("/api/v1/namespaces/%s/pods?name=*123", namespaceName),
			expectedNames: []string{"test-123"},
			description:   "Should match pods ending with '123'",
		},
		{
			name:          "wildcard no matches",
			endpoint:      fmt.Sprintf("/api/v1/namespaces/%s/pods?name=*notfound*", namespaceName),
			expectedNames: []string{},
			description:   "Should return empty list when no matches",
		},
		{
			name:          "validation error - conflicting parameters",
			endpoint:      fmt.Sprintf("/api/v1/namespaces/%s/pods/test-123?name=*test*", namespaceName),
			expectedNames: []string{}, // Not used for error cases
			description:   "Should return 400 when both path and query name parameters are provided",
		},
		{
			name:          "validation error - wildcard in path parameter",
			endpoint:      fmt.Sprintf("/api/v1/namespaces/%s/pods/*test*", namespaceName),
			expectedNames: []string{}, // Not used for error cases
			description:   "Should return 400 when wildcard characters are used in path parameter",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Logf("Testing endpoint: %s", tc.endpoint)

			url := fmt.Sprintf("https://localhost:%s%s", port, tc.endpoint)

			if tc.name == "validation error - conflicting parameters" || tc.name == "validation error - wildcard in path parameter" {
				_, err := test.GetUrl(t, token.Status.Token, url, nil)
				assert.Error(t, err, tc.description)
				assert.Contains(t, err.Error(), "400", tc.description)
				return
			}

			retryErr := retry.New(retry.Attempts(240), retry.MaxDelay(4*time.Second)).Do(func() error {
				result, getUrlErr := test.GetUrl(t, token.Status.Token, url, nil)
				if getUrlErr != nil {
					return getUrlErr
				}

				var actualNames []string
				for _, item := range result.Items {
					metadata := item.Object["metadata"].(map[string]interface{})
					name := metadata["name"].(string)
					actualNames = append(actualNames, name)
				}

				if len(actualNames) != len(tc.expectedNames) {
					msg := fmt.Sprintf("expected %d pods, got %d", len(tc.expectedNames), len(actualNames))
					t.Log(msg)
					return errors.New(msg)
				}

				missingNames := []string{}
				for _, expectedName := range tc.expectedNames {
					found := false
					for _, actualName := range actualNames {
						if expectedName == actualName {
							found = true
							break
						}
					}
					if !found {
						missingNames = append(missingNames, expectedName)
					}
				}

				if len(missingNames) > 0 {
					msg := fmt.Sprintf("missing expected pods: %s", strings.Join(missingNames, ", "))
					t.Log(msg)
					return errors.New(msg)
				}

				t.Logf("Found expected pods for wildcard pattern: %v", actualNames)
				return nil
			})

			if retryErr != nil {
				t.Fatal(retryErr)
			}
		})
	}
}
