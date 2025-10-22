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
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TestKubeArchiveDeployments is redundant with the kubectl rollout status from the hack/quick-install.sh
// ,but it serves as a valid integration test, not a dummy that is not testing anything real.
func TestAllDeploymentsReady(t *testing.T) {
	t.Parallel()

	client, _ := test.GetKubernetesClient(t)

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

func TestNormalOperation(t *testing.T) {
	t.Parallel()

	clientset, _ := test.GetKubernetesClient(t)
	port := test.PortForwardApiServer(t, clientset)
	namespaceName, token := test.CreateTestNamespace(t, false)
	test.CreateKAC(t, "testdata/kac-with-resources.yaml", namespaceName)
	test.RunLogGenerator(t, namespaceName)

	// Retrieve the objects from the DB using the API.
	url := fmt.Sprintf("https://localhost:%s/apis/batch/v1/namespaces/%s/jobs", port, namespaceName)

	// Retrieve the objects from the DB using the API.
	retryErr := retry.Do(func() error {
		list, getUrlErr := test.GetUrl(t, token.Status.Token, url, map[string][]string{})
		if getUrlErr != nil {
			return getUrlErr
		}

		if len(list.Items) == 1 {
			t.Log("✓ Successfully retrieved Job from the API")
			return nil
		}
		return errors.New("could not retrieve a Job from the API")
	}, retry.Attempts(20), retry.MaxDelay(2*time.Second))

	if retryErr != nil {
		t.Fatal(retryErr)
	}
}

func TestGzipCompression(t *testing.T) {
	t.Parallel()

	clientset, _ := test.GetKubernetesClient(t)
	port := test.PortForwardApiServer(t, clientset)
	namespaceName, token := test.CreateTestNamespaceWithClusterAccess(t, false, true)
	test.CreateKAC(t, "testdata/kac-with-resources.yaml", namespaceName)

	// Run a few jobs to generate data, but don't wait for completion
	// Instead, we'll wait for the API to have substantial data
	const numJobs = 3
	for i := 0; i < numJobs; i++ {
		test.RunLogGenerator(t, namespaceName)
		t.Logf("Started job %d/%d", i+1, numJobs)
	}

	t.Logf("Started %d jobs, will wait for substantial data in API responses", numJobs)

	// Test different API endpoints with and without gzip compression
	testCases := []struct {
		name        string
		url         string
		description string
		expectGzip  bool
	}{
		{
			name:        "Core API pods endpoint (namespaced)",
			url:         fmt.Sprintf("https://localhost:%s/api/v1/namespaces/%s/pods", port, namespaceName),
			description: "Core API namespaced endpoint should support gzip compression",
			expectGzip:  true,
		},
		{
			name:        "Core API pods endpoint (cluster-wide)",
			url:         fmt.Sprintf("https://localhost:%s/api/v1/pods", port),
			description: "Core API cluster-wide endpoint should support gzip compression",
			expectGzip:  true,
		},
		{
			name:        "APIs jobs endpoint (namespaced)",
			url:         fmt.Sprintf("https://localhost:%s/apis/batch/v1/namespaces/%s/jobs", port, namespaceName),
			description: "APIs namespaced endpoint should support gzip compression",
			expectGzip:  true,
		},
		{
			name:        "APIs jobs endpoint (cluster-wide)",
			url:         fmt.Sprintf("https://localhost:%s/apis/batch/v1/jobs", port),
			description: "APIs cluster-wide endpoint should support gzip compression",
			expectGzip:  true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			var uncompressedResponse *test.GzipTestResponse
			var compressedResponse *test.GzipTestResponse

			// Test without gzip compression - wait for substantial data
			retryErr := retry.Do(func() error {
				gzipResp, list, getUrlErr := test.GetUrlWithGzipCheck(t, token.Status.Token, testCase.url, map[string][]string{})
				if getUrlErr != nil {
					return getUrlErr
				}

				// Wait for substantial data (multiple items or large response)
				if len(list.Items) < 3 && len(gzipResp.Body) < 1024 {
					return fmt.Errorf("waiting for substantial data: got %d items, %d bytes", len(list.Items), len(gzipResp.Body))
				}

				uncompressedResponse = gzipResp
				t.Logf("%s works without gzip compression - %d items, %d bytes, Content-Encoding: %s, Actually compressed: %v",
					testCase.description, len(list.Items), len(gzipResp.Body), gzipResp.ContentEncoding, gzipResp.IsActuallyGzipped)
				return nil
			}, retry.Attempts(30), retry.MaxDelay(5*time.Second))

			if retryErr != nil {
				t.Fatalf("✗ Failed to test %s without gzip: %v", testCase.name, retryErr)
			}

			// Verify uncompressed response immediately
			assert.False(t, uncompressedResponse.IsActuallyGzipped, "Response without Accept-Encoding: gzip should not be compressed")
			assert.NotEqual(t, "gzip", uncompressedResponse.ContentEncoding, "Response without Accept-Encoding: gzip should not have Content-Encoding: gzip header")
			assert.GreaterOrEqual(t, len(uncompressedResponse.Body), 1024, "Expected substantial response data (>1024 bytes) after running %d jobs", numJobs)

			t.Logf("Uncompressed response validated - %d bytes, proceeding with compression test", len(uncompressedResponse.Body))

			// Test with gzip compression
			retryErr = retry.Do(func() error {
				headers := map[string][]string{
					"Accept-Encoding": {"gzip"},
				}
				gzipResp, list, getUrlErr := test.GetUrlWithGzipCheck(t, token.Status.Token, testCase.url, headers)
				if getUrlErr != nil {
					return getUrlErr
				}

				// Wait for substantial data (multiple items or large response)
				if len(list.Items) < 3 && len(gzipResp.Body) < 1024 {
					return fmt.Errorf("waiting for substantial data: got %d items, %d bytes", len(list.Items), len(gzipResp.Body))
				}

				compressedResponse = gzipResp
				t.Logf("%s works with gzip compression - %d items, %d bytes, Content-Encoding: %s, Actually compressed: %v, Compressed size: %d bytes, Decompressed size: %d bytes",
					testCase.description, len(list.Items), len(gzipResp.Body), gzipResp.ContentEncoding, gzipResp.IsActuallyGzipped, gzipResp.CompressedSize, gzipResp.DecompressedSize)
				return nil
			}, retry.Attempts(30), retry.MaxDelay(5*time.Second))

			if retryErr != nil {
				t.Fatalf("✗ Failed to test %s with gzip: %v", testCase.name, retryErr)
			}

			// Verify compressed response immediately
			if testCase.expectGzip {
				assert.True(t, compressedResponse.IsActuallyGzipped, "Response with Accept-Encoding: gzip should be compressed when data is substantial (%d bytes)", len(uncompressedResponse.Body))
				assert.Equal(t, "gzip", compressedResponse.ContentEncoding, "Response with Accept-Encoding: gzip should have Content-Encoding: gzip header")

				// Verify that compression actually reduces size
				compressionRatio := float64(compressedResponse.CompressedSize) / float64(compressedResponse.DecompressedSize)
				t.Logf("✓ Compression ratio: %.2f (compressed: %d bytes, decompressed: %d bytes)",
					compressionRatio, compressedResponse.CompressedSize, compressedResponse.DecompressedSize)

				assert.Less(t, compressionRatio, 1.0, "Compression should reduce size, but compressed size (%d) >= decompressed size (%d)",
					compressedResponse.CompressedSize, compressedResponse.DecompressedSize)

				t.Logf("✓ Compression working effectively with %d%% size reduction",
					int((1.0-compressionRatio)*100))

				t.Logf("✓ Gzip compression verification passed for %s", testCase.name)
			}
		})
	}
}

func TestKindNotFound(t *testing.T) {
	t.Parallel()

	clientset, _ := test.GetKubernetesClient(t)
	port := test.PortForwardApiServer(t, clientset)
	namespaceName, token := test.CreateTestNamespace(t, false)

	// Retrieve the objects from the DB using the API.
	url := fmt.Sprintf("https://localhost:%s/apis/batch/v1/namespaces/%s/pobs", port, namespaceName)
	retryErr := retry.Do(func() error {
		_, getUrlErr := test.GetUrl(t, token.Status.Token, url, map[string][]string{})

		if strings.Contains(getUrlErr.Error(), "404") {
			return nil
		}

		return errors.New("expecting 404")
	}, retry.Attempts(5), retry.MaxDelay(5*time.Second))

	if retryErr != nil {
		t.Fatal(retryErr)
	}
}
