// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0
//go:build integration

package main

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/avast/retry-go/v5"
	"github.com/kubearchive/kubearchive/test"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestTimestampFiltering(t *testing.T) {
	t.Parallel()

	clientset, _ := test.GetKubernetesClient(t)
	namespaceName, token := test.CreateTestNamespace(t, false)
	port := test.PortForwardApiServer(t, clientset)

	test.CreateKAC(t, "testdata/kac-with-resources.yaml", namespaceName)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespaceName,
		},
		Spec: corev1.PodSpec{
			RestartPolicy: corev1.RestartPolicyNever,
			Containers: []corev1.Container{
				{
					Name:    "fedora",
					Command: []string{"sleep", "1"},
					Image:   "quay.io/fedora/fedora-minimal:latest",
				},
			},
		},
	}

	// Create 3 pods with real time gaps (2 minutes apart)
	podTimes := make([]time.Time, 3)
	for i := 0; i < 3; i++ {
		// Record the actual creation time
		podTimes[i] = time.Now()

		pod.SetName("pod-" + strconv.Itoa(i))
		// Remove the artificial timestamp setting - let Kubernetes set the real creation time
		pod.SetCreationTimestamp(metav1.Time{})

		_, err := clientset.CoreV1().Pods(namespaceName).Create(context.Background(), pod, metav1.CreateOptions{})
		if err != nil {
			t.Fatal(err)
		}

		// Wait 10 seconds between pod creations (except for the last pod)
		if i < 2 {
			t.Logf("Created pod-%d, waiting 10 seconds before creating next pod...", i)
			time.Sleep(10 * time.Second)
		}
	}

	// Wait for all pods to be archived
	url := fmt.Sprintf("https://localhost:%s/api/v1/namespaces/%s/pods", port, namespaceName)
	var allPods *unstructured.UnstructuredList
	err := retry.New(retry.Attempts(240), retry.MaxDelay(4*time.Second)).Do(func() error {
		var getUrlErr error
		allPods, getUrlErr = test.GetUrl(t, token.Status.Token, url, map[string][]string{})
		if getUrlErr != nil {
			t.Fatal(getUrlErr)
		}
		if len(allPods.Items) >= 3 {
			return nil
		}
		return errors.New("could not retrieve all Pods from the API")
	})

	if err != nil {
		t.Fatal(err)
	}

	t.Log("All pods were archived, starting timestamp filter tests...")

	// Test cases for timestamp filtering
	testCases := []struct {
		name                      string
		creationTimestampAfter    string
		creationTimestampBefore   string
		expectedPods              int
		expectedStatusCode        int
		shouldContainErrorMessage string
	}{
		{
			name:                   "filter after first pod",
			creationTimestampAfter: podTimes[1].UTC().Format(time.RFC3339),
			expectedPods:           1, // only pod 2 (created after pod 1)
			expectedStatusCode:     200,
		},
		{
			name:                    "filter before last pod",
			creationTimestampBefore: podTimes[2].UTC().Format(time.RFC3339),
			expectedPods:            2, // pods 0, 1 (created before pod 2)
			expectedStatusCode:      200,
		},
		{
			name:                    "filter between pods",
			creationTimestampAfter:  podTimes[0].UTC().Format(time.RFC3339),
			creationTimestampBefore: podTimes[2].UTC().Format(time.RFC3339),
			expectedPods:            1, // only pod 1 (created after pod 0 and before pod 2)
			expectedStatusCode:      200,
		},
		{
			name:                   "filter after all pods (now)",
			creationTimestampAfter: time.Now().UTC().Format(time.RFC3339),
			expectedPods:           0, // no pods should match
			expectedStatusCode:     200,
		},
		{
			name:                      "invalid order - before is earlier than after",
			creationTimestampAfter:    podTimes[2].UTC().Format(time.RFC3339),
			creationTimestampBefore:   podTimes[0].UTC().Format(time.RFC3339),
			expectedStatusCode:        400,
			shouldContainErrorMessage: "creationTimestampBefore must be after creationTimestampAfter",
		},
		{
			name:                      "invalid order - same timestamps",
			creationTimestampAfter:    podTimes[1].UTC().Format(time.RFC3339),
			creationTimestampBefore:   podTimes[1].UTC().Format(time.RFC3339),
			expectedStatusCode:        400,
			shouldContainErrorMessage: "creationTimestampBefore must be after creationTimestampAfter",
		},
		{
			name:                      "invalid timestamp format - after",
			creationTimestampAfter:    "invalid-timestamp",
			expectedStatusCode:        400,
			shouldContainErrorMessage: "invalid creationTimestampAfter format",
		},
		{
			name:                      "invalid timestamp format - before",
			creationTimestampBefore:   "invalid-timestamp",
			expectedStatusCode:        400,
			shouldContainErrorMessage: "invalid creationTimestampBefore format",
		},
		{
			name:                   "RFC3339Nano format support",
			creationTimestampAfter: time.Now().Add(-2 * time.Hour).UTC().Format(time.RFC3339Nano),
			expectedPods:           3, // should return all pods since timestamp is 2 hours ago
			expectedStatusCode:     200,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Build URL with query parameters
			testURL := fmt.Sprintf("https://localhost:%s/api/v1/namespaces/%s/pods", port, namespaceName)
			queryParams := []string{}

			if tc.creationTimestampAfter != "" {
				queryParams = append(queryParams, fmt.Sprintf("creationTimestampAfter=%s", tc.creationTimestampAfter))
			}
			if tc.creationTimestampBefore != "" {
				queryParams = append(queryParams, fmt.Sprintf("creationTimestampBefore=%s", tc.creationTimestampBefore))
			}

			if len(queryParams) > 0 {
				testURL += "?" + strings.Join(queryParams, "&")
			}

			t.Logf("Testing URL: %s", testURL)

			if tc.expectedStatusCode == 200 {
				result, err := test.GetUrl(t, token.Status.Token, testURL, map[string][]string{})
				if err != nil {
					t.Fatalf("Failed to get URL: %v", err)
				}
				assert.Equal(t, tc.expectedPods, len(result.Items), "Number of returned pods should match expected")
			} else {
				_, err := test.GetUrl(t, token.Status.Token, testURL, map[string][]string{})
				assert.Error(t, err, "Expected an error for invalid request")

				errorMsg := err.Error()
				expectedStatusStr := fmt.Sprintf("%d", tc.expectedStatusCode)
				assert.Contains(t, errorMsg, expectedStatusStr, "Error should contain expected status code")
			}
		})
	}
}
