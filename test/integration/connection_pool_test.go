// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0
//go:build integration

package main

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/avast/retry-go/v5"
	"github.com/kubearchive/kubearchive/pkg/database/env"
	"github.com/kubearchive/kubearchive/test"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

var dbDeployments = []string{"kubearchive-api-server", "kubearchive-sink"}

func TestConnectionPoolDefaults(t *testing.T) {

	for _, deployment := range dbDeployments {
		t.Run(deployment, func(t *testing.T) {
			logs, err := test.GetPodLogs(t, "kubearchive", deployment)
			if err != nil {
				t.Fatalf("Failed to get %s logs: %v", deployment, err)
			}

			expectedLog := "Database connection pool configured"
			if !strings.Contains(logs, expectedLog) {
				t.Fatalf("Expected log message %q not found in %s logs", expectedLog, deployment)
			}

			if !strings.Contains(logs, fmt.Sprintf("max_open_conns=%d", env.DbDefaultMaxOpenConns)) {
				t.Errorf("Expected default max_open_conns=%d in logs", env.DbDefaultMaxOpenConns)
			}
			if !strings.Contains(logs, fmt.Sprintf("max_idle_conns=%d", env.DbDefaultMaxIdleConns)) {
				t.Errorf("Expected default max_idle_conns=%d in logs", env.DbDefaultMaxIdleConns)
			}
			if !strings.Contains(logs, fmt.Sprintf("conn_max_lifetime=%s", env.DbDefaultConnMaxLifetime)) {
				t.Errorf("Expected default conn_max_lifetime=%s in logs", env.DbDefaultConnMaxLifetime)
			}
			if !strings.Contains(logs, fmt.Sprintf("conn_max_idle_time=%s", env.DbDefaultConnMaxIdleTime)) {
				t.Errorf("Expected default conn_max_idle_time=%s in logs", env.DbDefaultConnMaxIdleTime)
			}
		})
	}
}

func TestConnectionPoolCustomValues(t *testing.T) {
	clientset, _ := test.GetKubernetesClient(t)

	saveAndRestoreDeploymentEnvs(t, clientset)

	customEnvVars := map[string]string{
		"DATABASE_MAX_OPEN_CONNS":     "15",
		"DATABASE_MAX_IDLE_CONNS":     "7",
		"DATABASE_CONN_MAX_LIFETIME":  "10m",
		"DATABASE_CONN_MAX_IDLE_TIME": "3m",
	}

	for _, name := range dbDeployments {
		setDeploymentEnvVar(t, clientset, name, customEnvVars)
		waitForDeploymentReady(t, name)
	}

	for _, name := range dbDeployments {
		t.Run(name, func(t *testing.T) {
			logs := waitForLogMessage(t, name, "max_open_conns=15")

			if !strings.Contains(logs, "max_idle_conns=7") {
				t.Error("Expected max_idle_conns=7 in logs")
			}
			if !strings.Contains(logs, "conn_max_lifetime=10m0s") {
				t.Error("Expected conn_max_lifetime=10m0s in logs")
			}
			if !strings.Contains(logs, "conn_max_idle_time=3m0s") {
				t.Error("Expected conn_max_idle_time=3m0s in logs")
			}
		})
	}
}

func TestConnectionPoolRejectsInvalidValues(t *testing.T) {
	clientset, _ := test.GetKubernetesClient(t)

	saveAndRestoreDeploymentEnvs(t, clientset)

	invalidEnvVars := map[string]string{
		"DATABASE_MAX_OPEN_CONNS": "-10",
	}

	for _, name := range dbDeployments {
		setDeploymentEnvVar(t, clientset, name, invalidEnvVars)
		waitForDeploymentReady(t, name)
	}

	for _, name := range dbDeployments {
		t.Run(name, func(t *testing.T) {
			logs := waitForLogMessage(t, name, "Zero or negative value for environment variable is not allowed")

			if !strings.Contains(logs, fmt.Sprintf("max_open_conns=%d", env.DbDefaultMaxOpenConns)) {
				t.Errorf("Expected fallback to default max_open_conns=%d in logs", env.DbDefaultMaxOpenConns)
			}
		})
	}
}

