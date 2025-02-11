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
	"os/exec"
	"strings"
	"testing"

	"github.com/avast/retry-go/v4"
	"github.com/kubearchive/kubearchive/test"
	authenticationv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestLogging(t *testing.T) {
	namespaceName := fmt.Sprintf("test-%s", test.RandomString())
	clientset, _, errClient := test.GetKubernetesClient()
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

	// Install the log-generator.
	namespaceCmd := fmt.Sprintf("--namespace=%s", namespaceName)
	cmd := exec.Command("bash", "../log-generators/cronjobs/install.sh", namespaceCmd, "--num-jobs=1")
	output, errScript := cmd.CombinedOutput()
	if errScript != nil {
		fmt.Println("Could not run the log-generator: ", errScript)
		t.Fatal(errScript)
	}
	fmt.Println("Output: ", string(output))

	url := fmt.Sprintf("https://localhost:8081/apis/batch/v1/namespaces/%s/cronjobs/generate-log-1/log", namespaceName)
	retryErr = retry.Do(func() error {
		body, err := test.GetLogs(&client, token.Status.Token, url)
		if err != nil {
			return err
		}

		if len(body) == 0 {
			return errors.New("could not retrieve the pod log")
		}
		fmt.Println("Successfully retrieved logs")

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
