// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0
//go:build integration

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/avast/retry-go/v5"
	kubearchiveapi "github.com/kubearchive/kubearchive/cmd/operator/api/v1"
	"github.com/kubearchive/kubearchive/pkg/constants"
	"github.com/kubearchive/kubearchive/test"
	"github.com/stretchr/testify/assert"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	errs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
)

const (
	clusterVacuumConfigName = "cluster-vacuum-test"
)

func TestClusterVacuum(t *testing.T) {
	t.Helper()

	type resourceType struct {
		filename string
		addNS    bool
	}
	tests := map[string]struct {
		expected  string
		ckac      string
		kac       string
		vacuumRes []resourceType
		allRes    string
	}{
		"cvac-no-resources": {
			expected:  "testdata/cvac-no-resources.txt",
			ckac:      "testdata/ckac-with-pod.yaml",
			kac:       "testdata/kac-with-job.yaml",
			vacuumRes: []resourceType{{filename: "testdata/vac-no-resources.yaml", addNS: true}},
			allRes:    "",
		},
		"cvac-ckac-resource": {
			expected:  "testdata/cvac-ckac-resource.txt",
			ckac:      "testdata/ckac-with-pod.yaml",
			kac:       "testdata/kac-with-job.yaml",
			vacuumRes: []resourceType{{filename: "testdata/vac-pod-resource.yaml", addNS: true}},
			allRes:    "",
		},
		"cvac-kac-resource": {
			expected:  "testdata/cvac-kac-resource.txt",
			ckac:      "testdata/ckac-with-pod.yaml",
			kac:       "testdata/kac-with-job.yaml",
			vacuumRes: []resourceType{{filename: "testdata/vac-job-resource.yaml", addNS: true}},
			allRes:    "",
		},
		"cvac-both-resources": {
			expected:  "testdata/cvac-both-resources.txt",
			ckac:      "testdata/ckac-with-pod.yaml",
			kac:       "testdata/kac-with-job.yaml",
			vacuumRes: []resourceType{{filename: "testdata/vac-job-pod-resources.yaml", addNS: true}},
			allRes:    "",
		},
		"cvac-two-namespaces": {
			expected: "testdata/cvac-two-namespaces.txt",
			ckac:     "testdata/ckac-with-pod.yaml",
			kac:      "testdata/kac-with-job.yaml",
			vacuumRes: []resourceType{
				{filename: "testdata/vac-pod-resource.yaml", addNS: true},
				{filename: "testdata/vac-job-resource.yaml", addNS: true},
			},
			allRes: "",
		},
		"cvac-all-only": {
			expected:  "testdata/cvac-all-only.txt",
			ckac:      "testdata/ckac-with-pod.yaml",
			kac:       "testdata/kac-with-job.yaml",
			vacuumRes: []resourceType{{filename: "", addNS: false}},
			allRes:    "testdata/vac-pod-resource.yaml",
		},
		"cvac-all-one-and-one": {
			expected:  "testdata/cvac-all-one-and-one.txt",
			ckac:      "testdata/ckac-with-pod.yaml",
			kac:       "testdata/kac-with-job.yaml",
			vacuumRes: []resourceType{{filename: "", addNS: false}, {filename: "testdata/vac-job-resource.yaml", addNS: true}},
			allRes:    "testdata/vac-pod-resource.yaml",
		},
		"cvac-keep-last-when-cluster-only": {
			expected:  "testdata/cvac-keep-last-when-cluster-only.txt",
			ckac:      "testdata/ckac-keep-last-when-batch.yaml",
			kac:       "testdata/kac-empty.yaml",
			vacuumRes: []resourceType{{filename: "testdata/vac-job-resource.yaml", addNS: true}},
			allRes:    "",
		},
		"cvac-keep-last-when-override": {
			expected:  "testdata/cvac-keep-last-when-override.txt",
			ckac:      "testdata/ckac-keep-last-when-override.yaml",
			kac:       "testdata/kac-keep-last-when-override.yaml",
			vacuumRes: []resourceType{{filename: "testdata/vac-job-resource.yaml", addNS: true}},
			allRes:    "",
		},
		"cvac-keep-last-when-multiple-clauses": {
			expected:  "testdata/cvac-keep-last-when-multiple-clauses.txt",
			ckac:      "testdata/ckac-keep-last-when-multiple.yaml",
			kac:       "testdata/kac-empty.yaml",
			vacuumRes: []resourceType{{filename: "testdata/vac-job-resource.yaml", addNS: true}},
			allRes:    "",
		},
		"cvac-keep-last-when-count-zero": {
			expected:  "testdata/cvac-keep-last-when-count-zero.txt",
			ckac:      "testdata/ckac-keep-last-when-count-zero.yaml",
			kac:       "testdata/kac-empty.yaml",
			vacuumRes: []resourceType{{filename: "testdata/vac-job-resource.yaml", addNS: true}},
			allRes:    "",
		},
	}

	for name, values := range tests {
		t.Run(name, func(t *testing.T) {

			t.Cleanup(func() {
				test.DeleteCKAC(t)
				deleteClusterVacuumConfig(t)
			})

			clientset, _ := test.GetKubernetesClient(t)

			test.CreateCKAC(t, values.ckac)

			cvc := loadCVC(t, "testdata/cvc-empty.yaml", "cluster-vacuum-test")
			for i, data := range values.vacuumRes {
				namespace, _ := test.CreateTestNamespaceWithName(t, false, fmt.Sprintf("test-%s-%03d", name, i+1))
				test.CreateKAC(t, values.kac, namespace)

				if data.addNS {
					cvc.Spec.Namespaces[namespace] = loadClusterVacuumConfigNamespaceSpec(t, data.filename)
				}

				// For keepLastWhen tests, create multiple jobs to test the functionality
				if strings.Contains(name, "keep-last-when") {
					var jobNames []string
					jobCount := 5
					if strings.Contains(name, "multiple-clauses") {
						jobCount = 8
					} else if strings.Contains(name, "overlapping") {
						jobCount = 6
					}

					for i := 0; i < jobCount; i++ {
						jobName := fmt.Sprintf("vacuum-job-%03d", i+1)
						test.RunLogGeneratorWithLinesWithName(t, namespace, 5, jobName)
						jobNames = append(jobNames, jobName)
						// Sleep for a second to ensure different creation timestamps
						time.Sleep(1 * time.Second)
					}

					// Wait for all jobs to complete
					for _, jobName := range jobNames {
						test.WaitForJob(t, clientset, namespace, jobName)
					}
				} else {
					jobName := test.RunLogGeneratorWithLinesWithName(t, namespace, 10, "vacuum-job-001")
					test.WaitForJob(t, clientset, namespace, jobName)
				}
			}

			if values.allRes != "" {
				cvc.Spec.Namespaces[constants.ClusterVacuumAllNamespaces] = loadClusterVacuumConfigNamespaceSpec(t, values.allRes)
			}

			createClusterVacuumConfig(t, cvc)

			vjobName := runClusterVacuum(t, clientset)
			t.Cleanup(func() {
				deleteClusterVacuumJob(t, vjobName)
			})
			results := test.GetVacuumResults(t, clientset, constants.KubeArchiveNamespace, vjobName)

			expected := test.ReadExpected(t, values.expected)

			assert.Equal(t, expected, results)
		})
	}
}

