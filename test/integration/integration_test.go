// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0
//go:build integration

package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/avast/retry-go/v4"
	kubearchivev1alpha1 "github.com/kubearchive/kubearchive/cmd/operator/api/v1alpha1"
	"github.com/kubearchive/kubearchive/test"
	"github.com/stretchr/testify/assert"
	authenticationv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestMain(m *testing.M) {
	if os.Getenv("KO_DOCKER_REPO") == "" {
		os.Setenv("KO_DOCKER_REPO", "kind.local")
	}
	os.Exit(m.Run())
}

// TestKubeArchiveDeployments is redundant with the kubectl rollout status from the hack/quick-install.sh
// ,but it serves as a valid integration test, not a dummy that is not testing anything real.
func TestKubeArchiveDeployments(t *testing.T) {
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

// TestDatabaseConnection verifies the database connection retries using the Sink component.
func TestDatabaseConnection(t *testing.T) {
	clientset, _, err := test.GetKubernetesClient()
	if err != nil {
		t.Fatal(err)
	}

	dynclient, err := test.GetDynamicKubernetesClient()
	if err != nil {
		t.Fatal(err)
	}

	// Fence database to make it unavailable - https://cloudnative-pg.io/documentation/1.24/fencing/
	clusterResource := schema.GroupVersionResource{Group: "postgresql.cnpg.io", Version: "v1", Resource: "clusters"}
	resource, err := dynclient.Resource(clusterResource).Namespace("postgresql").Get(context.Background(), "kubearchive", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	annotations := resource.GetAnnotations()
	annotations["cnpg.io/fencedInstances"] = "[\"*\"]"
	resource.SetAnnotations(annotations)

	resource, err = dynclient.Resource(clusterResource).Namespace("postgresql").Update(context.Background(), resource, metav1.UpdateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Fenced database")

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
	t.Logf("Changing sink to %d replicas", scaleSink.Spec.Replicas)
	t.Log(*usSink)

	t.Logf("Waiting 5 seconds for kubearchive-sink to scale down...")
	time.Sleep(5 * time.Second)

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
	t.Logf("Changing sink to %d replicas", scaleSink.Spec.Replicas)
	t.Log(*usSink)

	// wait to sink pod ready and generate connection retries with the database
	t.Logf("Waiting for sink to be up, and to generate retries with the database")
	retryErr := retry.Do(func() error {
		logs, err := test.GetPodLogs(clientset, "kubearchive", "kubearchive-sink")
		if err != nil {
			return err
		}

		t.Logf("Logs\n%s", logs)
		if strings.Contains(logs, "connection refused") {
			return nil
		}

		return fmt.Errorf("Pod didn't try to connect to the database yet")
	})

	if retryErr != nil {
		t.Fatal(retryErr)
	}

	// Unfence database
	resource, err = dynclient.Resource(clusterResource).Namespace("postgresql").Get(context.Background(), "kubearchive", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	annotations = resource.GetAnnotations()
	delete(annotations, "cnpg.io/fencedInstances")
	resource.SetAnnotations(annotations)

	resource, err = dynclient.Resource(clusterResource).Namespace("postgresql").Update(context.Background(), resource, metav1.UpdateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Unfenced database")

	retryErr = retry.Do(func() error {
		logs, err := test.GetPodLogs(clientset, "kubearchive", "kubearchive-sink")
		if err != nil {
			return nil
		}

		t.Logf("Logs:\n%s", logs)
		if strings.Contains(logs, "Successfully connected to the database") {
			return nil
		}

		return errors.New("Pod didn't connect successfully to the database yet")
	})

	if retryErr != nil {
		t.Fatal(retryErr)
	}
}

func TestArchiveAndRead(t *testing.T) {
	clientset, _, errClient := test.GetKubernetesClient()
	if errClient != nil {
		t.Fatal(errClient)
	}

	// Install splunk
	cmdInstallSplunk := exec.Command("bash", "../../integrations/logging/splunk/install.sh")
	outputInstallSplunk, errScriptSplunk := cmdInstallSplunk.CombinedOutput()
	if errScriptSplunk != nil {
		fmt.Println("Could not run the splunk installer: ", errScriptSplunk)
		t.Fatal(errScriptSplunk)
	}
	fmt.Println("Output: ", string(outputInstallSplunk))

	// Forward api service port
	pods, err := clientset.CoreV1().Pods("kubearchive").List(context.Background(), metav1.ListOptions{
		LabelSelector: "app=kubearchive-api-server",
		FieldSelector: "status.phase=Running",
	})
	fmt.Println(fmt.Sprintf("Pod to forward: %s", pods.Items[0].Name))
	if err != nil {
		t.Fatal(err)
	}
	portForward, errPortForward := test.PortForward([]string{"8081:8081"}, pods.Items[0].Name, "kubearchive")
	if errPortForward != nil {
		t.Fatal(errPortForward)
	}

	defer close(portForward)

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
	})

	// Call the log-generator.
	namespaceCmd := fmt.Sprintf("--namespace=%s", namespaceName)
	cmd := exec.Command("bash", "../log-generators/cronjobs/install.sh", namespaceCmd, "--num-jobs=1")
	output, errScript := cmd.CombinedOutput()
	if errScript != nil {
		fmt.Println("Could not run the log-generator: ", errScript)
		t.Fatal(errScript)
	}
	fmt.Println("Output: ", string(output))

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

	token, errToken := clientset.CoreV1().ServiceAccounts(namespaceName).CreateToken(context.Background(), "default", &authenticationv1.TokenRequest{}, metav1.CreateOptions{})
	if errToken != nil {
		fmt.Printf("could not create a token, %s", errToken)
		t.Fatal(errToken)
	}

	caCertPool := x509.NewCertPool()
	appendCert := caCertPool.AppendCertsFromPEM(secret.Data["ca.crt"])
	if !appendCert {
		fmt.Printf("could not append the CA cert")
		t.Fatal(errors.New("could not append the CA cert"))
	}
	clientHTTP := http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs: caCertPool,
			},
		},
	}

	url := fmt.Sprintf("https://localhost:8081/apis/batch/v1/namespaces/%s/cronjobs", namespaceName)
	request, errRequest := http.NewRequest("GET", url, nil)
	if errRequest != nil {
		fmt.Printf("could not create a request, %s", errRequest)
		t.Fatal(errRequest)
	}
	request.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token.Status.Token))

	retryErr := retry.Do(func() error {
		response, errResp := clientHTTP.Do(request)
		if errResp != nil {
			fmt.Printf("Couldn't get a response HTTP, %s\n", errResp)
			t.Fatal(errResp)
			return errResp
		}
		defer response.Body.Close()

		body, errReadAll := io.ReadAll(response.Body)
		if errReadAll != nil {
			fmt.Printf("Couldn't read Body, %s\n", errReadAll)
			return errReadAll
		}
		fmt.Printf("HTTP body: %s\n", body)

		if response.StatusCode != http.StatusOK {
			fmt.Printf("The HTTP status returned is not OK, %s\n", response.Status)
			return errors.New(response.Status)
		}
		type Response struct {
			Items []*unstructured.Unstructured `json:"items"`
		}
		var data Response
		errJson := json.Unmarshal(body, &data)
		if errJson != nil {
			fmt.Printf("Couldn't unmarshal JSON, %s\n", errJson)
			return errJson
		}
		if len(data.Items) == 1 {
			return nil
		}
		return errors.New("could not retrieved a CronJob from the API")
	}, retry.Attempts(20))

	if retryErr != nil {
		t.Fatal(retryErr)
	}

	// Remove cronjob to stop generating more pods
	errDel := clientset.BatchV1().CronJobs(namespaceName).Delete(context.Background(), "generate-log-1", metav1.DeleteOptions{})
	if errDel != nil {
		t.Fatal(errDel)
	}

	// Retrieve cronjob logs

	url = fmt.Sprintf("https://localhost:8081/apis/batch/v1/namespaces/%s/cronjobs/generate-log-1/log", namespaceName)
	request, errRequest = http.NewRequest("GET", url, nil)
	if errRequest != nil {
		fmt.Printf("could not create a request, %s", errRequest)
		t.Fatal(errRequest)
	}
	request.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token.Status.Token))
	fmt.Printf("HTTP request: %s\n", request.URL.RequestURI())

	retryErr = retry.Do(func() error {
		response, errResp := clientHTTP.Do(request)
		if errResp != nil {
			fmt.Printf("Couldn't get a response HTTP, %s\n", errResp)
			return errResp
		}
		defer response.Body.Close()

		body, errReadAll := io.ReadAll(response.Body)
		if errReadAll != nil {
			fmt.Printf("Couldn't read Body, %s\n", errReadAll)
			return errReadAll
		}

		if response.StatusCode != http.StatusOK {
			fmt.Printf("HTTP body: %s\n", body)
			fmt.Printf("The HTTP status returned is not OK, %s\n", response.Status)
			return errors.New(response.Status)
		}

		if len(body) == 0 {
			return errors.New("could not retrieve the cronjob pod log")
		}
		return nil
	}, retry.Attempts(20))

	if retryErr != nil {
		t.Fatal(retryErr)
	}

}

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
					Name:    "busybox",
					Command: []string{"echo", "hello"},
					Image:   "quay.io/fedora/fedora:latest",
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
	portForward, errPortForward := test.PortForward([]string{"8081"}, pods.Items[0].Name, "kubearchive")
	if errPortForward != nil {
		t.Fatal(errPortForward)
	}
	defer close(portForward)

	url := fmt.Sprintf("https://localhost:8081/api/v1/namespaces/%s/pods", namespaceName)
	err = retry.Do(func() error {
		list, err := getUrl(&clientHTTP, token.Status.Token, url)
		if err != nil {
			t.Fatal(err)
		}
		// We want to wait until everything is stored in the DB to avoid out of order inserts
		if len(list.Items) >= 30 {
			return nil
		}
		return errors.New("could not retrieve Pods from the API")
	}, retry.Attempts(30), retry.MaxDelay(4*time.Second))

	if err != nil {
		t.Fatal(err)
	}

	url = fmt.Sprintf("https://localhost:8081/api/v1/namespaces/%s/pods?limit=10", namespaceName)
	list, err := getUrl(&clientHTTP, token.Status.Token, url)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, 10, len(list.Items))

	url = fmt.Sprintf("https://localhost:8081/api/v1/namespaces/%s/pods?limit=10&continue=%s", namespaceName, list.GetContinue())
	continueList, err := getUrl(&clientHTTP, token.Status.Token, url)
	if err != nil {
		t.Fatal(err)
	}

	url = fmt.Sprintf("https://localhost:8081/api/v1/namespaces/%s/pods?limit=20", namespaceName)
	allList, err := getUrl(&clientHTTP, token.Status.Token, url)
	if err != nil {
		t.Fatal(err)
	}

	var listNames []string
	for _, item := range list.Items {
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

func getUrl(client *http.Client, token string, url string) (*unstructured.UnstructuredList, error) {
	request, err := http.NewRequest("GET", url, nil)
	if err != nil {
		fmt.Printf("could not create a request, %s", err)
		return nil, err
	}
	request.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	response, err := client.Do(request)
	if err != nil {
		fmt.Printf("Couldn't get a response HTTP, %s\n", err)
		return nil, err
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		fmt.Printf("Couldn't read Body, %s\n", err)
		return nil, err
	}

	if response.StatusCode != http.StatusOK {
		fmt.Printf("The HTTP status returned is not OK, %s - %s \n", response.Status, string(body))
		return nil, fmt.Errorf("%d", response.StatusCode)
	}

	var data unstructured.UnstructuredList
	err = json.Unmarshal(body, &data)
	if err != nil {
		fmt.Printf("Couldn't unmarshal JSON, %s\n", err)
		return nil, err
	}
	fmt.Printf("The HTTP status returned is OK, %s - %s \n", response.Status, string(body))
	return &data, nil
}
