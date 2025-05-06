// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0
//go:build integration

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/avast/retry-go/v4"
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
		"no-resources": {
			expected:  "testdata/cvac-no-resources.json",
			ckac:      "testdata/ckac-with-pod.yaml",
			kac:       "testdata/kac-with-job.yaml",
			vacuumRes: []resourceType{{filename: "testdata/vac-no-resources.yaml", addNS: true}},
			allRes:    "",
		},
		"ckac-resource": {
			expected:  "testdata/cvac-ckac-resource.json",
			ckac:      "testdata/ckac-with-pod.yaml",
			kac:       "testdata/kac-with-job.yaml",
			vacuumRes: []resourceType{{filename: "testdata/vac-pod-resource.yaml", addNS: true}},
			allRes:    "",
		},
		"kac-resource": {
			expected:  "testdata/cvac-kac-resource.json",
			ckac:      "testdata/ckac-with-pod.yaml",
			kac:       "testdata/kac-with-job.yaml",
			vacuumRes: []resourceType{{filename: "testdata/vac-job-resource.yaml", addNS: true}},
			allRes:    "",
		},
		"both-resources": {
			expected:  "testdata/cvac-both-resources.json",
			ckac:      "testdata/ckac-with-pod.yaml",
			kac:       "testdata/kac-with-job.yaml",
			vacuumRes: []resourceType{{filename: "testdata/vac-job-pod-resources.yaml", addNS: true}},
			allRes:    "",
		},
		"two-namespaces": {
			expected: "testdata/cvac-two-namespaces.json",
			ckac:     "testdata/ckac-with-pod.yaml",
			kac:      "testdata/kac-with-job.yaml",
			vacuumRes: []resourceType{
				{filename: "testdata/vac-pod-resource.yaml", addNS: true},
				{filename: "testdata/vac-job-resource.yaml", addNS: true},
			},
			allRes: "",
		},
		"all-only": {
			expected:  "testdata/cvac-ckac-resource.json",
			ckac:      "testdata/ckac-with-pod.yaml",
			kac:       "testdata/kac-with-job.yaml",
			vacuumRes: []resourceType{{filename: "", addNS: false}},
			allRes:    "testdata/vac-pod-resource.yaml",
		},
		"all-one-and-one": {
			expected:  "testdata/cvac-two-namespaces.json",
			ckac:      "testdata/ckac-with-pod.yaml",
			kac:       "testdata/kac-with-job.yaml",
			vacuumRes: []resourceType{{filename: "", addNS: false}, {filename: "testdata/vac-job-resource.yaml", addNS: true}},
			allRes:    "testdata/vac-pod-resource.yaml",
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
			for _, data := range values.vacuumRes {
				namespace, _ := test.CreateTestNamespace(t, false)
				test.CreateKAC(t, values.kac, namespace)

				if data.addNS {
					cvc.Spec.Namespaces[namespace] = loadClusterVacuumConfigNamespaceSpec(t, data.filename)
				}

				jobName := test.RunLogGenerator(t, namespace)
				test.WaitForJob(t, clientset, namespace, jobName)
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

	retryErr := retry.Do(func() error {
		_, getErr := dynamicClient.Resource(gvr).Namespace(constants.KubeArchiveNamespace).Get(context.Background(), clusterVacuumConfigName, metav1.GetOptions{})
		if !errs.IsNotFound(getErr) {
			return fmt.Errorf("Waiting for cluster vacuum config '%s/%s' to be deleted", constants.KubeArchiveNamespace, clusterVacuumConfigName)
		}
		return nil
	}, retry.Attempts(10), retry.MaxDelay(3*time.Second))

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

	retryErr := retry.Do(func() error {
		_, getErr := clientset.BatchV1().Jobs(constants.KubeArchiveNamespace).Get(context.Background(), name, metav1.GetOptions{})
		if !errs.IsNotFound(getErr) {
			return fmt.Errorf("Waiting for cluster vacuum job '%s/%s' to be deleted", constants.KubeArchiveNamespace, name)
		}
		return nil
	}, retry.Attempts(10), retry.MaxDelay(3*time.Second))

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
