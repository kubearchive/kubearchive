// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package k8s

import (
	"fmt"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
)

// returns a dynamic.DynamicClient. This is different than the kubernetes.ClientSet used in the api server and allows the
// sink to build a GroupVersionResource for a resource on the cluster so that the sink can delete objects of that
// resource's type
func GetKubernetesClient() (*dynamic.DynamicClient, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("Error retrieving in-cluster k8s client config: %s", err)
	}
	client, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("Error instantiating k8s from host %s: %s", config.Host, err)
	}
	return client, nil
}
