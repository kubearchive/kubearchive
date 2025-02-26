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
	"github.com/kubearchive/kubearchive/test"
	authenticationv1 "k8s.io/api/authentication/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	errs "k8s.io/apimachinery/pkg/api/errors"
	kubearchivev1alpha1 "github.com/kubearchive/kubearchive/cmd/operator/api/v1alpha1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestLogging(t *testing.T) {
	namespaceName := fmt.Sprintf("test-%s", test.RandomString())
	t.Log("Running in namespace "+namespaceName)
	clientset, dynamicClient, errClient := test.GetKubernetesClient()
	if errClient != nil {
		t.Fatal(errClient)
	}

	pods, err := clientset.CoreV1().Pods("kubearchive").List(context.Background(), metav1.ListOptions{
		LabelSelector: "app=kubearchive-api-server",
		FieldSelector: "status.phase=Running",
	})
	fmt.Println(fmt.Sprintf("Pod to forward: %s", pods.Items[0].Name))
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
		t.Fatal(retryErr)
	}

	defer close(portForward)

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
							"apiVersion": "batch/v1",
							"kind":       "Job",
						},
						"archiveWhen": "has(status.completionTime)",
					},
					{
						"selector": map[string]string{
							"apiVersion": "v1",
							"kind":       "Pod",
						},
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

	token, tokenErr := clientset.CoreV1().ServiceAccounts(namespaceName).CreateToken(context.Background(), "default", &authenticationv1.TokenRequest{}, metav1.CreateOptions{})
	if tokenErr != nil {
		fmt.Printf("could not create a token, %s", tokenErr)
		t.Fatal(tokenErr)
	}

	client := http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
	}

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name: "generate-log-1",
			Namespace:    namespaceName,
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec {
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					Containers: []corev1.Container{
						corev1.Container{
							Name:    "flog",
							Command: []string{"flog", "-n", "10", "-d", "1ms"},
							Image:   "quay.io/kubearchive/mingrammer/flog",
						},
					},
				},
			},
		},
	}

	_, err = clientset.BatchV1().Jobs(namespaceName).Create(context.Background(), job, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}

	url := fmt.Sprintf("https://localhost:8081/apis/batch/v1/namespaces/%s/jobs/generate-log-1/log", namespaceName)
	retryErr = retry.Do(func() error {
		body, err := test.GetLogs(&client, token.Status.Token, url)
		if err != nil {
			return err
		}

		if len(body) == 0 {
			return errors.New("could not retrieve the job log")
		}
		fmt.Println("Successfully retrieved logs")

		bodyString := string(body)
		if len(strings.Split(bodyString, "\n")) != 11 {
			return fmt.Errorf("expected 11 lines, currently '%d'. Trying again...", len(strings.Split(bodyString, "\n")))
		}

		return nil
	}, retry.Attempts(60), retry.MaxDelay(2*time.Second))

	if retryErr != nil {
		t.Fatal(retryErr)
	}
}
