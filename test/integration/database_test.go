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

	"github.com/avast/retry-go/v4"
	"github.com/kubearchive/kubearchive/test"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// TestDatabaseConnection verifies the database connection retries using the Sink component.
func TestDatabaseConnection(t *testing.T) {
	clientset, dynclient := test.GetKubernetesClient(t)

	// Fence database to make it unavailable - https://cloudnative-pg.io/documentation/1.24/fencing/
	clusterResource := schema.GroupVersionResource{Group: "postgresql.cnpg.io", Version: "v1", Resource: "clusters"}
	resource, err := dynclient.Resource(clusterResource).Namespace("postgresql").Get(context.Background(), "kubearchive", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	annotations := resource.GetAnnotations()
	annotations["cnpg.io/fencedInstances"] = "[\"*\"]"
	resource.SetAnnotations(annotations)

	resource, err = dynclient.Resource(clusterResource).Namespace("postgresql").Update(context.Background(), resource, metav1.UpdateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Fenced database")

	podName := test.GetPodName(t, clientset, "kubearchive", "kubearchive-sink")

	// restart sink pod - replicas = 0
	deploymentScaleSink, err := clientset.AppsV1().Deployments("kubearchive").GetScale(context.Background(), "kubearchive-sink", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}

	scaleSink := *deploymentScaleSink
	scaleSink.Spec.Replicas = 0
	usSink, err := clientset.AppsV1().Deployments("kubearchive").UpdateScale(context.Background(), "kubearchive-sink", &scaleSink, metav1.UpdateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Changing sink to %d replicas", scaleSink.Spec.Replicas)
	t.Log(*usSink)

	t.Logf("Waiting for sink pod '%s' to disappear", podName)
	err = retry.Do(func() error {
		_, e := clientset.CoreV1().Pods("kubearchive").Get(context.Background(), podName, metav1.GetOptions{})
		return e
	}, retry.Attempts(10), retry.MaxDelay(2*time.Second))

	// restart sink pod - replicas = 1
	deploymentScaleSink, err = clientset.AppsV1().Deployments("kubearchive").GetScale(context.Background(), "kubearchive-sink", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	scaleSink = *deploymentScaleSink
	scaleSink.Spec.Replicas = 1
	usSink, err = clientset.AppsV1().Deployments("kubearchive").UpdateScale(context.Background(), "kubearchive-sink", &scaleSink, metav1.UpdateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Changing sink to %d replicas", scaleSink.Spec.Replicas)
	t.Log(*usSink)

	// wait to sink pod ready and generate connection retries with the database
	t.Logf("Waiting for sink to be up, and to generate retries with the database")
	retryErr := retry.Do(func() error {
		logs, err := test.GetPodLogs(t, "kubearchive", "kubearchive-sink")
		if err != nil {
			return err
		}

		if strings.Contains(logs, "connection refused") {
			return nil
		}

		return fmt.Errorf("Sink pod didn't try to connect to the database yet")
	}, retry.Attempts(30), retry.MaxDelay(2*time.Second))

	if retryErr != nil {
		t.Fatal(retryErr)
	}

	// Unfence database
	resource, err = dynclient.Resource(clusterResource).Namespace("postgresql").Get(context.Background(), "kubearchive", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	annotations = resource.GetAnnotations()
	delete(annotations, "cnpg.io/fencedInstances")
	resource.SetAnnotations(annotations)

	resource, err = dynclient.Resource(clusterResource).Namespace("postgresql").Update(context.Background(), resource, metav1.UpdateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Unfenced database")

	retryErr = retry.Do(func() error {
		logs, err := test.GetPodLogs(t, "kubearchive", "kubearchive-sink")
		if err != nil {
			return nil
		}

		if strings.Contains(logs, "Successfully connected to the database") {
			return nil
		}

		return errors.New("Sink pod did not connect successfully to the database yet")
	}, retry.Attempts(30), retry.MaxDelay(2*time.Second))

	if retryErr != nil {
		t.Fatal(retryErr)
	}

	// Wait for sink pod to be ready.
	podName = test.GetPodName(t, clientset, "kubearchive", "kubearchive-sink")
	err = retry.Do(func() error {
		pod, e := clientset.CoreV1().Pods("kubearchive").Get(context.Background(), podName, metav1.GetOptions{})
		if e != nil {
			return e
		}

		for _, condition := range pod.Status.Conditions {
			if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
				return nil
			}
		}
		return errors.New("Sink pod is not in 'Ready' status")
	}, retry.Attempts(10), retry.MaxDelay(2*time.Second))
}
