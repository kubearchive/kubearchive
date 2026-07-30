// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0
//go:build integration

package main

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/avast/retry-go/v5"
	"github.com/kubearchive/kubearchive/test"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/remotecommand"
)

// TestPasswordRotation verifies that database password rotation works correctly.
//
// Prerequisite (external to KubeArchive):
//   - The database administrator changes the kubearchive user password in PostgreSQL.
//
// KubeArchive user procedure:
//  1. Update the kubearchive-database-credentials Secret with the new password.
//  2. Rollout restart kubearchive-sink and kubearchive-api-server.
//  3. Verify the components reconnect successfully with the new credentials.
func TestPasswordRotation(t *testing.T) {
	clientset, _ := test.GetKubernetesClient(t)
	ctx := context.Background()

	secret, err := clientset.CoreV1().Secrets("kubearchive").Get(ctx, "kubearchive-database-credentials", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Failed to get kubearchive-database-credentials secret: %v", err)
	}
	originalPassword := string(secret.Data["DATABASE_PASSWORD"])
	t.Logf("Retrieved original database password from Secret")

	newPassword := "Rotated-P@ss-" + test.RandomString()

	// Register cleanup steps as separate t.Cleanup calls so each runs
	// independently (LIFO order) even if a previous one fails.
	t.Cleanup(func() {
		waitForLogMessage(t, "kubearchive-api-server", "Successfully connected to the database")
	})
	t.Cleanup(func() {
		waitForLogMessage(t, "kubearchive-sink", "Successfully connected to the database")
	})
	t.Cleanup(func() {
		scaleRestartDeployment(t, clientset, "kubearchive-api-server")
	})
	t.Cleanup(func() {
		scaleRestartDeployment(t, clientset, "kubearchive-sink")
	})
	t.Cleanup(func() {
		setDatabasePassword(t, clientset, originalPassword)
	})
	t.Cleanup(func() {
		t.Log("Restoring original database password")
		alterDatabasePassword(t, clientset, originalPassword)
	})

	// Prerequisite: database administrator changes the password in PostgreSQL
	t.Log("Prerequisite: changing database password in PostgreSQL")
	alterDatabasePassword(t, clientset, newPassword)

	// Step 1: update the kubearchive-database-credentials Secret
	t.Log("Step 1: updating kubearchive-database-credentials Secret")
	setDatabasePassword(t, clientset, newPassword)

	// Step 2: rollout restart the KubeArchive deployments
	t.Log("Step 2: restarting kubearchive-sink")
	scaleRestartDeployment(t, clientset, "kubearchive-sink")
	t.Log("Step 2: restarting kubearchive-api-server")
	scaleRestartDeployment(t, clientset, "kubearchive-api-server")

	// Step 3: verify the components reconnect with the new credentials
	t.Log("Step 3: waiting for sink to connect to the database with the new password")
	waitForLogMessage(t, "kubearchive-sink", "Successfully connected to the database")
	t.Log("Step 3: waiting for api-server to connect to the database with the new password")
	waitForLogMessage(t, "kubearchive-api-server", "Successfully connected to the database")
}

// alterDatabasePassword changes the kubearchive database user password by
// executing ALTER USER via psql in the PostgreSQL pod.
func alterDatabasePassword(t testing.TB, clientset *kubernetes.Clientset, password string) {
	t.Helper()

	config, err := test.GetKubernetesConfig()
	if err != nil {
		t.Fatalf("Failed to get Kubernetes config: %v", err)
	}

	podName := test.GetPodName(t, clientset, "postgresql", "kubearchive-")
	if podName == "" {
		t.Fatal("Could not find PostgreSQL pod in namespace 'postgresql'")
	}

	escapedPassword := strings.ReplaceAll(password, "'", "''")
	cmd := []string{"psql", "-U", "postgres", "-c", fmt.Sprintf("ALTER USER kubearchive WITH PASSWORD '%s'", escapedPassword)}

	req := clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(podName).
		Namespace("postgresql").
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Command: cmd,
			Stdout:  true,
			Stderr:  true,
		}, scheme.ParameterCodec)

	executor, err := remotecommand.NewSPDYExecutor(config, "POST", req.URL())
	if err != nil {
		t.Fatalf("Failed to create SPDY executor: %v", err)
	}

	var stdout, stderr bytes.Buffer
	execCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	err = executor.StreamWithContext(execCtx, remotecommand.StreamOptions{
		Stdout: &stdout,
		Stderr: &stderr,
	})
	if err != nil {
		t.Fatalf("Failed to execute ALTER USER: %v, stderr: %s", err, stderr.String())
	}

	t.Logf("ALTER USER result: %s", strings.TrimSpace(stdout.String()))
}

