// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0
//go:build integration

package main

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/avast/retry-go/v4"
	kubearchivev1alpha1 "github.com/kubearchive/kubearchive/cmd/operator/api/v1alpha1"
	"github.com/kubearchive/kubearchive/test"
	"github.com/stretchr/testify/assert"
	authenticationv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	errs "k8s.io/apimachinery/pkg/api/errors"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestPagination(t *testing.T) {
	clientset, dynamicClient, err := test.GetKubernetesClient()
	if err != nil {
		t.Fatal(err)
	}

	namespaceName := fmt.Sprintf("test-%s", test.RandomString())
	_, err = clientset.CoreV1().Namespaces().Create(context.Background(), &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespaceName,
		},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		err = clientset.CoreV1().Namespaces().Delete(context.Background(), namespaceName, metav1.DeleteOptions{})
		if err != nil {
			t.Fatal(err)
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

	kac := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"kind":       "KubeArchiveConfig",
			"apiVersion": fmt.Sprintf("%s/%s", kubearchivev1alpha1.SchemeBuilder.GroupVersion.Group, kubearchivev1alpha1.SchemeBuilder.GroupVersion.Version),
			"metadata": map[string]string{
				"name":      "kubearchive",
				"namespace": namespaceName,
			},
			"spec": map[string]any{
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
			},
		},
	}

	gvr := kubearchivev1alpha1.GroupVersion.WithResource("kubearchiveconfigs")
	_, err = dynamicClient.Resource(gvr).Namespace(namespaceName).Create(context.Background(), kac, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "pod-",
			Namespace:    namespaceName,
		},
		Spec: corev1.PodSpec{
			RestartPolicy: corev1.RestartPolicyNever,
			Containers: []corev1.Container{
				corev1.Container{
					Name:    "fedora",
					Command: []string{"sleep", "1"},
					Image:   "quay.io/fedora/fedora-minimal:latest",
				},
			},
		},
	}

	for _ = range 30 {
		_, err = clientset.CoreV1().Pods(namespaceName).Create(context.Background(), pod, metav1.CreateOptions{})
		if err != nil {
			t.Fatal(err)
		}
	}

	_, err = clientset.RbacV1().RoleBindings(namespaceName).Create(context.Background(), &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: "view-default-test",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      "default",
				Namespace: namespaceName,
			},
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     "view",
		},
	},
		metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}

	token, err := clientset.CoreV1().ServiceAccounts(namespaceName).CreateToken(context.Background(), "default", &authenticationv1.TokenRequest{}, metav1.CreateOptions{})
	if err != nil {
		fmt.Printf("could not create a token, %s", err)
		t.Fatal(err)
	}

	clientHTTP := http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
	}

	// Forward api service port
	pods, err := clientset.CoreV1().Pods("kubearchive").List(context.Background(), metav1.ListOptions{LabelSelector: "app=kubearchive-api-server"})
	if err != nil {
		t.Fatal(err)
	}

	var portForward chan struct{}
	var errPortForward error
	retryErr := retry.Do(func() error {
		portForward, errPortForward = test.PortForward([]string{"8081:8081"}, pods.Items[0].Name, "kubearchive")
		if errPortForward != nil {
			return errPortForward
		}
		return nil
	}, retry.Attempts(3))

	if retryErr != nil {
		t.Fatal(errPortForward)
	}
	defer close(portForward)

	url := fmt.Sprintf("https://localhost:8081/api/v1/namespaces/%s/pods", namespaceName)
	var list *unstructured.UnstructuredList
	var getUrlErr error
	err = retry.Do(func() error {
		list, getUrlErr = test.GetUrl(&clientHTTP, token.Status.Token, url)
		if getUrlErr != nil {
			t.Fatal(getUrlErr)
		}
		// We want to wait until everything is stored in the DB to avoid out of order inserts
		if len(list.Items) >= 30 {
			return nil
		}
		return errors.New("could not retrieve Pods from the API")
	}, retry.Attempts(240), retry.MaxDelay(4*time.Second))

	if err != nil {
		t.Fatal(err)
	}

	url = fmt.Sprintf("https://localhost:8081/api/v1/namespaces/%s/pods?limit=10", namespaceName)
	initList, getUrlErrInitList := test.GetUrl(&clientHTTP, token.Status.Token, url)
	if getUrlErrInitList != nil {
		t.Fatal(getUrlErrInitList)
	}
	assert.Equal(t, 10, len(initList.Items))

	url = fmt.Sprintf("https://localhost:8081/api/v1/namespaces/%s/pods?limit=10&continue=%s", namespaceName, initList.GetContinue())
	continueList, getUrlErrContinueList := test.GetUrl(&clientHTTP, token.Status.Token, url)
	if getUrlErrContinueList != nil {
		t.Fatal(getUrlErrContinueList)
	}
	assert.Equal(t, 10, len(continueList.Items))

	url = fmt.Sprintf("https://localhost:8081/api/v1/namespaces/%s/pods?limit=20", namespaceName)
	allList, getUrlErrAllList := test.GetUrl(&clientHTTP, token.Status.Token, url)
	if getUrlErrAllList != nil {
		t.Fatal(getUrlErrAllList)
	}
	assert.Equal(t, 20, len(allList.Items))

	var listNames []string
	for _, item := range initList.Items {
		listNames = append(listNames, item.GetName())
	}

	var continueListNames []string
	for _, item := range continueList.Items {
		continueListNames = append(continueListNames, item.GetName())
	}
	assert.NotContains(t, continueListNames, listNames)

	var allListNames []string
	for _, item := range allList.Items {
		allListNames = append(allListNames, item.GetName())
	}
	assert.Equal(t, allListNames, append(listNames, continueListNames...))
}
