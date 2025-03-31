// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0
//go:build integration

package main

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/avast/retry-go/v4"
	"github.com/kubearchive/kubearchive/test"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestPagination(t *testing.T) {
	t.Parallel()

	clientset, _ := test.GetKubernetesClient(t)
	namespaceName, token := test.CreateTestNamespace(t, false)

	resources := map[string]any{
		"resources": []map[string]any{
			{
				"selector": map[string]string{
					"apiVersion": "v1",
					"kind":       "Pod",
				},
				"deleteWhen": "status.phase == 'Succeeded'",
			},
		},
	}

	test.CreateKAC(t, namespaceName, resources)

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

	for i := range 30 {
		pod.SetName("pod-" + strconv.Itoa(i))
		_, err := clientset.CoreV1().Pods(namespaceName).Create(context.Background(), pod, metav1.CreateOptions{})
		if err != nil {
			t.Fatal(err)
		}
	}

	port := test.PortForwardApiServer(t, clientset)
	url := fmt.Sprintf("https://localhost:%s/api/v1/namespaces/%s/pods", port, namespaceName)
	var list *unstructured.UnstructuredList
	var getUrlErr error
	err := retry.Do(func() error {
		list, getUrlErr = test.GetUrl(t, token.Status.Token, url)
		if getUrlErr != nil {
			t.Fatal(getUrlErr)
		}
		// We want to wait until everything is stored in the DB to avoid out of order inserts
		if len(list.Items) >= 30 {
			return nil
		}
		return errors.New("could not retrieve Pods from the API")
	}, retry.Attempts(240), retry.MaxDelay(4*time.Second))

	if err != nil {
		t.Fatal(err)
	}

	url = fmt.Sprintf("https://localhost:%s/api/v1/namespaces/%s/pods?limit=10", port, namespaceName)
	initList, getUrlErrInitList := test.GetUrl(t, token.Status.Token, url)
	if getUrlErrInitList != nil {
		t.Fatal(getUrlErrInitList)
	}
	assert.Equal(t, 10, len(initList.Items))

	url = fmt.Sprintf(
		"https://localhost:%s/api/v1/namespaces/%s/pods?limit=10&continue=%s",
		port,
		namespaceName,
		initList.GetContinue(),
	)
	continueList, getUrlErrContinueList := test.GetUrl(t, token.Status.Token, url)
	if getUrlErrContinueList != nil {
		t.Fatal(getUrlErrContinueList)
	}
	assert.Equal(t, 10, len(continueList.Items))

	url = fmt.Sprintf("https://localhost:%s/api/v1/namespaces/%s/pods?limit=20", port, namespaceName)
	allList, getUrlErrAllList := test.GetUrl(t, token.Status.Token, url)
	if getUrlErrAllList != nil {
		t.Fatal(getUrlErrAllList)
	}
	assert.Equal(t, 20, len(allList.Items))

	var listNames []string
	for _, item := range initList.Items {
		listNames = append(listNames, item.GetName())
	}

	var continueListNames []string
	for _, item := range continueList.Items {
		continueListNames = append(continueListNames, item.GetName())
	}
	assert.NotContains(t, continueListNames, listNames)

	var allListNames []string
	for _, item := range allList.Items {
		allListNames = append(allListNames, item.GetName())
	}
	assert.Equal(t, allListNames, append(listNames, continueListNames...))
}
