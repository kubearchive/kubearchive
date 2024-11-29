// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0
package test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
	"k8s.io/client-go/util/homedir"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	letterBytes   = "abcdefghijklmnopqrstuvwxyz"
	randSuffixLen = 8
)

func RandomString() string {
	suffix := make([]byte, randSuffixLen)
	for i := range suffix {
		suffix[i] = letterBytes[rand.Intn(len(letterBytes))] // #nosec G404
	}
	return string(suffix)
}

func CreateResources(resources ...string) error {
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

		slog.Info("running ko apply -f, file kept for inspection.", "file", f.Name())
		cmd := exec.Command("ko", "apply", "-f", f.Name()) // #nosec G204
		output, err := cmd.CombinedOutput()
		if err != nil {
			return errors.New(string(output))
		}
	}

	return nil
}

func DeleteResources(resources ...string) error {
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

		slog.Info("running kubectl delete -f, file kept for inspection.", "file", f.Name())
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

// PortForward forwards the given ports until the retrieved channel is closed
func PortForward(ports []string, pod, ns string) (chan struct{}, error) {
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
			fmt.Println(out.String())
		}
	}()
	return stopChan, errReady
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
