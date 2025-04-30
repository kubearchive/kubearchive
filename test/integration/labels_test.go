// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0
//go:build integration

package main

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/avast/retry-go/v4"
	"github.com/kubearchive/kubearchive/test"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestLabels(t *testing.T) {
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
				corev1.Container{
					Name:    "fedora",
					Command: []string{"sleep", "1"},
					Image:   "quay.io/fedora/fedora-minimal:latest",
				},
			},
		},
	}

	podsLabels := map[string]map[string]string{
		"no-labels":   map[string]string{},
		"hello-world": map[string]string{"hello": "world"},
		"env-prod":    map[string]string{"env": "prod"},
		"env-dev":     map[string]string{"env": "dev"},
		"env-stage":   map[string]string{"env": "stage"},
	}

	for podName, labels := range podsLabels {
		pod.SetName(podName)
		pod.SetLabels(labels)
		_, err := clientset.CoreV1().Pods(namespaceName).Create(context.Background(), pod, metav1.CreateOptions{})
		if err != nil {
			t.Fatal(err)
		}
	}

	testCases := map[string]struct {
		selector     string
		expectedPods []string
	}{
		"no selector": {
			selector:     "",
			expectedPods: []string{"no-labels", "hello-world", "env-prod", "env-dev", "env-stage"},
		},
		"hello label does not exist": {
			selector:     "!hello",
			expectedPods: []string{"no-labels", "env-prod", "env-dev", "env-stage"},
		},
		"env label does not exist": {
			selector:     "!env",
			expectedPods: []string{"no-labels", "hello-world"},
		},
		"equality label selector": {
			selector:     "hello=world",
			expectedPods: []string{"hello-world"},
		},
		"hello:world set based selector": {
			selector:     "hello in (world)",
			expectedPods: []string{"hello-world"},
		},
		"key hello with no world value - set based": {
			selector:     "hello notin (world)",
			expectedPods: []string{},
		},
		"env label existence": {
			selector:     "env",
			expectedPods: []string{"env-prod", "env-dev", "env-stage"},
		},
		"env different from prod - inequality": {
			selector:     "env!=prod",
			expectedPods: []string{"env-dev", "no-labels", "hello-world", "env-stage"},
		},
		"env label and no 'prod' value - inequality": {
			selector:     "env,env!=prod",
			expectedPods: []string{"env-dev", "env-stage"},
		},
		"env:prod set based selector": {
			selector:     "env in (prod)",
			expectedPods: []string{"env-prod"},
		},
		"env different from prod - set based": {
			selector:     "env notin (prod)",
			expectedPods: []string{"env-dev", "env-stage"},
		},
		"env different from dev - set based": {
			selector:     "env notin (dev)",
			expectedPods: []string{"env-prod", "env-stage"},
		},
		"env different from stage - set based": {
			selector:     "env notin (stage)",
			expectedPods: []string{"env-prod", "env-dev"},
		},
		"env different from stage or dev - set based": {
			selector:     "env notin (stage, dev)",
			expectedPods: []string{"env-prod"},
		},
		"env different from stage or dev and hello:world - set based": {
			selector:     "env notin (stage, dev),hello in (world)",
			expectedPods: []string{},
		},
		"env different from stage or dev and hello different from world - set based": {
			selector:     "env notin (stage, dev),hello notin (world)",
			expectedPods: []string{},
		},
		"env different from stage or dev - set based - and hello:world - equality": {
			selector:     "env notin (stage, dev), hello=world",
			expectedPods: []string{},
		},
	}

	var list *unstructured.UnstructuredList
	podsURL := fmt.Sprintf("https://localhost:%s/api/v1/namespaces/%s/pods", port, namespaceName)
	retryErr := retry.Do(func() error {
		var getUrlErr error
		list, getUrlErr = test.GetUrl(t, token.Status.Token, podsURL)
		if getUrlErr != nil {
			t.Fatal(getUrlErr)
		}

		// We want to wait until everything is stored in the DB to avoid out of order inserts
		if len(list.Items) >= len(podsLabels) {
			return nil
		}

		return errors.New("could not retrieve Pods from the API")
	}, retry.Attempts(240), retry.MaxDelay(4*time.Second))

	if retryErr != nil {
		t.Fatal(retryErr)
	}

	t.Log("All pods were archived, starting to check label selector results...")

	for testName, testData := range testCases {
		t.Run(testName, func(t *testing.T) {
			labelSelector := testData.selector
			podNames := testData.expectedPods

			t.Logf("testing selector '%s'", labelSelector)
			podsURL = fmt.Sprintf("https://localhost:%s/api/v1/namespaces/%s/pods", port, namespaceName)
			if labelSelector != "" {
				selector := url.QueryEscape(labelSelector)
				podsURL = fmt.Sprintf("https://localhost:%s/api/v1/namespaces/%s/pods?labelSelector=%s", port, namespaceName, selector)
			}

			retryErr := retry.Do(func() error {
				podList, getUrlErr := test.GetUrl(t, token.Status.Token, podsURL)
				if getUrlErr != nil {
					t.Fatal(getUrlErr)
				}

				if len(podList.Items) != len(podNames) {
					msg := fmt.Sprintf("expected '%d' pods, got '%d'", len(podNames), len(podList.Items))
					t.Log(msg)
					return errors.New(msg)
				}

				retrievedPodNames := []string{}
				for _, pod := range podList.Items {
					retrievedPodNames = append(retrievedPodNames, pod.GetName())
				}

				missingPods := []string{}
				for _, podName := range podNames {
					found := false
					for _, retrievedPodName := range retrievedPodNames {
						if podName == retrievedPodName {
							found = true
						}
					}

					if !found {
						missingPods = append(missingPods, podName)
					}
				}

				if len(missingPods) >= 1 {
					msg := fmt.Sprintf("There were missing pods: %s", strings.Join(missingPods, ", "))
					t.Log(msg)
					return errors.New(msg)
				}

				t.Log("found expected pods for the selector")
				return nil
			}, retry.Attempts(240), retry.MaxDelay(4*time.Second))

			if retryErr != nil {
				t.Fatal(retryErr)
			}
		})
	}
}
