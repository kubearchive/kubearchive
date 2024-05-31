package test

import (
	"errors"
	"log"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
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
