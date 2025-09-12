// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0
//go:build integration

package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/avast/retry-go/v4"
	"github.com/kubearchive/kubearchive/test"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestLogging(t *testing.T) {
	t.Parallel()

	clientset, _ := test.GetKubernetesClient(t)
	port := test.PortForwardApiServer(t, clientset)
	namespaceName, token := test.CreateTestNamespace(t, false)

	test.CreateKAC(t, "testdata/kac-with-resources.yaml", namespaceName)
	job := test.RunLogGenerator(t, namespaceName)
	url := fmt.Sprintf("https://localhost:%s/apis/batch/v1/namespaces/%s/jobs/%s/log", port, namespaceName, job)
	retryErr := retry.Do(func() error {
		body, err := test.GetLogs(t, token.Status.Token, url)
		if err != nil {
			return err
		}

		if len(body) == 0 {
			return errors.New("could not retrieve the job log")
		}
		t.Log("Successfully retrieved logs")

		bodyString := string(body)
		if len(strings.Split(bodyString, "\n")) != 11 {
			return fmt.Errorf("expected 11 lines, currently '%d'. Trying again...", len(strings.Split(bodyString, "\n")))
		}

		return nil
	}, retry.Attempts(60), retry.MaxDelay(2*time.Second))

	if retryErr != nil {
		t.Fatal(retryErr)
	}
}

func TestLogOrder(t *testing.T) {
	t.Parallel()

	clientset, _ := test.GetKubernetesClient(t)
	port := test.PortForwardApiServer(t, clientset)
	namespaceName, token := test.CreateTestNamespace(t, false)

	test.CreateKAC(t, "testdata/kac-with-resources.yaml", namespaceName)

	// Create a pod
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "logs-order",
		},
		Spec: corev1.PodSpec{
			RestartPolicy: corev1.RestartPolicyNever,
			Containers: []corev1.Container{
				{
					Name:    "logs",
					Image:   "quay.io/fedora/fedora:latest",
					Command: []string{"/bin/sh", "-c", "echo First && sleep 10 && echo Second"},
				},
			},
		},
	}
	_, err := clientset.CoreV1().Pods(namespaceName).Create(context.Background(), pod, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}

	url := fmt.Sprintf("https://localhost:%s/api/v1/namespaces/%s/pods/logs-order/log", port, namespaceName)
	retryErr := retry.Do(func() error {
		body, err := test.GetLogs(t, token.Status.Token, url)
		if err != nil {
			return err
		}

		if len(body) == 0 {
			return errors.New("could not retrieve the pod log")
		}
		t.Log("Successfully retrieved logs")

		bodyString := string(body)
		if bodyString != "First\nSecond\n" {
			t.Log("log does not match")
			return fmt.Errorf("log does not match the expected 'First\nSecond\n'")
		}

		return nil
	}, retry.Attempts(1000), retry.MaxDelay(2*time.Second))

	if retryErr != nil {
		t.Fatal(retryErr)
	}

}

func TestDefaultContainer(t *testing.T) {
	t.Parallel()

	clientset, _ := test.GetKubernetesClient(t)
	port := test.PortForwardApiServer(t, clientset)
	namespaceName, token := test.CreateTestNamespace(t, false)

	test.CreateKAC(t, "testdata/kac-with-resources.yaml", namespaceName)

	tests := []struct {
		podName     string
		annotations map[string]string
		expectedLog string
	}{
		{
			podName:     "defaults-to-first",
			annotations: map[string]string{},
			expectedLog: "I'm the container called first.",
		},
		{
			podName: "wants-second",
			annotations: map[string]string{
				"kubectl.kubernetes.io/default-container": "second",
			},
			expectedLog: "I'm the container called second.",
		},
		{
			podName: "empty-annotation",
			annotations: map[string]string{
				"kubectl.kubernetes.io/default-container": "",
			},
			expectedLog: "I'm the container called first.",
		},
	}

	for _, testCase := range tests {
		// Create a pod
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:        testCase.podName,
				Annotations: testCase.annotations,
			},
			Spec: corev1.PodSpec{
				RestartPolicy: corev1.RestartPolicyNever,
				Containers: []corev1.Container{
					{
						Name:    "first",
						Image:   "quay.io/fedora/fedora-minimal:latest",
						Command: []string{"echo", "-n", "I'm the container called first."},
					},
					{
						Name:    "second",
						Image:   "quay.io/fedora/fedora-minimal:latest",
						Command: []string{"echo", "-n", "I'm the container called second."},
					},
				},
			},
		}
		_, err := clientset.CoreV1().Pods(namespaceName).Create(context.Background(), pod, metav1.CreateOptions{})
		if err != nil {
			t.Fatal(err)
		}
	}

	for _, testCase := range tests {
		t.Logf("checking logs for pod '%s', expected log '%s'", testCase.podName, testCase.expectedLog)
		url := fmt.Sprintf("https://localhost:%s/api/v1/namespaces/%s/pods/%s/log", port, namespaceName, testCase.podName)
		retryErr := retry.Do(func() error {
			body, err := test.GetLogs(t, token.Status.Token, url)
			if err != nil {
				return err
			}

			if len(body) == 0 {
				return errors.New("could not retrieve the pod log")
			}
			t.Log("Successfully retrieved logs")

			bodyString := string(body)
			if strings.Trim(bodyString, "\n") != testCase.expectedLog {
				t.Log("log does not match")
				return fmt.Errorf("log does not match the expected '%s'", testCase.expectedLog)
			}

			return nil
		}, retry.Attempts(30), retry.MaxDelay(5*time.Second))

		if retryErr != nil {
			t.Fatal(retryErr)
		}
	}
}

