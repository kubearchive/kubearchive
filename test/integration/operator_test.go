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
	kubearchiveapi "github.com/kubearchive/kubearchive/cmd/operator/api/v1alpha1"
	"github.com/kubearchive/kubearchive/pkg/constants"
	"github.com/kubearchive/kubearchive/test"
	errs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestKACs(t *testing.T) {
	tests := map[string]struct {
		kac      string
		applyNS  int // Number of namespaces in SinkFilter after apply.
		deleteNS int // Number of namespaces in SinkFilter after delete.
	}{
		"emptyKAC": {
			kac:      "testdata/kac-empty.yaml",
			applyNS:  1,
			deleteNS: 0,
		},
		"nonEmptyKAC": {
			kac:      "testdata/kac-with-job.yaml",
			applyNS:  1,
			deleteNS: 0,
		},
	}
	for name, values := range tests {
		t.Run(name, func(t *testing.T) {
			namespace, _ := test.CreateTestNamespace(t, false)

			t.Cleanup(func() {
				// Delete any created API server source created.
				_, dynaclient := test.GetKubernetesClient(t)
				gvr := schema.GroupVersionResource{Group: "sources.knative.dev", Version: "v1", Resource: "apiserversources"}
				_ = dynaclient.Resource(gvr).Namespace(constants.KubeArchiveNamespace).Delete(context.Background(), test.A13eName, metav1.DeleteOptions{})
			})
			test.CreateKAC(t, values.kac, namespace)
			checkResourcesAfterApply(t, namespace, name, values.applyNS)
			test.DeleteKAC(t, namespace)
			checkResourcesAfterDelete(t, namespace, name, values.deleteNS)
		})
	}
}

func checkResourcesAfterApply(t testing.TB, namespace string, testName string, applyNS int) {
	t.Helper()

	clientset, dynaclient := test.GetKubernetesClient(t)

	err := retry.Do(func() error {
		gvr := schema.GroupVersionResource{Group: "sources.knative.dev", Version: "v1", Resource: "apiserversources"}
		_, err := dynaclient.Resource(gvr).Namespace(constants.KubeArchiveNamespace).Get(context.Background(), test.A13eName, metav1.GetOptions{})
		if err != nil {
			return err
		}
		object, err := dynaclient.Resource(kubearchiveapi.SinkFilterGVR).Namespace(constants.KubeArchiveNamespace).Get(context.Background(), constants.SinkFilterResourceName, metav1.GetOptions{})
		if err != nil {
			return err
		}
		sf, err := kubearchiveapi.ConvertObjectToSinkFilter(object)
		if err != nil {
			return err
		} else if len(sf.Spec.Namespaces) != applyNS {
			return fmt.Errorf("Found %d namespaces in SinkFilter, expected %d", len(sf.Spec.Namespaces), applyNS)
		}
		_, err = clientset.CoreV1().ServiceAccounts(constants.KubeArchiveNamespace).Get(context.Background(), test.A13eName, metav1.GetOptions{})
		if err != nil {
			return err
		}
		_, err = clientset.RbacV1().ClusterRoles().Get(context.Background(), test.A13eName, metav1.GetOptions{})
		if err != nil {
			return err
		}
		_, err = clientset.RbacV1().RoleBindings(namespace).Get(context.Background(), test.A13eName, metav1.GetOptions{})
		if err != nil {
			return err
		}
		_, err = clientset.RbacV1().Roles(namespace).Get(context.Background(), constants.KubeArchiveSinkName, metav1.GetOptions{})
		if err != nil {
			return err
		}
		_, err = clientset.RbacV1().RoleBindings(namespace).Get(context.Background(), constants.KubeArchiveSinkName, metav1.GetOptions{})
		if err != nil {
			return err
		}
		_, err = clientset.CoreV1().ServiceAccounts(namespace).Get(context.Background(), constants.KubeArchiveVacuumName, metav1.GetOptions{})
		if err != nil {
			return err
		}
		_, err = clientset.RbacV1().Roles(namespace).Get(context.Background(), constants.KubeArchiveVacuumName, metav1.GetOptions{})
		if err != nil {
			return err
		}
		_, err = clientset.RbacV1().RoleBindings(namespace).Get(context.Background(), constants.KubeArchiveVacuumName, metav1.GetOptions{})
		if err != nil {
			return err
		}
		binding, err := clientset.RbacV1().RoleBindings(constants.KubeArchiveNamespace).Get(context.Background(), constants.KubeArchiveVacuumBroker, metav1.GetOptions{})
		if err != nil {
			return err
		}
		if len(binding.Subjects) != 2 {
			return fmt.Errorf("Found %d subjects in %s/%s RoleBinding after apply, expected 2", len(binding.Subjects), constants.KubeArchiveNamespace, constants.KubeArchiveVacuumBroker)
		}
		cbinding, err := clientset.RbacV1().ClusterRoleBindings().Get(context.Background(), constants.ClusterKubeArchiveConfigClusterRoleBindingName, metav1.GetOptions{})
		if err != nil {
			return err
		}
		if len(cbinding.Subjects) != 2 {
			return fmt.Errorf("Found %d subjects in %s ClusterRoleBinding after apply, expected 2", len(cbinding.Subjects), constants.ClusterKubeArchiveConfigClusterRoleBindingName)
		}
		return nil
	}, retry.Attempts(10), retry.MaxDelay(3*time.Second))

	if err != nil {
		t.Fatal(err)
	}
}