func createClusterVacuumConfig(t testing.TB, cvc *kubearchiveapi.ClusterVacuumConfig) {
	t.Logf("Creating cluster vacuum config '%s/%s'", cvc.Namespace, cvc.Name)

	_, dynamicClient := test.GetKubernetesClient(t)

	data, err := runtime.DefaultUnstructuredConverter.ToUnstructured(cvc)
	if err != nil {
		t.Log("Could not convert to unstructured")
		t.Fatal(err)
	}
	obj := &unstructured.Unstructured{Object: data}

	gvr := kubearchiveapi.GroupVersion.WithResource("clustervacuumconfigs")
	_, err = dynamicClient.Resource(gvr).Namespace(constants.KubeArchiveNamespace).Create(context.Background(), obj, metav1.CreateOptions{})
	if err != nil {
		t.Log("Could not create ClusterVacuumConfig")
		t.Fatal(err)
	}
}

func deleteClusterVacuumConfig(t testing.TB) {
	t.Logf("Deleting cluster vacuum config '%s/%s'", constants.KubeArchiveNamespace, clusterVacuumConfigName)

	_, dynamicClient := test.GetKubernetesClient(t)

	gvr := kubearchiveapi.GroupVersion.WithResource("clustervacuumconfigs")
	err := dynamicClient.Resource(gvr).Namespace(constants.KubeArchiveNamespace).Delete(context.Background(), clusterVacuumConfigName, metav1.DeleteOptions{})
	if err != nil {
		t.Log("Could not delete ClusterVacuumConfig")
		t.Fatal(err)
	}

	retryErr := retry.New(retry.Attempts(10), retry.MaxDelay(3*time.Second)).Do(func() error {
		_, getErr := dynamicClient.Resource(gvr).Namespace(constants.KubeArchiveNamespace).Get(context.Background(), clusterVacuumConfigName, metav1.GetOptions{})
		if !errs.IsNotFound(getErr) {
			return fmt.Errorf("Waiting for cluster vacuum config '%s/%s' to be deleted", constants.KubeArchiveNamespace, clusterVacuumConfigName)
		}
		return nil
	})

	if retryErr != nil {
		t.Log(retryErr)
	}
}

