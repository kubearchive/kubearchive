// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0
//go:build integration

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	kubearchiveapi "github.com/kubearchive/kubearchive/cmd/operator/api/v1"
	"github.com/kubearchive/kubearchive/test"
	"github.com/stretchr/testify/assert"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/kubernetes"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	namespaceVacuumConfigName = "namespace-vacuum-test"
)

func TestNamespaceVacuum(t *testing.T) {
	t.Helper()

	tests := map[string]struct {
		expected  string
		ckac      string
		kac       string
		vacuumRes string
	}{
		"no-resources": {
			expected:  "testdata/nvac-no-resources.txt",
			ckac:      "testdata/ckac-with-pod.yaml",
			kac:       "testdata/kac-with-job.yaml",
			vacuumRes: "testdata/vac-no-resources.yaml",
		},
		"ckac-resource": {
			expected:  "testdata/nvac-ckac-resource.txt",
			ckac:      "testdata/ckac-with-pod.yaml",
			kac:       "testdata/kac-with-job.yaml",
			vacuumRes: "testdata/vac-pod-resource.yaml",
		},
		"kac-resource": {
			expected:  "testdata/nvac-kac-resource.txt",
			ckac:      "testdata/ckac-with-pod.yaml",
			kac:       "testdata/kac-with-job.yaml",
			vacuumRes: "testdata/vac-job-resource.yaml",
		},
		"all-resources": {
			expected:  "testdata/nvac-all-resources.txt",
			ckac:      "testdata/ckac-with-pod.yaml",
			kac:       "testdata/kac-with-job.yaml",
			vacuumRes: "testdata/vac-job-pod-resources.yaml",
		},
	}

	for name, values := range tests {
		t.Run(name, func(t *testing.T) {

			t.Cleanup(func() {
				test.DeleteCKAC(t)
			})

			clientset, _ := test.GetKubernetesClient(t)
			namespace, _ := test.CreateTestNamespace(t, false)

			// Run the job before creating the KAC/CKAC so that the deleteWhen can just us the existence of
			// completionTime for the vacuum. No waiting!

			jobName := test.RunLogGenerator(t, namespace)
			test.WaitForJob(t, clientset, namespace, jobName)

			test.CreateCKAC(t, values.ckac)
			test.CreateKAC(t, values.kac, namespace)

			nvc := loadNVC(t, "testdata/nvc-empty.yaml", namespace, namespaceVacuumConfigName)
			nvc.Spec = loadNamespaceVacuumConfigSpec(t, values.vacuumRes)
			createNamespaceVacuumConfig(t, nvc, namespace)

			vjobName := runNamespaceVacuum(t, clientset, namespace)
			results := test.GetVacuumResults(t, clientset, namespace, vjobName)

			expected := test.ReadExpected(t, values.expected)

			assert.Equal(t, expected, results)
		})
	}
}

func createNamespaceVacuumConfig(t testing.TB, nvc *kubearchiveapi.NamespaceVacuumConfig, namespace string) {
	t.Logf("Creating namespace vacuum config '%s/%s'", nvc.Namespace, nvc.Name)

	_, dynamicClient := test.GetKubernetesClient(t)

	data, err := runtime.DefaultUnstructuredConverter.ToUnstructured(nvc)
	if err != nil {
		t.Log("Could not convert to unstructured")
		t.Fatal(err)
	}
	obj := &unstructured.Unstructured{Object: data}

	gvr := kubearchiveapi.GroupVersion.WithResource("namespacevacuumconfigs")
	_, err = dynamicClient.Resource(gvr).Namespace(namespace).Create(context.Background(), obj, metav1.CreateOptions{})
	if err != nil {
		t.Log("Could not create NamespaceVacuumConfig")
		t.Fatal(err)
	}
}

func runNamespaceVacuum(t testing.TB, clientset *kubernetes.Clientset, namespace string) string {
	name := fmt.Sprintf("namespace-vacuum-%s", test.RandomString())
	t.Logf("Running job '%s/%s'", namespace, name)
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					ServiceAccountName: "kubearchive-vacuum",
					RestartPolicy:      corev1.RestartPolicyNever,
					Containers: []corev1.Container{
						{
							Name:    "vacuum",
							Command: []string{"/ko-app/vacuum", "--config", namespaceVacuumConfigName},
							Image:   "kind.local/vacuum:latest-build",
							Env: []corev1.EnvVar{
								{
									Name:  "NAMESPACE",
									Value: namespace,
								},
							},
						},
					},
				},
			},
		},
	}

	_, err := clientset.BatchV1().Jobs(namespace).Create(context.Background(), job, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}

	test.WaitForJob(t, clientset, namespace, name)
	return name
}

func loadNVC(t testing.TB, filename string, namespace string, name string) *kubearchiveapi.NamespaceVacuumConfig {
	object := test.ReadFileIntoUnstructured(t, filename)
	nvc, err := kubearchiveapi.ConvertUnstructuredToNamespaceVacuumConfig(object)
	if err != nil {
		t.Fatal("unable to convert to NamespaceVacuumConfig:", err)
	}
	nvc.Namespace = namespace
	nvc.Name = name
	return nvc
}

func loadNamespaceVacuumConfigSpec(t testing.TB, filename string) kubearchiveapi.NamespaceVacuumConfigSpec {
	object := test.ReadFileIntoUnstructured(t, filename)
	bytes, err := object.MarshalJSON()
	if err != nil {
		t.Fatal("unable to marshal spec:", err)
	}

	spec := kubearchiveapi.NamespaceVacuumConfigSpec{}

	if err := json.Unmarshal(bytes, &spec); err != nil {
		t.Fatal("unable to unmarshal spec:", err)
	}
	return spec
}
