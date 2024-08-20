//go:build integration

package main

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/kubearchive/kubearchive/test"
	corev1 "k8s.io/api/core/v1"
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

// This test checks if the Kubernetes Objects created by the KubeArchive Operator
// are deployed correctly.
func TestKubeArchiveOperator(t *testing.T) {
	namespaceName := fmt.Sprintf("test-%s", test.RandomString())
	namespace := fmt.Sprintf(`
---
apiVersion: v1
kind: Namespace
metadata:
  name: %s
`, namespaceName)

	kubearchiveconfig := fmt.Sprintf(`
---
apiVersion: kubearchive.kubearchive.org/v1alpha1
kind: KubeArchiveConfig
metadata:
  name: kubearchive
  namespace: %s
spec:
  resources:
    - selector:
        apiVersion: v1
        kind: Event
      archiveWhen: status.state != 'Completed'
      deleteWhen: status.state == 'Completed'
`, namespaceName)

	// create the test namespace and KubeArchiveConfig
	err := test.CreateResources(namespace, kubearchiveconfig)
	if err != nil {
		t.Fatal(err)
	}

	client, err := test.GetKubernetesClient()
	if err != nil {
		t.Fatal(err)
	}
	// wait for resources to be created
	time.Sleep(15 * time.Second)

	// check deployment
	deployments, err := client.AppsV1().Deployments(namespaceName).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		t.Fatal(err)
	}

	kaDeployment := false
	for _, deployment := range deployments.Items {
		log.Printf("Deployment '%s' has '%d' ready replicas", deployment.Name, deployment.Status.ReadyReplicas)
		if (strings.Contains(deployment.Name, "apiserversource-kubearchive")) && deployment.Status.ReadyReplicas == 1 {
			kaDeployment = true
		}
	}

	// check replica set
	replicasets, err := client.AppsV1().ReplicaSets(namespaceName).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		t.Fatal(err)
	}

	kaReplicaSet := false
	for _, replicaset := range replicasets.Items {
		log.Printf("Replicaset '%s' has '%d' ready replicas", replicaset.Name, replicaset.Status.ReadyReplicas)
		if strings.Contains(replicaset.Name, "apiserversource-kubearchive") && replicaset.Status.ReadyReplicas == 1 {
			kaReplicaSet = true
		}
	}

	// check pod
	pods, err := client.CoreV1().Pods(namespaceName).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	kaPod := false
	for _, pod := range pods.Items {
		if strings.Contains(pod.Name, "apiserversource-kubearchive") && (pod.Status.Phase == "Running") {
			log.Printf("The apiserversource-kubearchive Pod is Running")
			kaPod = true
		}
	}

	// check apiserversource
	// using kubectl and jsonpath
	// -o=jsonpath='{.items[?(@.metadata.name=="kubearchive")].status.conditions[?(@.type=="Ready")].status}'
	jsonPathApi := ".items[?(@.metadata.name==\"kubearchive\")]"
	kaApiServiceSource, _ := IsObjectReady("ApiServerSource", namespaceName, jsonPathApi)
	if kaApiServiceSource {
		log.Printf("The ApiServerSource is Ready")
	}
	// check eventtype
	// using kubectl and jsonpath
	//-o=jsonpath='{.items[?(@.spec.type==\"dev.knative.apiserver.resource.add\")].status.conditions[?(@.type==\"Ready\")].status}'"
	//-o=jsonpath='{.items[?(@.spec.type==\"dev.knative.apiserver.resource.delete\")].status.conditions[?(@.type==\"Ready\")].status}'"
	//-o=jsonpath='{.items[?(@.spec.type==\"dev.knative.apiserver.resource.update\")].status.conditions[?(@.type==\"Ready\")].status}'"

	pathSpecType := ".items[?(@.spec.type==\"dev.knative.apiserver.resource."
	jsonPathAdd := fmt.Sprintf("%sadd\")]", pathSpecType)
	jsonPathDelete := fmt.Sprintf("%sdelete\")]", pathSpecType)
	jsonPathUpdate := fmt.Sprintf("%supdate\")]", pathSpecType)

	kaEventAdd, _ := IsObjectReady("eventtype", namespaceName, jsonPathAdd)
	if kaEventAdd {
		log.Printf("The EventType add is Ready")
	}
	kaEventDelete, _ := IsObjectReady("eventtype", namespaceName, jsonPathDelete)
	if kaEventDelete {
		log.Printf("The EventType delete is Ready")
	}
	kaEventUpdate, _ := IsObjectReady("eventtype", namespaceName, jsonPathUpdate)
	if kaEventUpdate {
		log.Printf("The EventType update is Ready")
	}
	if (kaDeployment && kaReplicaSet && kaPod && kaApiServiceSource && kaEventAdd && kaEventUpdate && kaEventDelete) == true {
		log.Printf("All Kubernetes Objects created by the Operator are Ready.")

	} else {
		log.Printf("One or more Kubernetes Objects created by the Operator are Ready.")
	}

	// delete the test namespace
	errDelete := test.DeleteResources(namespace)
	if errDelete != nil {
		t.Fatal(errDelete)
	}
}

