// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"context"
	"fmt"
	"strings"
	"time"

	authenticationv1 "k8s.io/api/authentication/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

type discoverer struct {
	*kubernetes.Clientset
	dynamicClient      dynamic.Interface
	connectivityTester ConnectivityTester
}

func (d *discoverer) getKubeArchiveHost(ctx context.Context) (string, error) {
	// Look for namespaces containing "kubearchive" or related names
	namespaces, err := d.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to list namespaces: %w", err)
	}

	var candidateNamespaces []string
	for _, ns := range namespaces.Items {
		nsName := ns.Name
		if nsName == "kubearchive" || strings.Contains(nsName, "kubearchive") || strings.Contains(nsName, "k9e") {
			candidateNamespaces = append(candidateNamespaces, nsName)
		}
	}

	// Look for `kubearchive-api-server` service in all namespaces until found
	var service *v1.Service
	for _, namespace := range candidateNamespaces {
		service, err = d.CoreV1().Services(namespace).Get(ctx, "kubearchive-api-server", metav1.GetOptions{})
		if err == nil {
			break
		}
	}

	if service == nil {
		return "", fmt.Errorf("could not find kubearchive-api-server Service")
	}

	host, err := d.getKubeArchiveHostFromService(ctx, service)
	if err != nil {
		fmt.Println("Failed to get kubearchive-api-server external host.")
		fmt.Println("Checking port-forward...")
		host, err = d.checkPortForward()
		if err != nil {
			fmt.Println("KubeArchive API Server is not port-forwarded")
			return "", fmt.Errorf("failed to get a host to access kubearchive")
		}
	}

	return host, nil
}

func (d *discoverer) getKubeArchiveHostFromService(ctx context.Context, service *v1.Service) (string, error) {

	// TODO Add other ingress endpoints different from Openshift routes
	// Check for OpenShift routes (works for ClusterIP services too)
	routeHost, err := d.findOpenShiftRoute(ctx, service)
	if err != nil {
		return "", fmt.Errorf("no route found for %s: %w", service.Name, err)
	}
	return routeHost, nil
}

// findOpenShiftRoute looks for an existing OpenShift route that exposes the given service
func (d *discoverer) findOpenShiftRoute(ctx context.Context, service *v1.Service) (string, error) {

	// Define the route resource
	routeGVR := schema.GroupVersionResource{
		Group:    "route.openshift.io",
		Version:  "v1",
		Resource: "routes",
	}

	// List routes in the namespace
	routes, err := d.dynamicClient.Resource(routeGVR).Namespace(service.Namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to list routes: %w", err)
	}

	// Look for a route that points to our service
	for _, route := range routes.Items {
		spec, found := route.Object["spec"].(map[string]interface{})
		if !found {
			continue
		}

		to, found := spec["to"].(map[string]interface{})
		if !found {
			continue
		}

		routeServiceName, found := to["name"].(string)
		if !found {
			continue
		}

		// Check if this route points to our service
		if routeServiceName == service.Name {
			_, hasTLS := spec["tls"]
			host, found := spec["host"].(string)
			if found && host != "" {
				if hasTLS {
					return "https://" + host, nil
				}
				return "http://" + host, nil
			}
		}
	}

	return "", fmt.Errorf("no route found for service %s", service.Name)
}

// CheckPortForward scans localhost for any port running KubeArchive
func (d *discoverer) checkPortForward() (string, error) {
	// Scan common port ranges for KubeArchive services
	portRanges := []struct {
		start, end int
	}{
		{8080, 8090}, // Common web ports
		{9080, 9090}, // Alternative ports
	}

	for _, portRange := range portRanges {
		for port := portRange.start; port <= portRange.end; port++ {
			if host := d.testPortForKubeArchive(port); host != "" {
				return host, nil
			}
		}
	}
	return "", fmt.Errorf("no port-forward found")
}

// testPortForKubeArchive tests if a specific port is running KubeArchive
func (d *discoverer) testPortForKubeArchive(port int) string {
	httpsHost := fmt.Sprintf("https://localhost:%d", port)
	if d.connectivityTester.TestKubeArchiveLivezEndpoint(httpsHost, true, nil) == nil {
		return httpsHost
	}
	return ""
}

func (d *discoverer) getTokenForSA(serviceAccount, namespace string) (string, error) {
	tokenRequest := &authenticationv1.TokenRequest{
		Spec: authenticationv1.TokenRequestSpec{
			ExpirationSeconds: func() *int64 {
				duration := int64(24 * 60 * 60) // 24 hours
				return &duration
			}(),
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	token, err := d.CoreV1().ServiceAccounts(namespace).CreateToken(ctx, serviceAccount, tokenRequest, metav1.CreateOptions{})
	if token != nil {
		return token.Status.Token, nil
	}
	return "", fmt.Errorf("failed to create token for service account '%s' in namespace '%s': %w", serviceAccount, namespace, err)
}
