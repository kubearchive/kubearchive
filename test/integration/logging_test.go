// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0
//go:build integration

package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/avast/retry-go/v5"
	"github.com/kubearchive/kubearchive/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	batchv1 "k8s.io/api/batch/v1"
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
	retryErr := retry.New(retry.Attempts(60), retry.MaxDelay(2*time.Second)).Do(func() error {
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
	})

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
	retryErr := retry.New(retry.Attempts(1000), retry.MaxDelay(2*time.Second)).Do(func() error {
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
	})

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
		retryErr := retry.New(retry.Attempts(30), retry.MaxDelay(5*time.Second)).Do(func() error {
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
		})

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
			InitContainers: []corev1.Container{
				{
					Name:    "init1",
					Image:   "quay.io/fedora/fedora:latest",
					Command: []string{"echo", "-n", "I'm the container called init1."},
				},
				{
					Name:    "init2",
					Image:   "quay.io/fedora/fedora:latest",
					Command: []string{"echo", "-n", "I'm the container called init2."},
				},
			},
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
	retryErr := retry.New(retry.Attempts(60), retry.MaxDelay(2*time.Second)).Do(func() error {
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
	})

	if retryErr != nil {
		t.Fatal(retryErr)
	}

	url = fmt.Sprintf("https://localhost:%s/api/v1/namespaces/%s/pods/defaults-to-first/log?container=second", port, namespaceName)
	retryErr = retry.New(retry.Attempts(60), retry.MaxDelay(2*time.Second)).Do(func() error {
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
	})

	if retryErr != nil {
		t.Fatal(retryErr)
	}

	url = fmt.Sprintf("https://localhost:%s/api/v1/namespaces/%s/pods/defaults-to-first/log?container=init1", port, namespaceName)
	retryErr = retry.New(retry.Attempts(60), retry.MaxDelay(2*time.Second)).Do(func() error {
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
		if bodyString != "I'm the container called init1." {
			t.Logf("log does not match: %s", bodyString)
			return fmt.Errorf("log does not match the expected 'I'm the container called init1.'")
		}

		return nil
	})

	if retryErr != nil {
		t.Fatal(retryErr)
	}

	url = fmt.Sprintf("https://localhost:%s/api/v1/namespaces/%s/pods/defaults-to-first/log?container=init2", port, namespaceName)
	retryErr = retry.New(retry.Attempts(60), retry.MaxDelay(2*time.Second)).Do(func() error {
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
		if bodyString != "I'm the container called init2." {
			t.Logf("log does not match: %s", bodyString)
			return fmt.Errorf("log does not match the expected 'I'm the container called init2.'")
		}

		return nil
	})

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

	retryErr := retry.New(retry.Attempts(30), retry.MaxDelay(2*time.Second)).Do(func() error {
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
	})

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
	retryErr := retry.New(retry.Attempts(60), retry.MaxDelay(5*time.Second)).Do(func() error {
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
	})

	if retryErr != nil {
		t.Fatal(retryErr)
	}
}