// This function is used to extract the status of a Kubernetes Objects.
func IsObjectReady(resource string, namespace string, jsonPath string) (bool, error) {
	kaStatus := false
	pathStatusConditions := ".status.conditions[?(@.type==\"Ready\")].status}"
	jsonPathStatus := fmt.Sprintf("-o=jsonpath='{%s%s'", jsonPath, pathStatusConditions)
	//log.Printf("jsonPathStatus %s", jsonPathStatus )
	cmdTest := exec.Command("kubectl", "get", resource, "-n", namespace, jsonPathStatus)
	outputTest, err := cmdTest.CombinedOutput()
	if err != nil {
		return kaStatus, err
	}

	if string(outputTest) == "'True'" {
		kaStatus = true
	}
	return kaStatus, err
}

// This test verifies the database connection retries using the Sink component.
func TestDatabaseConnection(t *testing.T) {
	clientset, err := test.GetKubernetesClient()
	if err != nil {
		t.Fatal(err)
	}

	// database deployment
	deploymentScaleDatabase, err := clientset.AppsV1().Deployments("kubearchive").GetScale(context.Background(), "kubearchive-database", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	// database replicas = 0
	scaleDatabase := *deploymentScaleDatabase
	scaleDatabase.Spec.Replicas = 0
	usDatabase, err := clientset.AppsV1().Deployments("kubearchive").UpdateScale(context.Background(), "kubearchive-database", &scaleDatabase, metav1.UpdateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	log.Printf("Changing database to %d replicas", scaleDatabase.Spec.Replicas)
	log.Println(*usDatabase)

	// restart sink pod - replicas = 0
	deploymentScaleSink, err := clientset.AppsV1().Deployments("kubearchive").GetScale(context.Background(), "kubearchive-sink", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}

	scaleSink := *deploymentScaleSink
	scaleSink.Spec.Replicas = 0
	usSink, err := clientset.AppsV1().Deployments("kubearchive").UpdateScale(context.Background(), "kubearchive-sink", &scaleSink, metav1.UpdateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	log.Printf("Changing sink to %d replicas", scaleSink.Spec.Replicas)
	log.Println(*usSink)

	// restart sink pod - replicas = 1
	deploymentScaleSink, err = clientset.AppsV1().Deployments("kubearchive").GetScale(context.Background(), "kubearchive-sink", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	scaleSink = *deploymentScaleSink
	scaleSink.Spec.Replicas = 1
	usSink, err = clientset.AppsV1().Deployments("kubearchive").UpdateScale(context.Background(), "kubearchive-sink", &scaleSink, metav1.UpdateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	log.Printf("Changing sink to %d replicas", scaleSink.Spec.Replicas)
	log.Println(*usSink)

	// wait to sink pod ready and generate connection retries with the database
	log.Printf("Waiting for sink to be up, and to generate retries with the database")
	time.Sleep(150 * time.Second)

	// wake-up database -> replicas = 1
	deploymentScaleDatabase, err = clientset.AppsV1().Deployments("kubearchive").GetScale(context.Background(), "kubearchive-database", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	scaleDatabase = *deploymentScaleDatabase
	scaleDatabase.Spec.Replicas = 1
	usDatabase, err = clientset.AppsV1().Deployments("kubearchive").UpdateScale(context.Background(), "kubearchive-database", &scaleDatabase, metav1.UpdateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	log.Printf("Changing database to %d replicas", scaleDatabase.Spec.Replicas)
	log.Println(*usDatabase)

	// wait to have the database up
	time.Sleep(30 * time.Second)

	// get sink logs
	pods, err := clientset.CoreV1().Pods("kubearchive").List(context.Background(), metav1.ListOptions{})
	if err != nil {
		t.Fatal(err)
	}

	var sinkPodName string
	for _, pod := range pods.Items {
		if strings.Contains(pod.Name, "kubearchive-sink") {
			sinkPodName = pod.Name
		}
	}
	podLogOpts := corev1.PodLogOptions{}
	req := clientset.CoreV1().Pods("kubearchive").GetLogs(sinkPodName, &podLogOpts)
	sinkLogs, err := req.Stream(context.TODO())
	if err != nil {
		t.Fatal(err)
	}
	buf := new(bytes.Buffer)
	buf.ReadFrom(sinkLogs)
	respBytes := buf.Bytes()
	respString := string(respBytes)
	if strings.Contains(respString, "Successfully connected to the database") {
		log.Printf(respString)
	} else {
		t.Fatal(fmt.Sprintf("Sink could not be connected to the database after the retries: %s", respString))
	}
}
