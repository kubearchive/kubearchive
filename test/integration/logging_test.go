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
	"strings"
	"testing"
	"time"

	"github.com/avast/retry-go/v4"
	kubearchivev1alpha1 "github.com/kubearchive/kubearchive/cmd/operator/api/v1alpha1"
	"github.com/kubearchive/kubearchive/test"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	errs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestLogging(t *testing.T) {
	t.Parallel()
	namespaceName := fmt.Sprintf("test-%s", test.RandomString())
	clientset, _, errClient := test.GetKubernetesClient()
	if errClient != nil {
		t.Fatal(errClient)
	}

	port, closePort := test.PortForwardApiServer(t, clientset)
	t.Cleanup(closePort)

	_, errNamespace := clientset.CoreV1().Namespaces().Create(context.Background(), &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespaceName,
		},
	}, metav1.CreateOptions{})
	if errNamespace != nil {
		t.Fatal(errNamespace)
	}

	t.Cleanup(func() {
		errNamespace = clientset.CoreV1().Namespaces().Delete(context.Background(), namespaceName, metav1.DeleteOptions{})
		if errNamespace != nil {
			t.Fatal(errNamespace)
		}

		retryErr := retry.Do(func() error {
			_, getErr := clientset.CoreV1().Namespaces().Get(context.Background(), namespaceName, metav1.GetOptions{})
			if !errs.IsNotFound(getErr) {
				return errors.New("Waiting for namespace " + namespaceName + " to be deleted")
			}
			return nil
		}, retry.Attempts(10), retry.MaxDelay(3*time.Second))

		if retryErr != nil {
			t.Log(retryErr)
		}
	})

	_, roleBindingErr := clientset.RbacV1().RoleBindings(namespaceName).Create(context.Background(), &rbacv1.RoleBinding{
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
	if roleBindingErr != nil {
		t.Fatal(roleBindingErr)
	}

	token := test.GetSAToken(t, clientset, namespaceName)

	client := http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
	}

	// Install the log-generator.
	numRuns := 1
	test.RunLogGenerators(t, test.CronJobGenerator, namespaceName, numRuns)

	url := fmt.Sprintf("https://localhost:%s/apis/batch/v1/namespaces/%s/cronjobs/generate-log-1/log", port, namespaceName)
	retryErr := retry.Do(func() error {
		body, err := test.GetLogs(t, &client, token.Status.Token, url)
		if err != nil {
			return err
		}

		if len(body) == 0 {
			return errors.New("could not retrieve the pod log")
		}
		t.Log("Successfully retrieved logs")

		bodyString := string(body)
		if len(strings.Split(bodyString, "\n")) != 1025 {
			return fmt.Errorf("expected 1025 lines, currently '%d'. Trying again...", len(strings.Split(bodyString, "\n")))
		}

		return nil
	}, retry.Attempts(20))

	if retryErr != nil {
		t.Fatal(retryErr)
	}
}

func TestDefaultContainer(t *testing.T) {
	t.Parallel()
	namespaceName := fmt.Sprintf("test-%s", test.RandomString())
	clientset, dynamicClient, errClient := test.GetKubernetesClient()
	if errClient != nil {
		t.Fatal(errClient)
	}

	port, closePort := test.PortForwardApiServer(t, clientset)
	t.Cleanup(closePort)

	_, errNamespace := clientset.CoreV1().Namespaces().Create(context.Background(), &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespaceName,
		},
	}, metav1.CreateOptions{})
	if errNamespace != nil {
		t.Fatal(errNamespace)
	}

	t.Cleanup(func() {
		errNamespace = clientset.CoreV1().Namespaces().Delete(context.Background(), namespaceName, metav1.DeleteOptions{})
		if errNamespace != nil {
			t.Fatal(errNamespace)
		}
	})

	_, roleBindingErr := clientset.RbacV1().RoleBindings(namespaceName).Create(context.Background(), &rbacv1.RoleBinding{
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
	if roleBindingErr != nil {
		t.Fatal(roleBindingErr)
	}

	token := test.GetSAToken(t, clientset, namespaceName)

	client := http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
	}

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
					},
				},
			},
		},
	}

	test.CreateKAC(t, clientset, dynamicClient, kac, namespaceName)

	// Create a pod
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "defaults-to-first",
		},
		Spec: corev1.PodSpec{
			RestartPolicy: corev1.RestartPolicyNever,
			Containers: []corev1.Container{
				{
					Name:    "first",
					Image:   "quay.io/fedora/fedora:latest",
					Command: []string{"echo", "-n", "I'm the container called first."},
				},
				{
					Name:    "second",
					Image:   "quay.io/fedora/fedora:latest",
					Command: []string{"echo", "-n", "I'm the container called second."},
				},
			},
		},
	}
	_, err := clientset.CoreV1().Pods(namespaceName).Create(context.Background(), pod, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}

	pod = &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "wants-second",
			Annotations: map[string]string{
				"kubectl.kubernetes.io/default-container": "second",
			}},
		Spec: corev1.PodSpec{
			RestartPolicy: corev1.RestartPolicyNever,
			Containers: []corev1.Container{
				{
					Name:    "first",
					Image:   "quay.io/fedora/fedora:latest",
					Command: []string{"echo", "-n", "I'm the container called first."},
				},
				{
					Name:    "second",
					Image:   "quay.io/fedora/fedora:latest",
					Command: []string{"echo", "-n", "I'm the container called second."},
				},
			},
		},
	}
	_, err = clientset.CoreV1().Pods(namespaceName).Create(context.Background(), pod, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}

	url := fmt.Sprintf("https://localhost:%s/api/v1/namespaces/%s/pods/defaults-to-first/log", port, namespaceName)
	retryErr := retry.Do(func() error {
		body, err := test.GetLogs(t, &client, token.Status.Token, url)
		if err != nil {
			return err
		}

		if len(body) == 0 {
			return errors.New("could not retrieve the pod log")
		}
		t.Log("Successfully retrieved logs")

		bodyString := string(body)
		if strings.Trim(bodyString, "\n") != "I'm the container called first." {
			t.Log("log does not match")
			return fmt.Errorf("log does not match the expected 'I'm the container called first.'")
		}

		return nil
	}, retry.Attempts(200), retry.MaxDelay(3*time.Second))

	if retryErr != nil {
		t.Fatal(retryErr)
	}

	url = fmt.Sprintf("https://localhost:%s/api/v1/namespaces/%s/pods/wants-second/log", port, namespaceName)
	retryErr = retry.Do(func() error {
		body, err := test.GetLogs(t, &client, token.Status.Token, url)
		if err != nil {
			return err
		}

		if len(body) == 0 {
			return errors.New("could not retrieve the pod log")
		}
		t.Log("Successfully retrieved logs")

		bodyString := string(body)
		if strings.Trim(bodyString, "\n") != "I'm the container called second." {
			t.Log("log does not match")
			return fmt.Errorf("log does not match the expected 'I'm the container called second.'")
		}

		return nil
	}, retry.Attempts(200), retry.MaxDelay(3*time.Second))

	if retryErr != nil {
		t.Fatal(retryErr)
	}
}
