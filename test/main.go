// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0
package test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	sourcesv1 "knative.dev/eventing/pkg/apis/sources/v1"
)

const (
	letterBytes   = "abcdefghijklmnopqrstuvwxyz"
	randSuffixLen = 8
	K9eNamespace  = "kubearchive"
)

func RandomString() string {
	suffix := make([]byte, randSuffixLen)
	for i := range suffix {
		suffix[i] = letterBytes[rand.Intn(len(letterBytes))] // #nosec G404
	}
	return string(suffix)
}

func CreateResources(t testing.TB, resources ...string) error {
	t.Helper()
	for _, resource := range resources {
		f, err := os.CreateTemp("/tmp", "resource-*.yml")
		if err != nil {
			return err
		}

		_, err = f.Write([]byte(resource))
		if err != nil {
			return err
		}
		f.Close()

		t.Logf("running ko apply -f %s, file kept for inspection.\n", f.Name())
		cmd := exec.Command("ko", "apply", "-f", f.Name()) // #nosec G204
		output, err := cmd.CombinedOutput()
		if err != nil {
			return errors.New(string(output))
		}
	}

	return nil
}

func DeleteResources(t testing.TB, resources ...string) error {
	t.Helper()
	for _, resource := range resources {
		f, err := os.CreateTemp("/tmp", "resource-*.yml")
		if err != nil {
			return err
		}

		_, err = f.Write([]byte(resource))
		if err != nil {
			return err
		}
		f.Close()

		t.Logf("running kubectl delete -f %s, file kept for inspection.\n", f.Name())
		cmd := exec.Command("kubectl", "delete", "-f", f.Name()) // #nosec G204
		output, err := cmd.CombinedOutput()
		if err != nil {
			return errors.New(string(output))
		}
	}

	return nil
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
			t.Log(out.String())
		}
	}()
	return stopChan, errReady
}

// state variables for PortForwardApiServer
var forwardRequestsMutex sync.Mutex
var forwardRequests int = 0
var forwardChan chan struct{}

const apiServerPort = "8081"

// Helper function to forward a port in a thread safe way. Returns the port used to access the Api Server and function
// for cleaning up the forwarded port that should be called with defer
func PortForwardApiServer(t testing.TB, clientset kubernetes.Interface) (string, func()) {
	t.Helper()
	forwardRequestsMutex.Lock()
	defer forwardRequestsMutex.Unlock()

	// forward the port if not already forwarded
	if forwardRequests == 0 {
		pods, err := clientset.CoreV1().Pods("kubearchive").List(context.Background(), metav1.ListOptions{
			LabelSelector: "app=kubearchive-api-server",
			FieldSelector: "status.phase=Running",
		})
		t.Logf("Pod to forward: %s\n", pods.Items[0].Name)
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

	return apiServerPort, func() {
		forwardRequestsMutex.Lock()
		defer forwardRequestsMutex.Unlock()
		forwardRequests--

		// close the port if no longer needed
		if forwardRequests == 0 {
			close(forwardChan)
		}
	}
}

func GetKubernetesClient() (*kubernetes.Clientset, *dynamic.DynamicClient, error) {

	config, err := GetKubernetesConfig()
	if err != nil {
		return nil, nil, err
	}

	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, nil, err
	}

	dynamicClient, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, nil, err
	}
	return client, dynamicClient, nil
}

func GetPodLogs(clientset *kubernetes.Clientset, namespace, podName string) (logs string, err error) {
	pods, err := clientset.CoreV1().Pods(namespace).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return "", fmt.Errorf("Couldn't get pods for '%s' namespace: %w", namespace, err)
	}

	var realPodName string
	for _, pod := range pods.Items {
		if strings.Contains(pod.Name, podName) {
			realPodName = pod.Name
		}
	}

	req := clientset.CoreV1().Pods("kubearchive").GetLogs(realPodName, &corev1.PodLogOptions{})
	logStream, err := req.Stream(context.TODO())
	if err != nil {
		return "", fmt.Errorf("Couldn't get logs for pod '%s' in the '%s' namespace: %w", realPodName, namespace, err)
	}

	buf := new(bytes.Buffer)
	_, err = buf.ReadFrom(logStream)
	if err != nil {
		return "", fmt.Errorf("Couldn't process ReadFrom the stream: %w", err)
	}
	logBytes := buf.Bytes()
	logs = string(logBytes)

	return logs, nil
}

func GetDynamicKubernetesClient() (*dynamic.DynamicClient, error) {
	var kubeconfig string
	if home := homedir.HomeDir(); home != "" {
		kubeconfig = filepath.Join(home, ".kube", "config")
	}

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, err
	}

	client, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("Error instantiating k8s from host %s: %s", config.Host, err)
	}
	return client, nil
}

