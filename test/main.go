// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0
package test

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/avast/retry-go/v4"

	kubearchivev1alpha1 "github.com/kubearchive/kubearchive/cmd/operator/api/v1alpha1"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
	"k8s.io/client-go/util/homedir"

	authenticationv1 "k8s.io/api/authentication/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	errs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	sourcesv1 "knative.dev/eventing/pkg/apis/sources/v1"
)

const (
	letterBytes   = "abcdefghijklmnopqrstuvwxyz"
	randSuffixLen = 8
	K9eNamespace  = "kubearchive"
	KACName       = "kubearchive"
)

func RandomString() string {
	suffix := make([]byte, randSuffixLen)
	for i := range suffix {
		suffix[i] = letterBytes[rand.Intn(len(letterBytes))] // #nosec G404
	}
	return string(suffix)
}

func GetKubernetesConfig() (*rest.Config, error) {
	var kubeconfig string
	if home := homedir.HomeDir(); home != "" {
		kubeconfig = filepath.Join(home, ".kube", "config")
	}

	return clientcmd.BuildConfigFromFlags("", kubeconfig)
}

// portForward forwards the given ports until the retrieved channel is closed
func portForward(t testing.TB, ports []string, pod, ns string) (chan struct{}, error) {
	t.Helper()
	config, err := GetKubernetesConfig()
	if err != nil {
		return nil, err
	}
	roundTripper, upgrader, err := spdy.RoundTripperFor(config)
	if err != nil {
		return nil, err
	}

	path := fmt.Sprintf("/api/v1/namespaces/%s/pods/%s/portforward", ns, pod)
	hostIP := strings.TrimLeft(config.Host, "htps:/")
	serverURL := url.URL{Scheme: "https", Path: path, Host: hostIP}

	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: roundTripper}, http.MethodPost, &serverURL)

	stopChan, readyChan := make(chan struct{}, 1), make(chan struct{}, 1)
	out, errOut := new(bytes.Buffer), new(bytes.Buffer)
	forwarder, err := portforward.New(dialer, ports, stopChan, readyChan, out, errOut)
	if err != nil {
		return nil, err
	}
	go func() {
		if errForward := forwarder.ForwardPorts(); err != nil {
			panic(errForward)
		}
	}()
	var errReady error
	func() {
		for range readyChan { // Kubernetes will close this channel when it has something to tell us.
		}
		if len(errOut.String()) != 0 {
			errReady = errors.New(errOut.String())
		} else if len(out.String()) != 0 {
			OutputLines(t, out.String())
		}
	}()
	return stopChan, errReady
}

// state variables for PortForwardApiServer
var forwardRequestsMutex sync.Mutex
var forwardRequests int = 0
var forwardChan chan struct{}

const apiServerPort = "8081"

// Helper function to forward a port in a thread safe way. Returns the port used to access the Api Server.
func PortForwardApiServer(t testing.TB, clientset kubernetes.Interface) string {
	t.Helper()
	forwardRequestsMutex.Lock()
	defer forwardRequestsMutex.Unlock()

	// forward the port if not already forwarded
	if forwardRequests == 0 {
		pods, err := clientset.CoreV1().Pods("kubearchive").List(context.Background(), metav1.ListOptions{
			LabelSelector: "app=kubearchive-api-server",
			FieldSelector: "status.phase=Running",
		})
		t.Logf("Pod to forward: %s", pods.Items[0].Name)
		if err != nil {
			t.Fatal(err)
		}
		var errPortForward error
		retryErr := retry.Do(func() error {
			forwardChan, errPortForward = portForward(t, []string{fmt.Sprintf("%s:%s", apiServerPort, apiServerPort)}, pods.Items[0].Name, "kubearchive")
			if errPortForward != nil {
				return errPortForward
			}
			return nil
		}, retry.Attempts(3))

		if retryErr != nil {
			t.Fatal(retryErr)
		}
	}

	forwardRequests++

	t.Cleanup(func() {
		forwardRequestsMutex.Lock()
		defer forwardRequestsMutex.Unlock()
		forwardRequests--

		// close the port if no longer needed
		if forwardRequests == 0 {
			close(forwardChan)
		}
	})

	return apiServerPort
}

func GetKubernetesClient(t testing.TB) (*kubernetes.Clientset, *dynamic.DynamicClient) {
	t.Helper()

	config, err := GetKubernetesConfig()
	if err != nil {
		t.Fatal(err)
	}

	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		t.Fatal(err)
	}

	dynamicClient, err := dynamic.NewForConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	return client, dynamicClient
}

