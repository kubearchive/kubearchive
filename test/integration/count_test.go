// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0
//go:build integration

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/avast/retry-go/v5"
	"github.com/kubearchive/kubearchive/test"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type countResponse struct {
	Count int64 `json:"count"`
}

func TestCount(t *testing.T) {
	t.Parallel()

	clientset, _ := test.GetKubernetesClient(t)
	namespaceName, token := test.CreateTestNamespace(t, false)

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

	expectedCount := 5
	for i := range expectedCount {
		pod.SetName("count-pod-" + strconv.Itoa(i))
		_, err := clientset.CoreV1().Pods(namespaceName).Create(context.Background(), pod, metav1.CreateOptions{})
		if err != nil {
			t.Fatal(err)
		}
	}

	port := test.PortForwardApiServer(t, clientset)

	// Wait for all pods to be archived
	listURL := fmt.Sprintf("https://localhost:%s/api/v1/namespaces/%s/pods", port, namespaceName)
	err := retry.New(retry.Attempts(240), retry.MaxDelay(4*time.Second)).Do(func() error {
		list, getUrlErr := test.GetUrl(t, token.Status.Token, listURL, map[string][]string{})
		if getUrlErr != nil {
			t.Fatal(getUrlErr)
		}
		if len(list.Items) >= expectedCount {
			return nil
		}
		return errors.New("waiting for pods to be archived")
	})
	if err != nil {
		t.Fatal(err)
	}

	// Test count endpoint
	countURL := fmt.Sprintf("https://localhost:%s/api/v1/namespaces/%s/pods?count=true", port, namespaceName)
	body, getUrlErr := test.GetRawUrl(t, token.Status.Token, countURL, map[string][]string{})
	if getUrlErr != nil {
		t.Fatal(getUrlErr)
	}

	var result countResponse
	err = json.Unmarshal(body, &result)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, int64(expectedCount), result.Count)
}

func TestCountWithNoResources(t *testing.T) {
	t.Parallel()

	clientset, _ := test.GetKubernetesClient(t)
	namespaceName, token := test.CreateTestNamespace(t, false)

	test.CreateKAC(t, "testdata/kac-with-resources.yaml", namespaceName)

	port := test.PortForwardApiServer(t, clientset)

	countURL := fmt.Sprintf("https://localhost:%s/api/v1/namespaces/%s/pods?count=true", port, namespaceName)
	body, getUrlErr := test.GetRawUrl(t, token.Status.Token, countURL, map[string][]string{})
	if getUrlErr != nil {
		t.Fatal(getUrlErr)
	}

	var result countResponse
	err := json.Unmarshal(body, &result)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, int64(0), result.Count)
}

func TestCountRejectsLimitParam(t *testing.T) {
	t.Parallel()

	clientset, _ := test.GetKubernetesClient(t)
	namespaceName, token := test.CreateTestNamespace(t, false)

	test.CreateKAC(t, "testdata/kac-with-resources.yaml", namespaceName)

	port := test.PortForwardApiServer(t, clientset)

	countURL := fmt.Sprintf("https://localhost:%s/api/v1/namespaces/%s/pods?count=true&limit=10", port, namespaceName)
	_, getUrlErr := test.GetRawUrl(t, token.Status.Token, countURL, map[string][]string{})
	assert.Error(t, getUrlErr)
	assert.Contains(t, getUrlErr.Error(), "400")
}

func TestCountRejectsContinueParam(t *testing.T) {
	t.Parallel()

	clientset, _ := test.GetKubernetesClient(t)
	namespaceName, token := test.CreateTestNamespace(t, false)

	test.CreateKAC(t, "testdata/kac-with-resources.yaml", namespaceName)

	port := test.PortForwardApiServer(t, clientset)

	countURL := fmt.Sprintf("https://localhost:%s/api/v1/namespaces/%s/pods?count=true&continue=MSAyMDI1LTAxLTAxVDAwOjAwOjAwWg==", port, namespaceName)
	_, getUrlErr := test.GetRawUrl(t, token.Status.Token, countURL, map[string][]string{})
	assert.Error(t, getUrlErr)
	assert.Contains(t, getUrlErr.Error(), "400")
}
