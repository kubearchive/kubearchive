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
	kubearchivev1alpha1 "github.com/kubearchive/kubearchive/cmd/operator/api/v1alpha1"
	"github.com/kubearchive/kubearchive/test"
	corev1 "k8s.io/api/core/v1"
	errs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type Test int

const (
	emptyKAC Test = iota
	nonEmptyKAC
)

const (
	kacName     = "kubearchive"
	a13eName    = kacName + "-a13e"
	sinkName    = kacName + "-sink"
	filtersName = "sink-filters"
)

func TestEmptyKAC(t *testing.T) {
	runTest(t, emptyKAC, map[string]any{})
}

func TestNonEmptyKAC(t *testing.T) {
	resources := map[string]any{
		"resources": []map[string]any{
			{
				"selector": map[string]string{
					"apiVersion": "v1",
					"kind":       "Pod",
				},
				"archiveWhen": "true",
				"deleteWhen":  "status.phase == 'Succeeded'",
			},
		},
	}
	runTest(t, nonEmptyKAC, resources)
}

func runTest(t testing.TB, testName Test, resources map[string]any) {
	t.Helper()

	clientset, dynaclient, errClient := test.GetKubernetesClient()
	if errClient != nil {
		t.Fatal(errClient)
	}

	// Create test namespace
	namespaceName := fmt.Sprintf("test-%s", test.RandomString())
	t.Log("Running test in namespace "+namespaceName)
	_, errNamespace := clientset.CoreV1().Namespaces().Create(context.Background(), &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespaceName,
		},
	}, metav1.CreateOptions{})
	if errNamespace != nil {
		t.Fatal(errNamespace)
	}
	// This register the function.
	t.Cleanup(func() {
		// delete the test namespace
		errNamespace = clientset.CoreV1().Namespaces().Delete(context.Background(), namespaceName, metav1.DeleteOptions{})
		if errNamespace != nil {
			t.Fatal(errNamespace)
		}

		retryErr := retry.Do(func() error {
			_, getErr := clientset.CoreV1().Namespaces().Get(context.Background(), namespaceName, metav1.GetOptions{})
			if !errs.IsNotFound(getErr) {
				return errors.New("Waiting for namespace "+namespaceName+" to be deleted")
			}
			return nil
		}, retry.Attempts(10), retry.MaxDelay(3*time.Second))

		if retryErr != nil {
			t.Log(retryErr)
		}
	})

	kac := newKAC(namespaceName, resources)
	gvr := kubearchivev1alpha1.GroupVersion.WithResource("kubearchiveconfigs")
	_, err := dynaclient.Resource(gvr).Namespace(namespaceName).Create(context.Background(), kac, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}

	checkResourcesAfterApply(t, namespaceName, testName)

	err = dynaclient.Resource(gvr).Namespace(namespaceName).Delete(context.Background(), kacName, metav1.DeleteOptions{})
	if err != nil {
		t.Fatal(err)
	}

	checkResourcesAfterDelete(t, namespaceName, testName)
}

