// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

//go:build integration

package main

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/avast/retry-go/v5"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

const (
	jobNamespace = "kubearchive"
	jobBaseName  = "migration-test"
)

func getClientset(t *testing.T) *kubernetes.Clientset {
	t.Helper()
	kubeconfig := filepath.Join(homedir.HomeDir(), ".kube", "config")
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		t.Fatalf("failed to build kubeconfig: %s", err)
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		t.Fatalf("failed to create clientset: %s", err)
	}
	return clientset
}

// runMigrationJob creates and runs a migration Job in the cluster, waits for
// completion, and returns its logs.
func runMigrationJob(t *testing.T, clientset *kubernetes.Clientset, migrationVersion string) string {
	t.Helper()
	ctx := context.Background()

	jobName := fmt.Sprintf("%s-%d", jobBaseName, time.Now().UnixNano())

	// Find the image from the existing migration job (deployed by install.sh).
	existingJob, err := clientset.BatchV1().Jobs(jobNamespace).Get(ctx, "kubearchive-schema-migration", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("failed to get existing migration job to extract image: %s", err)
	}
	image := existingJob.Spec.Template.Spec.Containers[0].Image

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: jobNamespace,
		},
		Spec: batchv1.JobSpec{
			BackoffLimit: int32Ptr(0),
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					Containers: []corev1.Container{
						{
							Name:  "migration",
							Image: image,
							Env: []corev1.EnvVar{
								{Name: "MIGRATION_VERSION", Value: migrationVersion},
							},
							EnvFrom: []corev1.EnvFromSource{
								{
									SecretRef: &corev1.SecretEnvSource{
										LocalObjectReference: corev1.LocalObjectReference{
											Name: "kubearchive-database-credentials",
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	_, err = clientset.BatchV1().Jobs(jobNamespace).Create(ctx, job, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("failed to create migration job: %s", err)
	}
	t.Cleanup(func() {
		propagation := metav1.DeletePropagationForeground
		clientset.BatchV1().Jobs(jobNamespace).Delete(ctx, jobName, metav1.DeleteOptions{ //nolint:errcheck
			PropagationPolicy: &propagation,
		})
	})

	// Wait for the Job to finish (complete or failed).
	err = retry.New(retry.Attempts(60), retry.MaxDelay(2*time.Second)).Do(func() error {
		j, getErr := clientset.BatchV1().Jobs(jobNamespace).Get(ctx, jobName, metav1.GetOptions{})
		if getErr != nil {
			return getErr
		}
		for _, c := range j.Status.Conditions {
			if c.Type == batchv1.JobComplete && c.Status == corev1.ConditionTrue {
				return nil
			}
			if c.Type == batchv1.JobFailed && c.Status == corev1.ConditionTrue {
				return retry.Unrecoverable(fmt.Errorf("job failed: %s", c.Message))
			}
		}
		return fmt.Errorf("job %s still running", jobName)
	})
	if err != nil {
		t.Fatalf("migration job did not complete: %s", err)
	}

	return getJobLogs(t, clientset, jobName)
}

func getJobLogs(t *testing.T, clientset *kubernetes.Clientset, jobName string) string {
	t.Helper()
	ctx := context.Background()

	pods, err := clientset.CoreV1().Pods(jobNamespace).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("job-name=%s", jobName),
	})
	if err != nil || len(pods.Items) == 0 {
		t.Fatalf("failed to find pod for job %s: %v", jobName, err)
	}

	req := clientset.CoreV1().Pods(jobNamespace).GetLogs(pods.Items[0].Name, &corev1.PodLogOptions{})
	logStream, err := req.Stream(ctx)
	if err != nil {
		t.Fatalf("failed to get logs: %s", err)
	}
	defer logStream.Close()

	buf := new(bytes.Buffer)
	buf.ReadFrom(logStream) //nolint:errcheck
	return buf.String()
}

func int32Ptr(i int32) *int32 { return &i }

func TestMigrationJobIdempotent(t *testing.T) {
	clientset := getClientset(t)

	logs := runMigrationJob(t, clientset, "")
	t.Log(logs)
	if !strings.Contains(logs, "Migration completed successfully") {
		t.Fatal("migration job failed")
	}
}

func TestMigrationJobDownAndUp(t *testing.T) {
	clientset := getClientset(t)

	// Migrate down to version 6.
	logs := runMigrationJob(t, clientset, "6")
	t.Log("Down to v6:", logs)
	if !strings.Contains(logs, "Migrating down") {
		t.Fatal("expected down migration log message")
	}
	if !strings.Contains(logs, "Migration completed successfully") {
		t.Fatal("down migration failed")
	}

	// Migrate back up to latest.
	logs = runMigrationJob(t, clientset, "")
	t.Log("Back to latest:", logs)
	if !strings.Contains(logs, "Migration completed successfully") {
		t.Fatal("up migration after down failed")
	}
}