// saveAndRestoreDeploymentEnvs saves the current env vars for all DB deployments
// and registers a t.Cleanup that restores them when the test finishes.
func saveAndRestoreDeploymentEnvs(t testing.TB, clientset *kubernetes.Clientset) {
	t.Helper()
	ctx := context.Background()

	originalEnvs := make(map[string][]corev1.EnvVar)
	for _, name := range dbDeployments {
		dep, err := clientset.AppsV1().Deployments("kubearchive").Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			t.Fatalf("Failed to get deployment %s: %v", name, err)
		}
		originalEnvs[name] = dep.Spec.Template.Spec.Containers[0].Env
	}

	t.Cleanup(func() {
		t.Log("Restoring original environment variables")
		for _, name := range dbDeployments {
			dep, cleanupErr := clientset.AppsV1().Deployments("kubearchive").Get(ctx, name, metav1.GetOptions{})
			if cleanupErr != nil {
				t.Logf("Failed to get deployment %s for cleanup: %v", name, cleanupErr)
				continue
			}
			dep.Spec.Template.Spec.Containers[0].Env = originalEnvs[name]
			_, cleanupErr = clientset.AppsV1().Deployments("kubearchive").Update(ctx, dep, metav1.UpdateOptions{})
			if cleanupErr != nil {
				t.Logf("Failed to restore deployment %s: %v", name, cleanupErr)
				continue
			}
			waitForDeploymentReady(t, name)
		}
	})
}

func setDeploymentEnvVar(t testing.TB, clientset *kubernetes.Clientset, deploymentName string, envVars map[string]string) {
	t.Helper()
	ctx := context.Background()

	deployment, err := clientset.AppsV1().Deployments("kubearchive").Get(ctx, deploymentName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Failed to get deployment %s: %v", deploymentName, err)
	}

	container := &deployment.Spec.Template.Spec.Containers[0]
	for key, value := range envVars {
		found := false
		for i, env := range container.Env {
			if env.Name == key {
				container.Env[i].Value = value
				found = true
				break
			}
		}
		if !found {
			container.Env = append(container.Env, corev1.EnvVar{Name: key, Value: value})
		}
	}

	_, err = clientset.AppsV1().Deployments("kubearchive").Update(ctx, deployment, metav1.UpdateOptions{})
	if err != nil {
		t.Fatalf("Failed to update deployment %s: %v", deploymentName, err)
	}
}

func waitForLogMessage(t testing.TB, deploymentName string, expectedMsg string) string {
	t.Helper()
	var logs string
	err := retry.New(retry.Attempts(20), retry.MaxDelay(3*time.Second)).Do(func() error {
		var logErr error
		logs, logErr = test.GetPodLogs(t, "kubearchive", deploymentName)
		if logErr != nil {
			return logErr
		}
		if !strings.Contains(logs, expectedMsg) {
			return fmt.Errorf("expected %q not found in %s logs yet", expectedMsg, deploymentName)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Timed out waiting for %q in %s logs: %v", expectedMsg, deploymentName, err)
	}
	return logs
}

func waitForDeploymentReady(t testing.TB, deploymentName string) {
	t.Helper()
	clientset, _ := test.GetKubernetesClient(t)
	ctx := context.Background()

	// Get the deployment's current generation to wait for the rollout to complete
	deployment, err := clientset.AppsV1().Deployments("kubearchive").Get(ctx, deploymentName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Failed to get deployment %s: %v", deploymentName, err)
	}
	expectedGeneration := deployment.Generation

	retryErr := retry.New(retry.Attempts(60), retry.MaxDelay(3*time.Second)).Do(func() error {
		dep, retryErr := clientset.AppsV1().Deployments("kubearchive").Get(ctx, deploymentName, metav1.GetOptions{})
		if retryErr != nil {
			return retryErr
		}
		if dep.Status.ObservedGeneration < expectedGeneration {
			return fmt.Errorf("deployment %s rollout not observed yet", deploymentName)
		}
		if dep.Status.UpdatedReplicas < *dep.Spec.Replicas {
			return fmt.Errorf("deployment %s waiting for updated replicas", deploymentName)
		}
		if dep.Status.AvailableReplicas < *dep.Spec.Replicas {
			return fmt.Errorf("deployment %s waiting for available replicas", deploymentName)
		}
		if dep.Status.UnavailableReplicas > 0 {
			return fmt.Errorf("deployment %s has unavailable replicas", deploymentName)
		}
		return nil
	})
	if retryErr != nil {
		t.Fatalf("Timed out waiting for deployment %s to be ready: %v", deploymentName, retryErr)
	}
}