func GetPodLogs(t testing.TB, namespace, podPrefix string) (logs string, err error) {
	t.Helper()

	clientset, _ := GetKubernetesClient(t)

	podName := GetPodName(t, clientset, namespace, podPrefix)
	if podName == "" {
		return "", fmt.Errorf("unable to find pod with prefix '%s'", podPrefix)
	}

	req := clientset.CoreV1().Pods("kubearchive").GetLogs(podName, &corev1.PodLogOptions{})
	logStream, err := req.Stream(context.TODO())
	if err != nil {
		return "", fmt.Errorf("could not get logs for pod '%s' in the '%s' namespace: %w", podName, namespace, err)
	}

	buf := new(bytes.Buffer)
	_, err = buf.ReadFrom(logStream)
	if err != nil {
		return "", fmt.Errorf("could not process ReadFrom the stream: %w", err)
	}
	logBytes := buf.Bytes()
	logs = string(logBytes)

	return logs, nil
}

// Returns the first pod in the namespace that starts with the given pod prefix.
func GetPodName(t testing.TB, clientset *kubernetes.Clientset, namespace, prefix string) string {
	pods, err := clientset.CoreV1().Pods(namespace).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		t.Fatal(errors.New("Unable to get pods in namespace '" + namespace + "'"))
	}

	var podName = ""
	for _, pod := range pods.Items {
		if strings.HasPrefix(pod.Name, prefix) {
			podName = pod.Name
		}
	}
	return podName
}

func GetUrl(t testing.TB, token string, url string) (*unstructured.UnstructuredList, error) {
	client := getHTTPClient(t)
	body, err := getUrl(t, &client, token, url)
	if err != nil {
		return nil, err
	}

	var data unstructured.UnstructuredList
	err = json.Unmarshal(body, &data)
	if err != nil {
		t.Logf("Could not unmarshal JSON, %s", err)
		return nil, err
	}
	t.Logf("HTTP status: 200 OK, returned %d items", len(data.Items))
	return &data, nil
}

func GetLogs(t testing.TB, token string, url string) ([]byte, error) {
	client := getHTTPClient(t)
	return getUrl(t, &client, token, url)
}

func getHTTPClient(t testing.TB) http.Client {
	t.Helper()

	clientset, _ := GetKubernetesClient(t)
	secret, errSecret := clientset.CoreV1().Secrets("kubearchive").Get(context.Background(),
		"kubearchive-api-server-tls", metav1.GetOptions{})
	if errSecret != nil {
		t.Fatal(errSecret)
	}

	caCertPool := x509.NewCertPool()
	appendCert := caCertPool.AppendCertsFromPEM(secret.Data["ca.crt"])
	if !appendCert {
		t.Log("could not append the CA cert")
		t.Fatal(errors.New("could not append the CA cert"))
	}

	return http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs:    caCertPool,
				MinVersion: tls.VersionTLS13,
			},
		},
	}
}

func getUrl(t testing.TB, client *http.Client, token string, url string) ([]byte, error) {
	t.Helper()
	request, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Logf("Could not create a request, %s", err)
		return nil, err
	}
	request.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	response, err := client.Do(request)
	if err != nil {
		t.Logf("Could not get an HTTP response, %s", err)
		return nil, err
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Logf("Could not read body, %s", err)
		return nil, err
	}

	if response.StatusCode != http.StatusOK {
		t.Logf("HTTP status: %s, response: %s", response.Status, string(body))
		return nil, fmt.Errorf("%d", response.StatusCode)
	}

	return body, nil
}

// Run a job to generate a log. Returns the job name.
func RunLogGenerator(t testing.TB, namespace string) string {
	clientset, _ := GetKubernetesClient(t)
	name := fmt.Sprintf("generate-log-%s", RandomString())
	t.Logf("Running job '%s/%s'", namespace, name)
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					Containers: []corev1.Container{
						{
							Name:    "flog",
							Command: []string{"flog", "-n", "10", "-d", "1ms"},
							Image:   "quay.io/kubearchive/mingrammer/flog",
						},
					},
				},
			},
		},
	}

	_, err := clientset.BatchV1().Jobs(namespace).Create(context.Background(), job, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	return name
}

// Create a test namespace, returning the namespace name and the SA token.
func CreateTestNamespace(t testing.TB, customCleanup bool) (string, *authenticationv1.TokenRequest) {
	// Create a randomly name testing namespace and return the name.
	t.Helper()

	clientset, _ := GetKubernetesClient(t)

	namespace := fmt.Sprintf("test-%s", RandomString())
	t.Log("Creating test namespace '" + namespace + "'")

	_, err := clientset.CoreV1().Namespaces().Create(context.Background(), &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
		},
	}, metav1.CreateOptions{})

	if err != nil {
		t.Fatal(err)
	}

	if !customCleanup {
		t.Cleanup(func() {
			DeleteTestNamespace(t, namespace)
		})
	}

	err = retry.Do(func() error {
		_, e := clientset.CoreV1().ServiceAccounts(namespace).Get(context.Background(), "default", metav1.GetOptions{})
		return e
	}, retry.Attempts(10), retry.MaxDelay(2*time.Second))
	if err != nil {
		t.Logf("Could not find Service Account 'default' in namespace '%s'", namespace)
		t.Fatal(err)
	}

	token, err := clientset.CoreV1().ServiceAccounts(namespace).CreateToken(context.Background(), "default", &authenticationv1.TokenRequest{}, metav1.CreateOptions{})
	if err != nil {
		t.Logf("Could not create a token for service account 'default' in namespace '%s'", namespace)
		t.Fatal(err)
	}

	_, err = clientset.RbacV1().RoleBindings(namespace).Create(context.Background(), &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: "view-default-test",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      "default",
				Namespace: namespace,
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

	return namespace, token
}

