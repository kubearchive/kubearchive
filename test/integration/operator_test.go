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
	errs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	sinkName    = test.KACName + "-sink"
	filtersName = "sink-filters"
)

func TestKACs(t *testing.T) {
	tests := map[string]struct {
		resources map[string]any
		applyNS   int // Number of namespaces in ConfigMap after apply.
		deleteNS  int // Number of namespaces in ConfigMap after delete.
	}{
		"emptyKAC": {resources: map[string]any{}, applyNS: 1, deleteNS: 0},
		"nonEmptyKAC": {resources: map[string]any{
			"resources": []map[string]any{
				{
					"selector": map[string]string{
						"apiVersion": "v1",
						"kind":       "Pod",
					},
					"archiveWhen": "true",
					"deleteWhen":  "status.phase == 'Succeeded'",
				},
			}}, applyNS: 1, deleteNS: 0},
	}
	for name, values := range tests {
		t.Run(name, func(t *testing.T) {
			namespace, _ := test.CreateTestNamespace(t, false)

			t.Cleanup(func() {
				// Delete any created API server source created.
				_, dynaclient := test.GetKubernetesClient(t)
				gvr := schema.GroupVersionResource{Group: "sources.knative.dev", Version: "v1", Resource: "apiserversources"}
				_ = dynaclient.Resource(gvr).Namespace(test.K9eNamespace).Delete(context.Background(), test.A13eName, metav1.DeleteOptions{})
			})
			test.CreateKAC(t, namespace, values.resources)
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
		_, getErr := dynaclient.Resource(gvr).Namespace(test.K9eNamespace).Get(context.Background(), test.A13eName, metav1.GetOptions{})
		if testName == "emptyKAC" {
			if !errs.IsNotFound(getErr) {
				return errors.New("Unexpectedly found an ApiServerSource.")
			}
		} else {
			if getErr != nil {
				return getErr
			}
		}
		cm, getErr := clientset.CoreV1().ConfigMaps(test.K9eNamespace).Get(context.Background(), filtersName, metav1.GetOptions{})
		if getErr != nil {
			return getErr
		} else if len(cm.Data) != applyNS {
			return fmt.Errorf("Found %d namespaces in ConfigMap, expected %d", len(cm.Data), applyNS)
		}
		_, getErr = clientset.CoreV1().ServiceAccounts(test.K9eNamespace).Get(context.Background(), test.A13eName, metav1.GetOptions{})
		if getErr != nil {
			return getErr
		}
		_, getErr = clientset.RbacV1().ClusterRoles().Get(context.Background(), test.A13eName, metav1.GetOptions{})
		if getErr != nil {
			return getErr
		}
		_, getErr = clientset.RbacV1().RoleBindings(namespace).Get(context.Background(), test.A13eName, metav1.GetOptions{})
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

func checkResourcesAfterDelete(t testing.TB, namespace string, testName string, deleteNS int) {
	t.Helper()

	clientset, dynaclient := test.GetKubernetesClient(t)

	err := retry.Do(func() error {
		gvr := schema.GroupVersionResource{Group: "sources.knative.dev", Version: "v1", Resource: "apiserversources"}
		_, getErr := dynaclient.Resource(gvr).Namespace(test.K9eNamespace).Get(context.Background(), test.A13eName, metav1.GetOptions{})
		if testName == "emptyKAC" {
			if !errs.IsNotFound(getErr) {
				return getErr
			}
		} else if getErr != nil {
			return getErr
		}
		cm, getErr := clientset.CoreV1().ConfigMaps(test.K9eNamespace).Get(context.Background(), filtersName, metav1.GetOptions{})
		if getErr != nil {
			return getErr
		} else if len(cm.Data) != deleteNS {
			return fmt.Errorf("Found %d namespaces in ConfigMap, expected %d", len(cm.Data), deleteNS)
		}
		_, getErr = clientset.CoreV1().ServiceAccounts(test.K9eNamespace).Get(context.Background(), test.A13eName, metav1.GetOptions{})
		if getErr != nil {
			return getErr
		}
		_, getErr = clientset.RbacV1().ClusterRoles().Get(context.Background(), test.A13eName, metav1.GetOptions{})
		if getErr != nil {
			return getErr
		}
		_, getErr = clientset.RbacV1().RoleBindings(namespace).Get(context.Background(), test.A13eName, metav1.GetOptions{})
		if !errs.IsNotFound(getErr) {
			return errors.New("Unexpectedly found Rolebinding " + test.A13eName + " in namespace " + namespace + ".")
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

// Test that having both a global and local KubeArchiveConfig works. This is done by archiving Pods
// in the global KubeArchiveConfig and Jobs in the local KubeArchiveConfig. Then run a job and verify
// that the job logs can be retrieved.
func TestGlobalAndLocalKAC(t *testing.T) {
	t.Helper()

	globalres := map[string]any{
		"resources": []map[string]any{
			{
				"selector": map[string]string{
					"apiVersion": "v1",
					"kind":       "Pod",
				},
				"archiveWhen": "status.phase == 'Succeeded'",
			},
		},
	}
	localres := map[string]any{
		"resources": []map[string]any{
			{
				"selector": map[string]string{
					"apiVersion": "batch/v1",
					"kind":       "Job",
				},
				"archiveWhen": "has(status.completionTime)",
			},
		},
	}

	t.Cleanup(func() {
		test.DeleteCKAC(t)
	})

	clientset, _ := test.GetKubernetesClient(t)
	port := test.PortForwardApiServer(t, clientset)
	namespace, token := test.CreateTestNamespace(t, false)

	test.CreateCKAC(t, globalres)
	test.CreateKAC(t, namespace, localres)

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
