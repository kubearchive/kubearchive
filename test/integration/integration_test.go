// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0
//go:build integration

package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/avast/retry-go/v4"
	"github.com/kubearchive/kubearchive/test"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	errs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TestKubeArchiveDeployments is redundant with the kubectl rollout status from the hack/quick-install.sh
// ,but it serves as a valid integration test, not a dummy that is not testing anything real.
func TestAllDeploymentsReady(t *testing.T) {
	t.Parallel()
	client, _, err := test.GetKubernetesClient()
	if err != nil {
		t.Fatal(err)
	}

	retryErr := retry.Do(func() error {
		deployments, errList := client.AppsV1().Deployments("kubearchive").List(context.Background(), metav1.ListOptions{})
		if errList != nil {
			return fmt.Errorf("Failed to get Deployments from the 'kubearchive' namespace: %w", errList)
		}

		if len(deployments.Items) == 0 {
			return errors.New("No deployments found in the 'kubearchive' namespace, something went wrong.")
		}

		areAllReady := true
		for _, deployment := range deployments.Items {
			t.Logf("Deployment '%s' has '%d' ready replicas", deployment.Name, deployment.Status.ReadyReplicas)
			areAllReady = areAllReady && deployment.Status.ReadyReplicas >= 1
		}

		if areAllReady {
			t.Log("All deployments ready.")
			return nil
		}

		return errors.New("Timed out while waiting for deployments to be ready.")
	})

	if retryErr != nil {
		t.Fatal(retryErr)
	}
}

func TestNormalOperation(t *testing.T) {
	t.Parallel()
	clientset, _, errClient := test.GetKubernetesClient()
	if errClient != nil {
		t.Fatal(errClient)
	}

	// Forward api service port
	port, closePort := test.PortForwardApiServer(t, clientset)
	t.Cleanup(closePort)

	// Create test namespace
	namespaceName := fmt.Sprintf("test-%s", test.RandomString())
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
				return errors.New("Waiting for namespace " + namespaceName + " to be deleted")
			}
			return nil
		}, retry.Attempts(10), retry.MaxDelay(3*time.Second))

		if retryErr != nil {
			t.Log(retryErr)
		}
	})

	// Call the log-generator.
	numJobs := 1
	test.RunLogGenerators(t, test.CronJobGenerator, namespaceName, numJobs)

	_, errRoleBinding := clientset.RbacV1().RoleBindings(namespaceName).Create(context.Background(), &rbacv1.RoleBinding{
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

	if errRoleBinding != nil {
		t.Fatal(errRoleBinding)
	}

	// Retrieve the objects from the DB using the API.

	// Set up the http client with cert
	secret, errSecret := clientset.CoreV1().Secrets("kubearchive").Get(context.Background(), "kubearchive-api-server-tls", metav1.GetOptions{})
	if errSecret != nil {
		t.Fatal(errSecret)
	}

	token := test.GetSAToken(t, clientset, namespaceName)

	caCertPool := x509.NewCertPool()
	appendCert := caCertPool.AppendCertsFromPEM(secret.Data["ca.crt"])
	if !appendCert {
		t.Log("could not append the CA cert")
		t.Fatal(errors.New("could not append the CA cert"))
	}
	clientHTTP := http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs: caCertPool,
			},
		},
	}

	url := fmt.Sprintf("https://localhost:%s/apis/batch/v1/namespaces/%s/cronjobs", port, namespaceName)
	retryErr := retry.Do(func() error {
		list, getUrlErr := test.GetUrl(t, &clientHTTP, token.Status.Token, url)
		if getUrlErr != nil {
			return getUrlErr
		}

		if len(list.Items) == 1 {
			return nil
		}
		return errors.New("could not retrieved a CronJob from the API")
	}, retry.Attempts(20))

	if retryErr != nil {
		t.Fatal(retryErr)
	}
}