func CreateKAC(t testing.TB, namespace string, resources map[string]any) {
	clientset, dynamicClient := GetKubernetesClient(t)
	kac := newKAC(namespace, resources)

	gvr := kubearchivev1alpha1.GroupVersion.WithResource("kubearchiveconfigs")
	_, err := dynamicClient.Resource(gvr).Namespace(namespace).Create(context.Background(), kac, metav1.CreateOptions{})
	if err != nil {
		t.Logf("Could not create KubeArchiveConfig in namespace '%s'", namespace)
		t.Fatal(err)
	}

	if len(resources) > 0 {
		// If we have resources, make sure ApiServerSource is created and there are sink filters before returning.
		a13eGvr := sourcesv1.SchemeGroupVersion.WithResource("apiserversources")
		err = retry.Do(func() error {
			_, retryErr := dynamicClient.Resource(a13eGvr).Namespace("kubearchive").Get(context.Background(), "kubearchive-a13e", metav1.GetOptions{})
			return retryErr
		}, retry.Attempts(10), retry.MaxDelay(2*time.Second))
		if err != nil {
			t.Fatal(err)
		}

		err = retry.Do(func() error {
			sinkFilters, retryErr := clientset.CoreV1().ConfigMaps("kubearchive").Get(context.Background(), "sink-filters", metav1.GetOptions{})
			if retryErr != nil {
				return retryErr
			}
			_, exists := sinkFilters.Data[namespace]
			if !exists {
				return fmt.Errorf("sink-filters ConfigMap does not yet have filters for the namespace %s", namespace)
			}
			return nil
		}, retry.Attempts(10), retry.MaxDelay(2*time.Second))
		if err != nil {
			t.Fatal(err)
		}
	}
}

func DeleteKAC(t testing.TB, namespace string) {
	clientset, dynamicClient := GetKubernetesClient(t)

	gvr := kubearchivev1alpha1.GroupVersion.WithResource("kubearchiveconfigs")
	err := dynamicClient.Resource(gvr).Namespace(namespace).Delete(context.Background(), KACName, metav1.DeleteOptions{})
	if err != nil {
		t.Logf("Could not delete KubeArchiveConfig in namespace '%s'", namespace)
		t.Fatal(err)
	}

	// Make sure KubeArchiveConfig is deleted.
	err = retry.Do(func() error {
		_, retryErr := dynamicClient.Resource(gvr).Namespace(namespace).Get(context.Background(), KACName, metav1.GetOptions{})
		return retryErr
	}, retry.Attempts(10), retry.MaxDelay(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}

	// Make sure the sink filters have been updated.
	err = retry.Do(func() error {
		sinkFilters, retryErr := clientset.CoreV1().ConfigMaps("kubearchive").Get(context.Background(), "sink-filters", metav1.GetOptions{})
		if retryErr != nil {
			return retryErr
		}
		_, exists := sinkFilters.Data[namespace]
		if exists {
			return fmt.Errorf("sink filters ConfigMap still has filters for namespace '%s'", namespace)
		}
		return nil
	}, retry.Attempts(10), retry.MaxDelay(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
}

func DeleteTestNamespace(t testing.TB, namespace string) {
	// Delete the given namespace and wait until it is removed from the cluster.
	t.Helper()

	t.Log("Deleting test namespace '" + namespace + "'")

	clientset, _ := GetKubernetesClient(t)

	errNamespace := clientset.CoreV1().Namespaces().Delete(context.Background(), namespace, metav1.DeleteOptions{})
	if errNamespace != nil {
		t.Fatal(errNamespace)
	}

	retryErr := retry.Do(func() error {
		_, getErr := clientset.CoreV1().Namespaces().Get(context.Background(), namespace, metav1.GetOptions{})
		if !errs.IsNotFound(getErr) {
			return errors.New("Waiting for namespace " + namespace + " to be deleted")
		}
		return nil
	}, retry.Attempts(10), retry.MaxDelay(3*time.Second))

	if retryErr != nil {
		t.Log(retryErr)
	}
}

func newKAC(namespace string, resources map[string]any) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"kind": "KubeArchiveConfig",
			"apiVersion": fmt.Sprintf("%s/%s",
				kubearchivev1alpha1.SchemeBuilder.GroupVersion.Group,
				kubearchivev1alpha1.SchemeBuilder.GroupVersion.Version),
			"metadata": map[string]string{
				"name":      KACName,
				"namespace": namespace,
			},
			"spec": resources,
		},
	}
}

func OutputLines(t testing.TB, output string) {
	for _, line := range strings.Split(output, "\n") {
		if len(line) > 0 {
			t.Log(line)
		}
	}
}
