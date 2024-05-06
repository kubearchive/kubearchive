//go:build integration

package main

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/kubearchive/kubearchive/test"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestMain(m *testing.M) {
	if os.Getenv("KO_DOCKER_REPO") == "" {
		os.Setenv("KO_DOCKER_REPO", "kind.local")
	}
	os.Exit(m.Run())
}

func TestSomething(t *testing.T) {
	namespaceName := fmt.Sprintf("test-%s", test.RandomString())
	namespace := fmt.Sprintf(`
---
apiVersion: v1
kind: Namespace
metadata:
  name: %s
`, namespaceName)

	deployment := fmt.Sprintf(`
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: kubearchive
  namespace: %s
  labels:
    app.kubernetes.io/name: kubearchive
spec:
  selector:
    matchLabels:
      app.kubernetes.io/name: kubearchive
  template:
    metadata:
      labels:
        app.kubernetes.io/name: kubearchive
    spec:
      containers:
      - name: kubearchive
        image: ko://github.com/kubearchive/kubearchive/cmd/api/
        ports:
        - containerPort: 8081
`, namespaceName)

	err := test.CreateResources(namespace, deployment)
	if err != nil {
		t.Fatal(err)
	}

	client, err := test.GetKubernetesClient()

	acc := 0.0
	for {
		if acc >= 30 {
			t.Fatal("Timed out waiting for deployment to be ready.")
		}

		deploymentResource, err := client.AppsV1().Deployments(namespaceName).Get(context.Background(), "kubearchive", metav1.GetOptions{})
		if err != nil {
			t.Fatal(err)
		}

		if deploymentResource.Status.AvailableReplicas == 1 {
			break
		} else {
			time.Sleep(3 * time.Second)
			acc += 3
		}
	}

	t.Cleanup(func() {
		err := test.DeleteResources(namespace)
		if err != nil {
			panic(err)
		}
	})
}
