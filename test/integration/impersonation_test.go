// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0
//go:build integration

package main

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/avast/retry-go/v4"
	"github.com/kubearchive/kubearchive/test"
	authenticationv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func TestImpersonation(t *testing.T) {
	tests := []struct {
		name                 string
		impersonationEnabled bool
		expectedAuth         bool
	}{
		{
			name:                 "impersonation without the feature flag enabled",
			impersonationEnabled: false,
			expectedAuth:         false,
		},
		{
			name:                 "impersonation with the feature flag enabled",
			impersonationEnabled: true,
			expectedAuth:         true,
		},
	}

	clientset, _ := test.GetKubernetesClient(t)
	t.Cleanup(func() {
		// The default state of impersonation is disabled
		setImpersonation(t, clientset, false)
	})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setImpersonation(t, clientset, tt.impersonationEnabled)
			port := test.PortForwardApiServer(t, clientset)
			namespaceName, _ := test.CreateTestNamespace(t, false)
			impersonatedUser := fmt.Sprintf("system:serviceaccount:%s:default", namespaceName)
			token := createImpersonatorSA(t, clientset, "impersonator", namespaceName, impersonatedUser)

			// Retrieve something
			url := fmt.Sprintf("https://localhost:%s/apis/batch/v1/namespaces/%s/jobs", port, namespaceName)
			impersonateHeaders := map[string][]string{
				authenticationv1.ImpersonateUserHeader: {impersonatedUser},
			}
			var auth bool
			retryErr := retry.Do(func() error {
				_, getUrlErr := test.GetUrl(t, token.Status.Token, url, impersonateHeaders)
				if getUrlErr != nil {
					if getUrlErr == test.ErrUnauth {
						return nil
					}
					auth = true
					return getUrlErr
				}
				auth = true
				return nil
			}, retry.Attempts(20), retry.MaxDelay(2*time.Second))

			if retryErr != nil {
				t.Fatal(retryErr)
			}
			if auth != tt.expectedAuth {
				t.Fatalf("Expected auth to be %v but got %v", tt.expectedAuth, auth)
			}
		})
	}

}

func setImpersonation(t *testing.T, clientset *kubernetes.Clientset, enabled bool) {
	t.Helper()

	t.Logf("Changing API to 0 replicas")
	deploymentScaleAPI, err := clientset.AppsV1().Deployments("kubearchive").GetScale(
		context.Background(), "kubearchive-api-server", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	scaleAPI := *deploymentScaleAPI
	scaleAPI.Spec.Replicas = 0
	_, err = clientset.AppsV1().Deployments("kubearchive").UpdateScale(
		context.Background(), "kubearchive-api-server", &scaleAPI, metav1.UpdateOptions{})
	if err != nil {
		t.Fatal(err)
	}

	// Wait API to have to pods, so port-forward doesn't pick the previous pod
	t.Logf("Waiting for API to have no pods")
	retryErr := retry.Do(func() error {
		pods, listErr := clientset.CoreV1().Pods("kubearchive").List(
			context.Background(),
			metav1.ListOptions{
				LabelSelector: "app=kubearchive-api-server",
			})
		if listErr != nil {
			return listErr
		}
		if len(pods.Items) == 0 {
			return nil
		}
		return fmt.Errorf("API still has pods")
	}, retry.Attempts(30), retry.MaxDelay(2*time.Second))

	if retryErr != nil {
		t.Fatal(retryErr)
	}

	deployment, err := clientset.AppsV1().Deployments("kubearchive").Get(
		context.Background(), "kubearchive-api-server", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Failed to get kubearchive-api-server: %v", err)
	}
	for idx := range deployment.Spec.Template.Spec.Containers {
		container := &deployment.Spec.Template.Spec.Containers[idx]
		if container.Name == "kubearchive-api-server" {
			var exists bool
			for envIdx := range container.Env {
				env := &container.Env[envIdx]
				if env.Name == "AUTH_IMPERSONATE" {
					t.Log("Updating AUTH_IMPERSONATE")
					env.Value = strconv.FormatBool(enabled)
					exists = true
					break
				}
			}

			if !exists {
				t.Fatalf("AUTH_IMPERSONATE should exists on the kubearchive-api-server Deployment")
			}
		}
	}

	t.Log("Updating kubearchive-api-server")
	_, err = clientset.AppsV1().Deployments("kubearchive").Update(
		context.Background(), deployment, metav1.UpdateOptions{})
	if err != nil {
		t.Fatalf("Failed to update kubearchive-api-server: %v", err)
	}

	t.Logf("Changing API to 1 replicas")
	deploymentScaleAPI, err = clientset.AppsV1().Deployments("kubearchive").GetScale(
		context.Background(), "kubearchive-api-server", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	scaleAPI = *deploymentScaleAPI
	scaleAPI.Spec.Replicas = 1
	_, err = clientset.AppsV1().Deployments("kubearchive").UpdateScale(
		context.Background(), "kubearchive-api-server", &scaleAPI, metav1.UpdateOptions{})
	if err != nil {
		t.Fatal(err)
	}

	// Wait API to be up
	t.Logf("Waiting for API to be up")
	retryErr = retry.Do(func() error {
		logs, getErr := test.GetPodLogs(t, "kubearchive", "kubearchive-api-server")
		if getErr != nil {
			return getErr
		}
		if strings.Contains(logs, "Successfully connected to the database") {
			return nil
		}
		t.Log("logs:", logs)
		return fmt.Errorf("API has not started yet")
	}, retry.Attempts(30), retry.MaxDelay(2*time.Second))

	if retryErr != nil {
		t.Fatal(retryErr)
	}
}

func createImpersonatorSA(t *testing.T, clientset *kubernetes.Clientset, name, namespace, impersonatedUser string) *authenticationv1.TokenRequest {

	t.Helper()
	t.Logf("Creating service account '%s' in namespace '%s'", name, namespace)
	_, err := clientset.CoreV1().ServiceAccounts(namespace).Create(
		context.Background(),
		&corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: name}},
		metav1.CreateOptions{},
	)
	if err != nil {
		t.Logf("Could not create service account '%s' in namespace '%s'", name, namespace)
		t.Fatal(err)
	}

	t.Logf("Creating token for service account '%s' in namespace '%s'", name, namespace)
	token, err := clientset.CoreV1().ServiceAccounts(namespace).CreateToken(
		context.Background(), name, &authenticationv1.TokenRequest{}, metav1.CreateOptions{})
	if err != nil {
		t.Logf("Could not create token for servica account '%s' in namespace '%s'", name, namespace)
		t.Fatal(err)
	}

	_, err = clientset.RbacV1().Roles(namespace).Create(context.Background(), &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name: "impersonate",
		},
		Rules: []rbacv1.PolicyRule{
			{
				Verbs:         []string{"impersonate"},
				Resources:     []string{"serviceaccounts"},
				APIGroups:     []string{""},
				ResourceNames: []string{impersonatedUser},
			},
		},
	},
		metav1.CreateOptions{},
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = clientset.RbacV1().RoleBindings(namespace).Create(context.Background(), &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: "edit-test",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      name,
				Namespace: namespace,
			},
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     "impersonate",
		},
	},
		metav1.CreateOptions{},
	)
	if err != nil {
		t.Fatal(err)
	}
	return token
}