func checkResourcesAfterDelete(t testing.TB, namespace string, testName string, deleteNS int) {
	t.Helper()

	clientset, dynaclient := test.GetKubernetesClient(t)

	err := retry.Do(func() error {
		gvr := schema.GroupVersionResource{Group: "sources.knative.dev", Version: "v1", Resource: "apiserversources"}
		_, err := dynaclient.Resource(gvr).Namespace(constants.KubeArchiveNamespace).Get(context.Background(), test.A13eName, metav1.GetOptions{})
		if err != nil {
			return err
		}
		object, err := dynaclient.Resource(kubearchiveapi.SinkFilterGVR).Namespace(constants.KubeArchiveNamespace).Get(context.Background(), constants.SinkFilterResourceName, metav1.GetOptions{})
		if err != nil {
			return err
		}
		sf, err := kubearchiveapi.ConvertObjectToSinkFilter(object)
		if err != nil {
			return err
		} else if len(sf.Spec.Namespaces) != deleteNS {
			return fmt.Errorf("Found %d namespaces in SinkFilter, expected %d", len(sf.Spec.Namespaces), deleteNS)
		}
		_, err = clientset.CoreV1().ServiceAccounts(constants.KubeArchiveNamespace).Get(context.Background(), test.A13eName, metav1.GetOptions{})
		if err != nil {
			return err
		}
		_, err = clientset.RbacV1().ClusterRoles().Get(context.Background(), test.A13eName, metav1.GetOptions{})
		if err != nil {
			return err
		}
		_, err = clientset.RbacV1().RoleBindings(namespace).Get(context.Background(), test.A13eName, metav1.GetOptions{})
		if !errs.IsNotFound(err) {
			return errors.New("Unexpectedly found Rolebinding " + test.A13eName + " in namespace " + namespace + ".")
		}
		_, err = clientset.RbacV1().Roles(namespace).Get(context.Background(), constants.KubeArchiveSinkName, metav1.GetOptions{})
		if !errs.IsNotFound(err) {
			return errors.New("Unexpectedly found Role " + constants.KubeArchiveSinkName + " in namespace " + namespace + ".")
		}
		_, err = clientset.CoreV1().ServiceAccounts(namespace).Get(context.Background(), constants.KubeArchiveVacuumName, metav1.GetOptions{})
		if !errs.IsNotFound(err) {
			return err
		}
		_, err = clientset.RbacV1().RoleBindings(namespace).Get(context.Background(), constants.KubeArchiveVacuumName, metav1.GetOptions{})
		if !errs.IsNotFound(err) {
			return errors.New("Unexpectedly found Rolebinding " + constants.KubeArchiveVacuumName + " in namespace " + namespace + ".")
		}
		_, err = clientset.RbacV1().Roles(namespace).Get(context.Background(), constants.KubeArchiveVacuumName, metav1.GetOptions{})
		if !errs.IsNotFound(err) {
			return errors.New("Unexpectedly found Role " + constants.KubeArchiveVacuumName + " in namespace " + namespace + ".")
		}
		binding, err := clientset.RbacV1().RoleBindings(constants.KubeArchiveNamespace).Get(context.Background(), constants.KubeArchiveVacuumBroker, metav1.GetOptions{})
		if err != nil {
			return err
		}
		if len(binding.Subjects) != 1 {
			return fmt.Errorf("Found %d subjects in %s/%s RoleBinding after delete, expected 1", len(binding.Subjects), constants.KubeArchiveNamespace, constants.KubeArchiveVacuumBroker)
		}
		cbinding, err := clientset.RbacV1().ClusterRoleBindings().Get(context.Background(), constants.ClusterKubeArchiveConfigClusterRoleBindingName, metav1.GetOptions{})
		if err != nil {
			return err
		}
		if len(cbinding.Subjects) != 1 {
			return fmt.Errorf("Found %d subjects in %s ClusterRoleBinding after delete, expected 1", len(cbinding.Subjects), constants.ClusterKubeArchiveConfigClusterRoleBindingName)
		}
		return nil
	}, retry.Attempts(10), retry.MaxDelay(3*time.Second))

	if err != nil {
		t.Fatal(err)
	}
}

// Test that having both a global and local KubeArchiveConfig works. This is done by archiving Pods
// in the global KubeArchiveConfig and Jobs in the local KubeArchiveConfig. Then run a job and verify
// that the job logs can be retrieved.
func TestGlobalAndLocalKAC(t *testing.T) {
	t.Helper()

	t.Cleanup(func() {
		test.DeleteCKAC(t)
	})

	clientset, _ := test.GetKubernetesClient(t)
	port := test.PortForwardApiServer(t, clientset)
	namespace, token := test.CreateTestNamespace(t, false)

	test.CreateCKAC(t, "testdata/ckac-with-pod.yaml")
	test.CreateKAC(t, "testdata/kac-with-job.yaml", namespace)

	job := test.RunLogGenerator(t, namespace)
	url := fmt.Sprintf("https://localhost:%s/apis/batch/v1/namespaces/%s/jobs/%s/log", port, namespace, job)
	retryErr := retry.Do(func() error {
		body, err := test.GetLogs(t, token.Status.Token, url)
		if err != nil {
			return err
		}

		if len(body) == 0 {
			return errors.New("could not retrieve the job log")
		}
		t.Log("Successfully retrieved logs")

		bodyString := string(body)
		if len(strings.Split(bodyString, "\n")) != 11 {
			return fmt.Errorf("expected 11 lines, currently '%d'. Trying again...", len(strings.Split(bodyString, "\n")))
		}

		return nil
	}, retry.Attempts(30), retry.MaxDelay(2*time.Second))

	if retryErr != nil {
		t.Fatal(retryErr)
	}
}
