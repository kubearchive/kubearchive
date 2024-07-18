//go:build integration

package main

import (
	"context"
	"fmt"
	"log"
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

// This test is redundant with the kubectl rollout status from the hack/quick-install.sh
// but it serves as a valid integration test, not a dummy that is not testing anything real.
func TestKubeArchiveDeployments(t *testing.T) {
	client, err := test.GetKubernetesClient()
	if err != nil {
		t.Fatal(err)
	}

	acc := 0.0
	for {
		if acc >= 30 {
			t.Fatal(fmt.Sprintf("Timed out waiting for deployment to be ready %f.", acc))
		}

		deployments, err := client.AppsV1().Deployments("kubearchive").List(context.Background(), metav1.ListOptions{})
		if err != nil {
			t.Fatal(err)
		}

		if len(deployments.Items) == 0 {
			t.Fatal("No deployments found in the 'kubearchive' namespace, something went wrong.")
		}

		areAllReady := true
		for _, deployment := range deployments.Items {
			log.Printf("Deployment '%s' has '%d' ready replicas", deployment.Name, deployment.Status.ReadyReplicas)
			areAllReady = areAllReady && deployment.Status.ReadyReplicas == 1
		}

		if areAllReady {
			log.Printf("All deployments ready.")
			break
		}

		log.Printf("Not all deployments are ready, waiting 5 seconds...")
		time.Sleep(5 * time.Second)
		acc += 5
	}
}

func TestKubeArchiveOperator(t *testing.T) {
	client, err := test.GetKubernetesClient()
	if err != nil {
		t.Fatal(err)
	}

	// clientset, err := kubernetes.NewForConfig(config)
	// 	if err != nil {
    // 		panic(err)
	// 	}

	// create a namespace
	nsName := &corev1.Namespace{
    	ObjectMeta: metav1.ObjectMeta{
        	Name: "test-1234",
    	},
	}
	client.CoreV1().Namespaces().Create(context.Background(), nsName, metav1.CreateOptions{})
	//client.CoreV1().Namespaces().Create(context.Background(), nsName, metav1.CreateOptions{})

	
	// deploy the operator 
	// check that in the test namespace the KA config was created.
	

	log.Printf("fake test")
}