func checkResourcesAfterApply(t testing.TB, namespace string, testName Test) {
	t.Helper()

	clientset, dynaclient, _ := test.GetKubernetesClient()

	err := retry.Do(func() error {
		gvr := schema.GroupVersionResource{Group: "sources.knative.dev", Version: "v1", Resource: "apiserversources"}
		_, getErr := dynaclient.Resource(gvr).Namespace(test.K9eNamespace).Get(context.Background(), a13eName, metav1.GetOptions{})
		if testName == emptyKAC {
			if !errs.IsNotFound(getErr) {
				return errors.New("Unexpectedly found an ApiServerSource.")
			}
		} else {
			if getErr != nil {
				return getErr
			}
		}
		_, getErr = clientset.CoreV1().ConfigMaps(test.K9eNamespace).Get(context.Background(), filtersName, metav1.GetOptions{})
		if getErr != nil {
			return getErr
		}
		_, getErr = clientset.CoreV1().ServiceAccounts(test.K9eNamespace).Get(context.Background(), a13eName, metav1.GetOptions{})
		if getErr != nil {
			return getErr
		}
		_, getErr = clientset.RbacV1().ClusterRoles().Get(context.Background(), a13eName, metav1.GetOptions{})
		if getErr != nil {
			return getErr
		}
		_, getErr = clientset.RbacV1().RoleBindings(namespace).Get(context.Background(), a13eName, metav1.GetOptions{})
		if getErr != nil {
			return getErr
		}
		_, getErr = clientset.RbacV1().Roles(namespace).Get(context.Background(), sinkName, metav1.GetOptions{})
		if getErr != nil {
			return getErr
		}
		_, getErr = clientset.RbacV1().RoleBindings(namespace).Get(context.Background(), sinkName, metav1.GetOptions{})
		if getErr != nil {
			return getErr
		}
		return nil
	}, retry.Attempts(10), retry.MaxDelay(3*time.Second))

	if err != nil {
		t.Fatal(err)
	}
}

func checkResourcesAfterDelete(t testing.TB, namespace string, testName Test) {
	t.Helper()

	clientset, dynaclient, _ := test.GetKubernetesClient()

	err := retry.Do(func() error {
		gvr := schema.GroupVersionResource{Group: "sources.knative.dev", Version: "v1", Resource: "apiserversources"}
		_, getErr := dynaclient.Resource(gvr).Namespace(test.K9eNamespace).Get(context.Background(), a13eName, metav1.GetOptions{})
		if testName == emptyKAC || testName == nonEmptyKAC {
			if !errs.IsNotFound(getErr) {
				return errors.New("Unexpectedly found an ApiServerSource.")
			}
		} else {
			if getErr != nil {
				return getErr
			}
		}
		_, getErr = clientset.CoreV1().ConfigMaps(test.K9eNamespace).Get(context.Background(), filtersName, metav1.GetOptions{})
		if getErr != nil {
			return getErr
		}
		_, getErr = clientset.CoreV1().ServiceAccounts(test.K9eNamespace).Get(context.Background(), a13eName, metav1.GetOptions{})
		if getErr != nil {
			return getErr
		}
		_, getErr = clientset.RbacV1().ClusterRoles().Get(context.Background(), a13eName, metav1.GetOptions{})
		if getErr != nil {
			return getErr
		}
		_, getErr = clientset.RbacV1().RoleBindings(namespace).Get(context.Background(), a13eName, metav1.GetOptions{})
		if !errs.IsNotFound(getErr) {
			return errors.New("Unexpectedly found Rolebinding " + a13eName + " in namespace " + namespace + ".")
		}
		_, getErr = clientset.RbacV1().Roles(namespace).Get(context.Background(), sinkName, metav1.GetOptions{})
		if !errs.IsNotFound(getErr) {
			return errors.New("Unexpectedly found Role " + sinkName + " in namespace " + namespace + ".")
		}
		_, getErr = clientset.RbacV1().RoleBindings(namespace).Get(context.Background(), sinkName, metav1.GetOptions{})
		if !errs.IsNotFound(getErr) {
			return errors.New("Unexpectedly found Rolebinding " + sinkName + " in namespace " + namespace + ".")
		}
		return nil
	}, retry.Attempts(10), retry.MaxDelay(3*time.Second))

	if err != nil {
		t.Fatal(err)
	}
}

func newKAC(namespace string, resources map[string]any) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"kind": "KubeArchiveConfig",
			"apiVersion": fmt.Sprintf("%s/%s",
				kubearchivev1alpha1.SchemeBuilder.GroupVersion.Group,
				kubearchivev1alpha1.SchemeBuilder.GroupVersion.Version),
			"metadata": map[string]string{
				"name":      kacName,
				"namespace": namespace,
			},
			"spec": resources,
		},
	}
}