func deleteClusterVacuumJob(t testing.TB, name string) {
	t.Logf("Deleting cluster vacuum job '%s/%s'", constants.KubeArchiveNamespace, name)

	clientset, _ := test.GetKubernetesClient(t)

	deleteOptions := metav1.DeleteOptions{
		PropagationPolicy: func() *metav1.DeletionPropagation {
			policy := metav1.DeletePropagationBackground
			return &policy
		}(),
	}

	err := clientset.BatchV1().Jobs(constants.KubeArchiveNamespace).Delete(context.Background(), name, deleteOptions)
	if err != nil {
		t.Log("Could not delete cluster vacuum job")
		t.Fatal(err)
	}

	retryErr := retry.New(retry.Attempts(10), retry.MaxDelay(3*time.Second)).Do(func() error {
		_, getErr := clientset.BatchV1().Jobs(constants.KubeArchiveNamespace).Get(context.Background(), name, metav1.GetOptions{})
		if !errs.IsNotFound(getErr) {
			return fmt.Errorf("Waiting for cluster vacuum job '%s/%s' to be deleted", constants.KubeArchiveNamespace, name)
		}
		return nil
	})

	if retryErr != nil {
		t.Log(retryErr)
	}
}

func runClusterVacuum(t testing.TB, clientset *kubernetes.Clientset) string {
	name := fmt.Sprintf("cluster-vacuum-%s", test.RandomString())
	t.Logf("Running job '%s/%s'", constants.KubeArchiveNamespace, name)
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: constants.KubeArchiveNamespace,
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					ServiceAccountName: "kubearchive-cluster-vacuum",
					RestartPolicy:      corev1.RestartPolicyNever,
					Containers: []corev1.Container{
						{
							Name:    "vacuum",
							Command: []string{"/ko-app/vacuum", "--type", "cluster", "--config", clusterVacuumConfigName},
							Image:   "kind.local/vacuum:latest-build",
							Env: []corev1.EnvVar{
								{
									Name:  "KUBEARCHIVE_NAMESPACE",
									Value: constants.KubeArchiveNamespace,
								},
							},
						},
					},
				},
			},
		},
	}

	_, err := clientset.BatchV1().Jobs(constants.KubeArchiveNamespace).Create(context.Background(), job, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}

	test.WaitForJob(t, clientset, constants.KubeArchiveNamespace, name)
	return name
}

func loadCVC(t testing.TB, filename string, name string) *kubearchiveapi.ClusterVacuumConfig {
	object := test.ReadFileIntoUnstructured(t, filename)
	cvc, err := kubearchiveapi.ConvertUnstructuredToClusterVacuumConfig(object)
	if err != nil {
		t.Fatal("unable to convert to ClusterVacuumConfig:", err)
	}
	cvc.Name = name
	return cvc
}

func loadClusterVacuumConfigNamespaceSpec(t testing.TB, filename string) kubearchiveapi.ClusterVacuumConfigNamespaceSpec {
	object := test.ReadFileIntoUnstructured(t, filename)
	bytes, err := object.MarshalJSON()
	if err != nil {
		t.Fatal("unable to marshal spec:", err)
	}

	spec := kubearchiveapi.ClusterVacuumConfigNamespaceSpec{}

	if err := json.Unmarshal(bytes, &spec); err != nil {
		t.Fatal("unable to unmarshal spec:", err)
	}
	return spec
}