func TestLogGzipCompression(t *testing.T) {
	t.Parallel()

	clientset, _ := test.GetKubernetesClient(t)
	port := test.PortForwardApiServer(t, clientset)
	namespaceName, token := test.CreateTestNamespaceWithClusterAccess(t, false, true)

	test.CreateKAC(t, "testdata/kac-with-resources.yaml", namespaceName)

	// Use RunLogGeneratorWithLines to create a job that generates 100 log lines
	jobName := test.RunLogGeneratorWithLines(t, namespaceName, 100)
	url := fmt.Sprintf("https://localhost:%s/apis/batch/v1/namespaces/%s/jobs/%s/log", port, namespaceName, jobName)

	tests := []struct {
		name           string
		headers        map[string][]string
		expectGzip     bool
		expectEncoding string
	}{
		{
			name:           "without gzip compression",
			headers:        map[string][]string{},
			expectGzip:     false,
			expectEncoding: "",
		},
		{
			name: "with gzip compression",
			headers: map[string][]string{
				"Accept-Encoding": {"gzip"},
			},
			expectGzip:     true,
			expectEncoding: "gzip",
		},
	}

	var uncompressedLogs []byte

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			var logs []byte
			var actuallyGzipped bool
			var contentEncoding string

			retryErr := retry.New(retry.Attempts(60), retry.MaxDelay(2*time.Second)).Do(func() error {
				body, gzipped, encoding, err := test.GetLogsWithGzipCheck(t, token.Status.Token, url, testCase.headers)
				if err != nil {
					return err
				}

				if len(body) == 0 {
					return errors.New("could not retrieve the pod log")
				}

				// Count lines to ensure we have the expected 100 log entries
				lines := strings.Split(strings.TrimSpace(string(body)), "\n")
				if len(lines) != 100 {
					return fmt.Errorf("expected 100 log lines, got %d", len(lines))
				}

				logs = body
				actuallyGzipped = gzipped
				contentEncoding = encoding
				t.Logf("Successfully retrieved logs %s", testCase.name)
				return nil
			})

			if retryErr != nil {
				t.Fatalf("✗ Failed to retrieve logs %s: %v", testCase.name, retryErr)
			}

			// Verify compression behavior immediately
			assert.Equal(t, testCase.expectGzip, actuallyGzipped)
			assert.Equal(t, testCase.expectEncoding, contentEncoding)
			assert.GreaterOrEqual(t, len(logs), 1000)

			// Store uncompressed logs for comparison
			if !testCase.expectGzip {
				uncompressedLogs = logs
			} else {
				// Verify that the decompressed content matches uncompressed content
				assert.True(t, bytes.Equal(uncompressedLogs, logs), "✗ Decompressed log content should match uncompressed log content")
			}

			t.Logf("✓ Log gzip compression verification passed %s - %d bytes of log content", testCase.name, len(logs))
		})
	}
}

func TestLogTailing(t *testing.T) {
	t.Parallel()

	clientset, _ := test.GetKubernetesClient(t)
	port := test.PortForwardApiServer(t, clientset)
	namespaceName, token := test.CreateTestNamespace(t, false)

	test.CreateKAC(t, "testdata/kac-with-resources.yaml", namespaceName)

	totalLines := 20
	// Create a pod that generates numbered log lines
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "tail-test",
		},
		Spec: corev1.PodSpec{
			RestartPolicy: corev1.RestartPolicyNever,
			Containers: []corev1.Container{
				{
					Name:    "logger",
					Image:   "quay.io/fedora/fedora:latest",
					Command: []string{"/bin/sh", "-c", fmt.Sprintf(`for i in $(seq 1 %d); do printf "line-%%02d\n" "$i"; done`, totalLines)},
				},
			},
		},
	}
	_, err := clientset.CoreV1().Pods(namespaceName).Create(context.Background(), pod, metav1.CreateOptions{})
	require.NoError(t, err)

	// Wait for all lines to be available via full mode
	baseURL := fmt.Sprintf("https://localhost:%s/api/v1/namespaces/%s/pods/tail-test/log", port, namespaceName)
	retryErr := retry.New(retry.Attempts(60), retry.MaxDelay(2*time.Second)).Do(func() error {
		body, err := test.GetLogs(t, token.Status.Token, baseURL)
		if err != nil {
			return err
		}
		if len(body) == 0 {
			return errors.New("could not retrieve the pod log")
		}
		lines := strings.Split(strings.TrimSpace(string(body)), "\n")
		if len(lines) != totalLines {
			return fmt.Errorf("expected %d lines, got %d", totalLines, len(lines))
		}
		return nil
	})
	require.NoError(t, retryErr, "all log lines should be available before testing tail")

	tests := []struct {
		name          string
		tailLines     int
		expectedCount int
		firstLine     string
		lastLine      string
	}{
		{
			name:          "tail 5 lines",
			tailLines:     5,
			expectedCount: 5,
			firstLine:     "line-16",
			lastLine:      "line-20",
		},
		{
			name:          "tail 1 line",
			tailLines:     1,
			expectedCount: 1,
			firstLine:     "line-20",
			lastLine:      "line-20",
		},
		{
			name:          "tail more than available",
			tailLines:     100,
			expectedCount: totalLines,
			firstLine:     "line-01",
			lastLine:      "line-20",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tailURL := fmt.Sprintf("%s?tailLines=%d", baseURL, tt.tailLines)
			retryErr := retry.New(retry.Attempts(30), retry.MaxDelay(2*time.Second)).Do(func() error {
				body, err := test.GetLogs(t, token.Status.Token, tailURL)
				if err != nil {
					return err
				}
				if len(body) == 0 {
					return errors.New("could not retrieve tailed log")
				}
				lines := strings.Split(strings.TrimSpace(string(body)), "\n")
				if len(lines) != tt.expectedCount {
					return fmt.Errorf("expected %d lines, got %d", tt.expectedCount, len(lines))
				}
				if lines[0] != tt.firstLine {
					return fmt.Errorf("expected first line '%s', got '%s'", tt.firstLine, lines[0])
				}
				if lines[len(lines)-1] != tt.lastLine {
					return fmt.Errorf("expected last line '%s', got '%s'", tt.lastLine, lines[len(lines)-1])
				}
				// Verify all lines are in sequential order
				for i := 1; i < len(lines); i++ {
					if lines[i-1] >= lines[i] {
						return fmt.Errorf("lines not in order at index %d: '%s' >= '%s'", i, lines[i-1], lines[i])
					}
				}
				t.Logf("tailLines=%d: got %d lines, first='%s', last='%s'", tt.tailLines, len(lines), lines[0], lines[len(lines)-1])
				return nil
			})
			require.NoError(t, retryErr)
		})
	}
}

