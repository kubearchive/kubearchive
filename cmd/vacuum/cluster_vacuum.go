// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	kubearchiveapi "github.com/kubearchive/kubearchive/cmd/operator/api/v1"
	publisher "github.com/kubearchive/kubearchive/pkg/cloudevents"
	"github.com/kubearchive/kubearchive/pkg/constants"
	"github.com/kubearchive/kubearchive/pkg/k8sclient"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	sourcesv1 "knative.dev/eventing/pkg/apis/sources/v1"
)

const (
	clusterVacuumEventType = "kubearchive.org.vacuum.cluster.resource.update"
)

func clusterVacuum(configName string) error {
	client, err := k8sclient.NewInstrumentedDynamicClient()
	if err != nil {
		return fmt.Errorf("unable to get client: %v", err)
	}

	scep, err := publisher.NewSinkCloudEventPublisher("kubearchive.org/cluster-vacuum")
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

	results := map[string]map[sourcesv1.APIVersionKind][]publisher.SinkCloudEventPublisherResult{}

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
		res := map[sourcesv1.APIVersionKind][]publisher.SinkCloudEventPublisherResult{}

		value, ok := config.Spec.Namespaces[namespace]
		if ok {
			// Namespace explicitly specified in VacuumClusterConfig
			if len(value.Resources) == 0 {
				res, err = scep.SendByNamespace(context.Background(), clusterVacuumEventType, namespace)
				if err != nil {
					slog.Error("Unable to send messages for namespace '" + namespace + "'")
				}
			} else {
				for _, avk := range value.Resources {
					res[avk] = scep.SendByAPIVersionKind(context.Background(), clusterVacuumEventType, namespace, &avk)
				}
			}
			results[namespace] = res
		} else {
			// Only way to get here is if allNS is true.
			if len(allResources.Resources) == 0 {
				res, err = scep.SendByNamespace(context.Background(), clusterVacuumEventType, namespace)
				if err != nil {
					slog.Error("Unable to send messages for namespace '" + namespace + "'")
				}
			} else {
				for _, avk := range allResources.Resources {
					res[avk] = scep.SendByAPIVersionKind(context.Background(), clusterVacuumEventType, namespace, &avk)
				}
			}
			results[namespace] = res
		}
		slog.Info("Finished publishing sink events", "namespace", namespace)
	}

	pretty := map[string]map[string][]publisher.SinkCloudEventPublisherResult{}
	for k, v := range results {
		pretty[k] = map[string][]publisher.SinkCloudEventPublisherResult{}
		for key, value := range v {
			pretty[k][fmt.Sprintf("%v", key)] = value
		}
	}
	jsonString, err := json.MarshalIndent(pretty, "", "  ")
	if err != nil {
		slog.Error("Unable to marshal results into JSON string", "error", err)
	} else {
		slog.Info("Cluster vacuum results:\n" + string(jsonString))
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
