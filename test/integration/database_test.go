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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	k8sretry "k8s.io/client-go/util/retry"
)

// TestDatabaseConnection verifies the database connection retries using the Sink component.
func TestDatabaseConnection(t *testing.T) {
	clientset, dynclient := test.GetKubernetesClient(t)

	clusterResource := schema.GroupVersionResource{Group: "postgresql.cnpg.io", Version: "v1", Resource: "clusters"}
	database := dynclient.Resource(clusterResource).Namespace("postgresql")
	unfenceDatabase := func() error {
		_, err := database.Patch(context.Background(), "kubearchive", types.MergePatchType,
			[]byte(`{"metadata":{"annotations":{"cnpg.io/fencedInstances":null}}}`), metav1.PatchOptions{})
		return err
	}

	originalScale, err := clientset.AppsV1().Deployments("kubearchive").GetScale(context.Background(), "kubearchive-sink", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if restoreErr := setSinkReplicas(clientset, originalScale.Spec.Replicas); restoreErr != nil {
			t.Errorf("Could not restore sink replicas: %v", restoreErr)
			return
		}
		if originalScale.Spec.Replicas > 0 {
			waitForDeploymentReady(t, "kubearchive-sink")
		}
	})
	// Cleanup runs in reverse order: restore the database before waiting for the sink.
	t.Cleanup(func() {
		if unfenceErr := unfenceDatabase(); unfenceErr != nil {
			t.Errorf("Could not unfence database: %v", unfenceErr)
		}
	})

	// Fence database to make it unavailable - https://cloudnative-pg.io/documentation/1.24/fencing/
	_, err = database.Patch(context.Background(), "kubearchive", types.MergePatchType,
		[]byte(`{"metadata":{"annotations":{"cnpg.io/fencedInstances":"[\"*\"]"}}}`), metav1.PatchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Fenced database")

	podName := test.GetPodName(t, clientset, "kubearchive", "kubearchive-sink")

	// restart sink pod - replicas = 0
	err = setSinkReplicas(clientset, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Changing sink to 0 replicas")

	t.Logf("Waiting for sink pod '%s' to disappear", podName)
	err = retry.New(retry.Attempts(30), retry.MaxDelay(2*time.Second)).Do(func() error {
		_, e := clientset.CoreV1().Pods("kubearchive").Get(context.Background(), podName, metav1.GetOptions{})
		if apierrors.IsNotFound(e) {
			return nil
		}
		if e != nil {
			return e
		}
		return fmt.Errorf("Sink pod '%s' still exists", podName)
	})
	if err != nil {
		t.Fatal(err)
	}

	// restart sink pod - replicas = 1
	err = setSinkReplicas(clientset, 1)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Changing sink to 1 replicas")

	// wait to sink pod ready and generate connection retries with the database
	t.Logf("Waiting for sink to be up, and to generate retries with the database")
	retryErr := retry.New(retry.Attempts(30), retry.MaxDelay(2*time.Second)).Do(func() error {
		logs, getErr := test.GetPodLogs(t, "kubearchive", "kubearchive-sink")
		if getErr != nil {
			return getErr
		}

		if strings.Contains(logs, "connection refused") {
			return nil
		}

		return fmt.Errorf("Sink pod didn't try to connect to the database yet")
	})

	if retryErr != nil {
		t.Fatal(retryErr)
	}

	// Unfence database
	err = unfenceDatabase()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Unfenced database")

	retryErr = retry.New(retry.Attempts(30), retry.MaxDelay(2*time.Second)).Do(func() error {
		logs, getErr := test.GetPodLogs(t, "kubearchive", "kubearchive-sink")
		if getErr != nil {
			return getErr
		}

		if strings.Contains(logs, "Successfully connected to the database") {
			return nil
		}

		return errors.New("Sink pod did not connect successfully to the database yet")
	})

	if retryErr != nil {
		t.Fatal(retryErr)
	}

	// Wait for sink pod to be ready.
	waitForDeploymentReady(t, "kubearchive-sink")
}

func setSinkReplicas(clientset kubernetes.Interface, replicas int32) error {
	return k8sretry.RetryOnConflict(k8sretry.DefaultRetry, func() error {
		scale, err := clientset.AppsV1().Deployments("kubearchive").GetScale(context.Background(), "kubearchive-sink", metav1.GetOptions{})
		if err != nil {
			return err
		}
		scale.Spec.Replicas = replicas
		_, err = clientset.AppsV1().Deployments("kubearchive").UpdateScale(context.Background(), "kubearchive-sink", scale, metav1.UpdateOptions{})
		return err
	})
}
