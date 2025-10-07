// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"fmt"
	"log/slog"

	kubearchiveapi "github.com/kubearchive/kubearchive/cmd/operator/api/v1"
	publisher "github.com/kubearchive/kubearchive/pkg/cloudevents"
	"github.com/kubearchive/kubearchive/pkg/constants"
	"github.com/kubearchive/kubearchive/pkg/k8sclient"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
)

func clusterVacuum(configName string) error {
	client, err := k8sclient.NewInstrumentedDynamicClient()
	if err != nil {
		return fmt.Errorf("unable to get client: %v", err)
	}

	scep, err := publisher.NewSinkCloudEventPublisher("localhost:8080:/foo", "org.kubearchive.vacuum.update")
	if err != nil {
		return fmt.Errorf("unable to create sink cloudevent publisher: %v", err)
	}

	obj, err := client.Resource(kubearchiveapi.ClusterVacuumConfigGVR).Namespace(constants.KubeArchiveNamespace).Get(context.Background(), configName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("unable to get ClusterVacuumConfig '%s': %v", configName, err)
	}

	config, err := kubearchiveapi.ConvertUnstructuredToClusterVacuumConfig(obj)
	if err != nil {
		return fmt.Errorf("unable to convert to ClusterVacuumConfig '%s': %v", configName, err)
	}

	var namespaces = getNamespacesFromClusterVacuumConfig(config)
	allResources, allNS := config.Spec.Namespaces[constants.ClusterVacuumAllNamespaces]
	if len(namespaces) == 0 || allNS {
		namespaces, err = getNamespacesFromSinkFilter(client)
		if err != nil {
			return fmt.Errorf("unable to get all namespaces from SinkFilters: %v", err)
		}
	}

	for namespace := range namespaces {
		slog.Info("Started publishing sink events", "namespace", namespace)

		value, ok := config.Spec.Namespaces[namespace]
		if ok {
			// Namespace explicitly specified in VacuumClusterConfig
			if len(value.Resources) == 0 {
				_, err = scep.SendByNamespace(context.Background(), namespace)
				if err != nil {
					slog.Error("Unable to send messages for namespace '" + namespace + "'")
				}
			} else {
				for _, avk := range value.Resources {
					scep.SendByAPIVersionKind(context.Background(), namespace, &avk)
				}
			}
		} else {
			// Only way to get here is if allNS is true.
			if len(allResources.Resources) == 0 {
				_, err = scep.SendByNamespace(context.Background(), namespace)
				if err != nil {
					slog.Error("Unable to send messages for namespace '" + namespace + "'")
				}
			} else {
				for _, avk := range allResources.Resources {
					scep.SendByAPIVersionKind(context.Background(), namespace, &avk)
				}
			}
		}
		slog.Info("Finished publishing sink events", "namespace", namespace)
	}

	return nil
}

func getNamespacesFromSinkFilter(client dynamic.Interface) (map[string]struct{}, error) {
	obj, err := client.Resource(kubearchiveapi.SinkFilterGVR).Namespace(constants.KubeArchiveNamespace).Get(context.Background(), constants.SinkFilterResourceName, metav1.GetOptions{})
	if err != nil {
		slog.Error("Unable to get SinkFilter", "error", err, "name", constants.SinkFilterResourceName)
		return nil, err
	}

	sf, err := kubearchiveapi.ConvertUnstructuredToSinkFilter(obj)
	if err != nil {
		slog.Error("Unable to convert to SinkFilter", "error", err, "name", constants.SinkFilterResourceName)
		return nil, err
	}

	namespaces := map[string]struct{}{}
	for namespace := range sf.Spec.Namespaces {
		if namespace != constants.SinkFilterGlobalNamespace {
			namespaces[namespace] = struct{}{}
		}
	}
	return namespaces, nil
}

func getNamespacesFromClusterVacuumConfig(config *kubearchiveapi.ClusterVacuumConfig) map[string]struct{} {
	namespaces := map[string]struct{}{}
	for namespace := range config.Spec.Namespaces {
		if namespace != constants.ClusterVacuumAllNamespaces {
			namespaces[namespace] = struct{}{}
		}
	}
	return namespaces
}