func TestLargeLogRetrieval(t *testing.T) {
	t.Parallel()

	clientset, _ := test.GetKubernetesClient(t)
	port := test.PortForwardApiServer(t, clientset)
	namespaceName, token := test.CreateTestNamespace(t, false)

	test.CreateKAC(t, "testdata/kac-with-resources.yaml", namespaceName)

	expectedLines := 500000
	// Create a job that generates 500K log lines using flog with no delay
	jobName := fmt.Sprintf("large-log-%s", test.RandomString())
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: namespaceName,
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					Containers: []corev1.Container{
						{
							Name:    "flog",
							Command: []string{"flog", "-n", fmt.Sprintf("%d", expectedLines), "-d", "0"},
							Image:   "quay.io/kubearchive/mingrammer/flog",
						},
					},
				},
			},
		},
	}
	_, err := clientset.BatchV1().Jobs(namespaceName).Create(context.Background(), job, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}

	// Wait for the job to complete before attempting log retrieval
	test.WaitForJob(t, clientset, namespaceName, jobName)
	t.Log("Job completed, waiting for logs to be ingested...")

	url := fmt.Sprintf("https://localhost:%s/apis/batch/v1/namespaces/%s/jobs/%s/log", port, namespaceName, jobName)
	retryErr := retry.New(retry.Attempts(120), retry.MaxDelay(5*time.Second)).Do(func() error {
		body, err := test.GetLogs(t, token.Status.Token, url)
		if err != nil {
			return err
		}

		if len(body) == 0 {
			return errors.New("could not retrieve the pod log")
		}

		bodyString := string(body)
		lines := strings.Split(strings.TrimSpace(bodyString), "\n")
		t.Logf("Retrieved %d log lines (%d bytes)", len(lines), len(body))
		if len(lines) > expectedLines {
			// More lines than expected — likely duplicates at pagination boundaries.
			// No point retrying, fail immediately with diagnostics.
			seen := make(map[string]int, len(lines))
			for _, line := range lines {
				seen[line]++
			}
			duplicateCount := 0
			for line, count := range seen {
				if count > 1 {
					duplicateCount += count - 1
					if duplicateCount <= 10 {
						t.Logf("Duplicate line (%dx): %.100s", count, line)
					}
				}
			}
			t.Fatalf("Expected %d lines, got %d (unique: %d, duplicates: %d)", expectedLines, len(lines), len(seen), duplicateCount)
		}
		if len(lines) != expectedLines {
			return fmt.Errorf("expected %d lines, got %d", expectedLines, len(lines))
		}
		return nil
	})

	if retryErr != nil {
		t.Fatal(retryErr)
	}
}

