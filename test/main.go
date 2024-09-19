package test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
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

		log.Printf("running ko apply -f %s, file kept for inspection.", f.Name())
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

		log.Printf("running kubectl delete -f %s, file kept for inspection.", f.Name())
		cmd := exec.Command("kubectl", "delete", "-f", f.Name()) // #nosec G204
		output, err := cmd.CombinedOutput()
		if err != nil {
			return errors.New(string(output))
		}
	}

	return nil
}

func GetKubernetesClient() (*kubernetes.Clientset, error) {
	var kubeconfig string
	if home := homedir.HomeDir(); home != "" {
		kubeconfig = filepath.Join(home, ".kube", "config")
	}

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, err
	}

	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	return client, nil
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
