// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package k8s

import (
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
)

// returns a dynamic.DynamicClient. This is different than the kubernetes.ClientSet used in the api server and allows the
// sink to build a GroupVersionResource for a resource on the cluster so that the sink can delete objects of that
// resource's type.
func GetKubernetesClient() (*dynamic.DynamicClient, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("error retrieving in-cluster k8s client config: %s", err)
	}
	client, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("error instantiating k8s from host %s: %s", config.Host, err)
	}
	return client, nil
}

// returns a kubernetes.Clientset using rest.InClusterConfig() or an error if the kubernetes.Clientset could not be
// started.
func GetKubernetesClientset() (*kubernetes.Clientset, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("error retrieving in-cluster k8s client config: %s", err)
	}
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("error instantiating k8s from host %s: %s", config.Host, err)
	}
	return client, nil
}

func GetRESTMapper() (meta.RESTMapper, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("error retrieving in-cluster k8s client config: %s", err)
	}

	discoveryClient, err := discovery.NewDiscoveryClientForConfig(config)
	if err != nil {
		return nil, err
	}

	groupResources, err := restmapper.GetAPIGroupResources(discoveryClient)
	if err != nil {
		return nil, err
	}

	return restmapper.NewDiscoveryRESTMapper(groupResources), nil
}
