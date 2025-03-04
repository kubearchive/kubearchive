// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0
//go:build integration

package main

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/avast/retry-go/v4"
	"github.com/kubearchive/kubearchive/test"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TestKubeArchiveDeployments is redundant with the kubectl rollout status from the hack/quick-install.sh
// ,but it serves as a valid integration test, not a dummy that is not testing anything real.
func TestAllDeploymentsReady(t *testing.T) {
	t.Parallel()

	client, _ := test.GetKubernetesClient(t)

	retryErr := retry.Do(func() error {
		deployments, errList := client.AppsV1().Deployments("kubearchive").List(context.Background(), metav1.ListOptions{})
		if errList != nil {
			return fmt.Errorf("Failed to get Deployments from the 'kubearchive' namespace: %w", errList)
		}

		if len(deployments.Items) == 0 {
			return errors.New("No deployments found in the 'kubearchive' namespace, something went wrong.")
		}

		areAllReady := true
		for _, deployment := range deployments.Items {
			t.Logf("Deployment '%s' has '%d' ready replicas", deployment.Name, deployment.Status.ReadyReplicas)
			areAllReady = areAllReady && deployment.Status.ReadyReplicas >= 1
		}

		if areAllReady {
			t.Log("All deployments ready.")
			return nil
		}

		return errors.New("Timed out while waiting for deployments to be ready.")
	})

	if retryErr != nil {
		t.Fatal(retryErr)
	}
}

func TestNormalOperation(t *testing.T) {
	t.Parallel()

	clientset, _ := test.GetKubernetesClient(t)
	port := test.PortForwardApiServer(t, clientset)
	namespaceName, token := test.CreateTestNamespace(t, false)

	resources := map[string]any{
		"resources": []map[string]any{
			{
				"selector": map[string]string{
					"apiVersion": "batch/v1",
					"kind":       "Job",
				},
				"archiveWhen": "has(status.completionTime)",
			},
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
	test.RunLogGenerator(t, namespaceName)

	// Retrieve the objects from the DB using the API.
	url := fmt.Sprintf("https://localhost:%s/apis/batch/v1/namespaces/%s/jobs", port, namespaceName)
	retryErr := retry.Do(func() error {
		list, getUrlErr := test.GetUrl(t, token.Status.Token, url)
		if getUrlErr != nil {
			return getUrlErr
		}

		if len(list.Items) == 1 {
			return nil
		}
		return errors.New("could not retrieve a Job from the API")
	}, retry.Attempts(20), retry.MaxDelay(2*time.Second))

	if retryErr != nil {
		t.Fatal(retryErr)
	}
}