// setDatabasePassword updates the DATABASE_PASSWORD in the kubearchive-database-credentials Secret.
func setDatabasePassword(t testing.TB, clientset *kubernetes.Clientset, password string) {
	t.Helper()
	ctx := context.Background()

	secret, err := clientset.CoreV1().Secrets("kubearchive").Get(ctx, "kubearchive-database-credentials", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Failed to get secret: %v", err)
	}

	secret.Data["DATABASE_PASSWORD"] = []byte(password)

	_, err = clientset.CoreV1().Secrets("kubearchive").Update(ctx, secret, metav1.UpdateOptions{})
	if err != nil {
		t.Fatalf("Failed to update secret: %v", err)
	}

	// Verify the Secret was updated correctly
	updated, err := clientset.CoreV1().Secrets("kubearchive").Get(ctx, "kubearchive-database-credentials", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Failed to verify updated secret: %v", err)
	}
	if string(updated.Data["DATABASE_PASSWORD"]) != password {
		t.Fatal("Secret password mismatch after update")
	}
	t.Log("Secret updated successfully")
}

// scaleRestartDeployment restarts a deployment by scaling it to 0 and back
// to its original replica count, waiting for the old pods to terminate and
// new pods to be created.
func scaleRestartDeployment(t testing.TB, clientset *kubernetes.Clientset, deploymentName string) {
	t.Helper()
	ctx := context.Background()

	labelSelector := fmt.Sprintf("app=%s", deploymentName)

	scale, err := clientset.AppsV1().Deployments("kubearchive").GetScale(ctx, deploymentName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Failed to get scale for %s: %v", deploymentName, err)
	}
	originalReplicas := scale.Spec.Replicas

	// Scale to 0
	scaleCopy := *scale
	scaleCopy.Spec.Replicas = 0
	_, err = clientset.AppsV1().Deployments("kubearchive").UpdateScale(ctx, deploymentName, &scaleCopy, metav1.UpdateOptions{})
	if err != nil {
		t.Fatalf("Failed to scale %s to 0: %v", deploymentName, err)
	}
	t.Logf("Scaled %s to 0 replicas", deploymentName)

	// Wait for all pods to disappear
	err = retry.New(retry.Attempts(30), retry.MaxDelay(2*time.Second)).Do(func() error {
		pods, listErr := clientset.CoreV1().Pods("kubearchive").List(ctx, metav1.ListOptions{
			LabelSelector: labelSelector,
		})
		if listErr != nil {
			return listErr
		}
		if len(pods.Items) > 0 {
			return fmt.Errorf("%s still has %d pod(s)", deploymentName, len(pods.Items))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Timed out waiting for %s pods to terminate: %v", deploymentName, err)
	}

	// Scale back to the original replica count
	scale, err = clientset.AppsV1().Deployments("kubearchive").GetScale(ctx, deploymentName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Failed to get scale for %s: %v", deploymentName, err)
	}
	scaleCopy = *scale
	scaleCopy.Spec.Replicas = originalReplicas
	_, err = clientset.AppsV1().Deployments("kubearchive").UpdateScale(ctx, deploymentName, &scaleCopy, metav1.UpdateOptions{})
	if err != nil {
		t.Fatalf("Failed to scale %s to %d: %v", deploymentName, originalReplicas, err)
	}
	t.Logf("Scaled %s to %d replicas", deploymentName, originalReplicas)

	waitForDeploymentReady(t, deploymentName)
}