func TestLogTailingOrder(t *testing.T) {
	t.Parallel()

	clientset, _ := test.GetKubernetesClient(t)
	port := test.PortForwardApiServer(t, clientset)
	namespaceName, token := test.CreateTestNamespace(t, false)

	test.CreateKAC(t, "testdata/kac-with-resources.yaml", namespaceName)

	// Create a pod with two log entries separated by a delay to ensure distinct timestamps
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "tail-order",
		},
		Spec: corev1.PodSpec{
			RestartPolicy: corev1.RestartPolicyNever,
			Containers: []corev1.Container{
				{
					Name:    "logger",
					Image:   "quay.io/fedora/fedora:latest",
					Command: []string{"/bin/sh", "-c", "echo First && sleep 5 && echo Second && sleep 5 && echo Third"},
				},
			},
		},
	}
	_, err := clientset.CoreV1().Pods(namespaceName).Create(context.Background(), pod, metav1.CreateOptions{})
	require.NoError(t, err)

	// Wait for all 3 lines to be available
	baseURL := fmt.Sprintf("https://localhost:%s/api/v1/namespaces/%s/pods/tail-order/log", port, namespaceName)
	retryErr := retry.New(retry.Attempts(60), retry.MaxDelay(5*time.Second)).Do(func() error {
		body, err := test.GetLogs(t, token.Status.Token, baseURL)
		if err != nil {
			return err
		}
		if len(body) == 0 {
			return errors.New("could not retrieve the pod log")
		}
		bodyString := strings.TrimSpace(string(body))
		if bodyString != "First\nSecond\nThird" {
			return fmt.Errorf("expected 'First\\nSecond\\nThird', got '%s'", bodyString)
		}
		return nil
	})
	require.NoError(t, retryErr, "all log lines should be available before testing tail order")

	// Tail the last 2 lines - should get "Second\nThird" in chronological order
	tailURL := fmt.Sprintf("%s?tailLines=2", baseURL)
	retryErr = retry.New(retry.Attempts(30), retry.MaxDelay(2*time.Second)).Do(func() error {
		body, err := test.GetLogs(t, token.Status.Token, tailURL)
		if err != nil {
			return err
		}
		if len(body) == 0 {
			return errors.New("could not retrieve tailed log")
		}
		bodyString := strings.TrimSpace(string(body))
		if bodyString != "Second\nThird" {
			return fmt.Errorf("expected 'Second\\nThird', got '%s'", bodyString)
		}
		t.Log("Tail order verified: last 2 lines returned in chronological order")
		return nil
	})
	require.NoError(t, retryErr)

	// Tail the last 1 line - should get just "Third"
	tailURL = fmt.Sprintf("%s?tailLines=1", baseURL)
	retryErr = retry.New(retry.Attempts(30), retry.MaxDelay(2*time.Second)).Do(func() error {
		body, err := test.GetLogs(t, token.Status.Token, tailURL)
		if err != nil {
			return err
		}
		if len(body) == 0 {
			return errors.New("could not retrieve tailed log")
		}
		bodyString := strings.TrimSpace(string(body))
		if bodyString != "Third" {
			return fmt.Errorf("expected 'Third', got '%s'", bodyString)
		}
		t.Log("Tail order verified: last 1 line is 'Third'")
		return nil
	})
	require.NoError(t, retryErr)
}