func TestQueryContainer(t *testing.T) {
	t.Parallel()

	clientset, _ := test.GetKubernetesClient(t)
	port := test.PortForwardApiServer(t, clientset)
	namespaceName, token := test.CreateTestNamespace(t, false)

	test.CreateKAC(t, "testdata/kac-with-resources.yaml", namespaceName)

	// Create a pod
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "defaults-to-first",
		},
		Spec: corev1.PodSpec{
			RestartPolicy: corev1.RestartPolicyNever,
			Containers: []corev1.Container{
				{
					Name:    "first",
					Image:   "quay.io/fedora/fedora:latest",
					Command: []string{"echo", "-n", "I'm the container called first."},
				},
				{
					Name:    "second",
					Image:   "quay.io/fedora/fedora:latest",
					Command: []string{"echo", "-n", "I'm the container called second."},
				},
			},
		},
	}
	_, err := clientset.CoreV1().Pods(namespaceName).Create(context.Background(), pod, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}

	url := fmt.Sprintf("https://localhost:%s/api/v1/namespaces/%s/pods/defaults-to-first/log", port, namespaceName)
	retryErr := retry.Do(func() error {
		body, err := test.GetLogs(t, token.Status.Token, url)
		if err != nil {
			return err
		}

		if len(body) == 0 {
			return errors.New("could not retrieve the pod log")
		}
		t.Log("Successfully retrieved logs")

		bodyString := string(body)
		if strings.Trim(bodyString, "\n") != "I'm the container called first." {
			t.Log("log does not match")
			return fmt.Errorf("log does not match the expected 'I'm the container called first.'")
		}

		return nil
	}, retry.Attempts(60), retry.MaxDelay(2*time.Second))

	if retryErr != nil {
		t.Fatal(retryErr)
	}

	url = fmt.Sprintf("https://localhost:%s/api/v1/namespaces/%s/pods/defaults-to-first/log?container=second", port, namespaceName)
	retryErr = retry.Do(func() error {
		body, err := test.GetLogs(t, token.Status.Token, url)
		if err != nil {
			return err
		}

		if len(body) == 0 {
			return errors.New("could not retrieve the pod log")
		}
		t.Log("Successfully retrieved logs")

		bodyString := string(body)
		bodyString = strings.Trim(bodyString, "\n")
		if bodyString != "I'm the container called second." {
			t.Logf("log does not match: %s", bodyString)
			return fmt.Errorf("log does not match the expected 'I'm the container called second.'")
		}

		return nil
	}, retry.Attempts(60), retry.MaxDelay(2*time.Second))

	if retryErr != nil {
		t.Fatal(retryErr)
	}

}

