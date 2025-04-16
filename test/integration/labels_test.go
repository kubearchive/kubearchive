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

	test.CreateKAC(t, namespaceName, map[string]any{
		"resources": []map[string]any{
			{
				"selector": map[string]string{
					"apiVersion": "v1",
					"kind":       "Pod",
				},
				"archiveWhen": "status.phase == 'Succeeded'",
			},
		},
	})

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

	labelsSelectors := map[string][]string{
		"":                       []string{"no-labels", "hello-world", "env-prod", "env-dev", "env-stage"},
		"!hello":                 []string{"no-labels", "env-prod", "env-dev", "env-stage"},
		"!env":                   []string{"no-labels", "hello-world"},
		"hello=world":            []string{"hello-world"},
		"hello in (world)":       []string{"hello-world"},
		"hello notin (world)":    []string{},
		"env":                    []string{"env-prod", "env-dev", "env-stage"},
		"env!=prod":              []string{"env-dev", "no-labels", "hello-world", "env-stage"},
		"env,env!=prod":          []string{"env-dev", "env-stage"},
		"env in (prod)":          []string{"env-prod"},
		"env notin (prod)":       []string{"env-dev", "env-stage"},
		"env notin (dev)":        []string{"env-prod", "env-stage"},
		"env notin (stage)":      []string{"env-prod", "env-dev"},
		"env notin (stage, dev)": []string{"env-prod"},
		"env notin (stage, dev),hello in (world)":    []string{},
		"env notin (stage, dev),hello notin (world)": []string{},
		"env notin (stage, dev), hello=world":        []string{},
	}

	for podName, labels := range podsLabels {
		pod.SetName(podName)
		pod.SetLabels(labels)
		_, err := clientset.CoreV1().Pods(namespaceName).Create(context.Background(), pod, metav1.CreateOptions{})
		if err != nil {
			t.Fatal(err)
		}
	}

	var list *unstructured.UnstructuredList
	port := test.PortForwardApiServer(t, clientset)
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

	for labelSelector, podNames := range labelsSelectors {
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
	}

}
