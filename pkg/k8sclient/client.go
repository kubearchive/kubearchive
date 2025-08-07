// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package k8sclient

import (
	"fmt"
	"net/http"

	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// getInstrumentedInClusterConfig loads the in-cluster configuration and wraps
// it with OpenTelemetry instrumentation for observability. This consolidates
// the common logic for all client creation functions.
func getInstrumentedInClusterConfig() (*rest.Config, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("error retrieving in-cluster k8s client config: %w", err)
	}

	// Wrap the transport with our instrumented round tripper
	config.Wrap(func(rt http.RoundTripper) http.RoundTripper {
		return NewInstrumentedRoundTripper(rt)
	})

	return config, nil
}

// NewInstrumentedKubernetesClient creates an instrumented Kubernetes clientset
// using in-cluster configuration.
func NewInstrumentedKubernetesClient() (*kubernetes.Clientset, error) {
	config, err := getInstrumentedInClusterConfig()
	if err != nil {
		return nil, err
	}

	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("error creating instrumented kubernetes client: %w", err)
	}

	return client, nil
}

// NewInstrumentedDynamicClient creates an instrumented dynamic client
// using in-cluster configuration.
func NewInstrumentedDynamicClient() (*dynamic.DynamicClient, error) {
	config, err := getInstrumentedInClusterConfig()
	if err != nil {
		return nil, err
	}

	client, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("error creating instrumented dynamic client: %w", err)
	}

	return client, nil
}

// NewInstrumentedDiscoveryClient creates an instrumented discovery client
// using in-cluster configuration.
func NewInstrumentedDiscoveryClient() (*discovery.DiscoveryClient, error) {
	config, err := getInstrumentedInClusterConfig()
	if err != nil {
		return nil, err
	}

	client, err := discovery.NewDiscoveryClientForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("error creating instrumented discovery client: %w", err)
	}

	return client, nil
}