func TestLogsWithResourceThatHasNoPods(t *testing.T) {
	t.Parallel()

	clientset, _ := test.GetKubernetesClient(t)
	port := test.PortForwardApiServer(t, clientset)
	namespaceName, token := test.CreateTestNamespace(t, false)

	test.CreateKAC(t, "testdata/kac-with-resources.yaml", namespaceName)

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: "my-service",
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"app": "my-app",
			},
			Ports: []corev1.ServicePort{
				corev1.ServicePort{
					Protocol:   corev1.ProtocolTCP,
					Port:       80,
					TargetPort: intstr.IntOrString{IntVal: 8080},
				},
			},
		},
	}
	_, err := clientset.CoreV1().Services(namespaceName).Create(context.Background(), service, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}

	retryErr := retry.Do(func() error {
		t.Log("testing that the service is properly archived...")
		url := fmt.Sprintf("https://localhost:%s/api/v1/namespaces/%s/services", port, namespaceName)
		list, err := test.GetUrl(t, token.Status.Token, url, map[string][]string{})
		if err != nil {
			return err
		}

		if len(list.Items) != 1 {
			// Update the service to get it archived
			service.Labels = map[string]string{"timestamp": fmt.Sprintf("%d", time.Now().Unix())}
			_, updateErr := clientset.CoreV1().Services(namespaceName).Update(context.Background(), service, metav1.UpdateOptions{})
			if updateErr != nil {
				return fmt.Errorf("failed to update service: %s", err)
			}
			return fmt.Errorf("expected one service, got %d", len(list.Items))
		}

		t.Log("testing if the service returns logs...")
		url = fmt.Sprintf("https://localhost:%s/api/v1/namespaces/%s/services/my-service/log", port, namespaceName)
		_, err = test.GetLogs(t, token.Status.Token, url)
		if err != nil && err.Error() != "404" {
			return err
		}

		return nil
	}, retry.Attempts(30), retry.MaxDelay(2*time.Second))

	if retryErr != nil {
		t.Fatal(retryErr)
	}
}

// TestLogRetrievalConsecutiveNumbers tests that logs with consecutive numbers are retrieved in order
func TestLogRetrievalConsecutiveNumbers(t *testing.T) {
	t.Parallel()

	clientset, _ := test.GetKubernetesClient(t)
	port := test.PortForwardApiServer(t, clientset)
	namespaceName, token := test.CreateTestNamespace(t, false)

	test.CreateKAC(t, "testdata/kac-with-resources.yaml", namespaceName)

	// Create a pod that generates 1000 consecutive log entries
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "consecutive-logs",
		},
		Spec: corev1.PodSpec{
			RestartPolicy: corev1.RestartPolicyNever,
			Containers: []corev1.Container{
				{
					Name:  "log-generator",
					Image: "quay.io/fedora/fedora:latest",
					Command: []string{"/bin/sh", "-c", `
						for i in $(seq 1 1000); do
							echo "INFO - log-entry-$i"
						done
						for i in $(seq 1001 2000); do
							echo "SUCCESS - log-entry-$i"
						done
						for i in $(seq 2001 3000); do
							echo "ERROR - log-entry-$i"
						done
						for i in $(seq 3001 10000); do
							echo "WARN - log-entry-$i"
						done
					`},
				},
			},
		},
	}
	_, err := clientset.CoreV1().Pods(namespaceName).Create(context.Background(), pod, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}

	url := fmt.Sprintf("https://localhost:%s/api/v1/namespaces/%s/pods/consecutive-logs/log", port, namespaceName)
	retryErr := retry.Do(func() error {
		body, err := test.GetLogs(t, token.Status.Token, url)
		if err != nil {
			return err
		}

		if len(body) == 0 {
			return errors.New("could not retrieve the pod log")
		}
		t.Log("Successfully retrieved logs")

		bodyString := string(body)
		lines := strings.Split(strings.TrimSpace(bodyString), "\n")

		// Verify we got exactly 1000 log entries
		if len(lines) != 10000 {
			msg := fmt.Sprintf("expected 10000 logs, got %d", len(lines))
			t.Log(msg)
			return fmt.Errorf("%s", msg)
		}

		// Verify that each log entry is in the correct order
		for i, line := range lines {
			expectedMessage := fmt.Sprintf("log-entry-%d", i+1)
			if !strings.HasSuffix(line, expectedMessage) {
				msg := fmt.Sprintf("expected suffix '%s', got '%s'", expectedMessage, line)
				t.Log(msg)
				return fmt.Errorf("%s", msg)
			}
		}

		t.Log("All 10000 log entries are in correct order")
		return nil
	}, retry.Attempts(60), retry.MaxDelay(5*time.Second))

	if retryErr != nil {
		t.Fatal(retryErr)
	}
}