func GetUrl(t testing.TB, client *http.Client, token string, url string) (*unstructured.UnstructuredList, error) {
	t.Helper()
	body, err := getUrl(t, client, token, url)
	if err != nil {
		return nil, err
	}

	var data unstructured.UnstructuredList
	err = json.Unmarshal(body, &data)
	if err != nil {
		t.Logf("Couldn't unmarshal JSON, %s\n", err)
		return nil, err
	}
	t.Logf("The HTTP status returned is OK, returned %d items\n", len(data.Items))
	return &data, nil
}

func GetLogs(t testing.TB, client *http.Client, token string, url string) ([]byte, error) {
	t.Helper()
	return getUrl(t, client, token, url)
}

func getUrl(t testing.TB, client *http.Client, token string, url string) ([]byte, error) {
	t.Helper()
	request, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Logf("could not create a request, %s\n", err)
		return nil, err
	}
	request.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	response, err := client.Do(request)
	if err != nil {
		t.Logf("Couldn't get a response HTTP, %s\n", err)
		return nil, err
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Logf("Couldn't read Body, %s\n", err)
		return nil, err
	}

	if response.StatusCode != http.StatusOK {
		t.Logf("The HTTP status returned is not OK, %s - %s \n", response.Status, string(body))
		return nil, fmt.Errorf("%d", response.StatusCode)
	}

	return body, nil
}

var logGenMutex sync.Mutex

type logGenType int

const (
	CronJobGenerator logGenType = iota
	PipelineGenerator
)

func (l logGenType) installScriptPath(t testing.TB) string {
	t.Helper()
	switch l {
	case CronJobGenerator:
		return "../log-generators/cronjobs/install.sh"
	case PipelineGenerator:
		return "../log-generators/pipelines/install.sh"
	default:
		t.Fatal("Invalid log generator. Use CronJobs or Pipelines")
	}
	return ""
}

// RunLogGenerators runs the log generators in the thread safe way.
func RunLogGenerators(t testing.TB, logGen logGenType, namespace string, numRuns int) {
	t.Helper()
	logGenMutex.Lock()
	defer logGenMutex.Unlock()

	namespaceFlag := "--namespace=" + namespace
	numFlag := "--num-jobs=" + strconv.Itoa(numRuns)

	t.Log("Command:", "bash", logGen.installScriptPath(t), namespaceFlag, numFlag)

	cmd := exec.Command("bash", logGen.installScriptPath(t), namespaceFlag, numFlag) // #nosec G204
	output, errScript := cmd.CombinedOutput()
	if errScript != nil {
		t.Log("Could not run the log-generator: ", string(output))
		t.Fatal(errScript)
	}
	t.Log("Output: ", string(output))
}

func CreateKAC(t testing.TB, clientset kubernetes.Interface, dynamicClient dynamic.Interface, kac *unstructured.Unstructured, namespaceName string) {
	gvr := kubearchivev1alpha1.GroupVersion.WithResource("kubearchiveconfigs")
	_, err := dynamicClient.Resource(gvr).Namespace(namespaceName).Create(context.Background(), kac, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}

	a13eGvr := sourcesv1.SchemeGroupVersion.WithResource("apiserversources")
	err = retry.Do(func() error {
		_, retryErr := dynamicClient.Resource(a13eGvr).Namespace("kubearchive").Get(context.Background(), "kubearchive-a13e", metav1.GetOptions{})
		return retryErr
	}, retry.Attempts(10), retry.MaxDelay(3*time.Second))
	if err != nil {
		t.Fatal(err)
	}

	err = retry.Do(func() error {
		sinkFilters, retryErr := clientset.CoreV1().ConfigMaps("kubearchive").Get(context.Background(), "sink-filters", metav1.GetOptions{})
		if retryErr != nil {
			return retryErr
		}
		_, exists := sinkFilters.Data[namespaceName]
		if !exists {
			return fmt.Errorf("sink-filters ConfigMap does not yet have filters for the namespace %s", namespaceName)
		}
		return nil
	}, retry.Attempts(10), retry.MaxDelay(3*time.Second))
	if err != nil {
		t.Fatal(err)
	}
}

// GetSAToken waits for Service Account 'default' to exist in the namespace before creating a TokenRequest
func GetSAToken(t testing.TB, clientset kubernetes.Interface, namespaceName string) *authenticationv1.TokenRequest {
	t.Helper()
	saErr := retry.Do(func() error {
		_, err := clientset.CoreV1().ServiceAccounts(namespaceName).Get(context.Background(), "default", metav1.GetOptions{})
		return err
	}, retry.Attempts(10), retry.MaxDelay(2*time.Second))
	if saErr != nil {
		t.Logf("Could not find Service Account 'default' in namespace '%s'\n", namespaceName)
		t.Fatal(saErr)
	}

	token, tokenErr := clientset.CoreV1().ServiceAccounts(namespaceName).CreateToken(context.Background(), "default", &authenticationv1.TokenRequest{}, metav1.CreateOptions{})
	if tokenErr != nil {
		t.Logf("Could not create a token for service account 'default' in namespace '%s'\n", namespaceName)
		t.Fatal(tokenErr)
	}
	return token
}
