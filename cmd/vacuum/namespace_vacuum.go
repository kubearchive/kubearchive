// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"

	kubearchiveapi "github.com/kubearchive/kubearchive/cmd/operator/api/v1alpha1"
	"github.com/kubearchive/kubearchive/cmd/sink/k8s"
	publisher "github.com/kubearchive/kubearchive/pkg/cloudevents"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	sourcesv1 "knative.dev/eventing/pkg/apis/sources/v1"
)

func namespaceVacuum(configName string) {
	namespace := os.Getenv("NAMESPACE")
	if namespace == "" {
		slog.Error("No NAMESPACE environment variable set")
		return
	}

	client, err := k8s.GetKubernetesClient()
	if err != nil {
		slog.Error("Unable to get client", "error", err)
		return
	}

	scep, err := publisher.NewSinkCloudEventPublisher("localhost:8080:/foo", "org.kubearchive.vacuum.update")
	if err != nil {
		slog.Error("Unable to create sink cloudevent publisher", "error", err)
		return
	}

	obj, err := client.Resource(kubearchiveapi.NamespaceVacuumConfigGVR).Namespace(namespace).Get(context.Background(), configName, metav1.GetOptions{})
	if err != nil {
		slog.Error("Unable to get NamespaceVacuumConfig", "error", err, "name", configName)
		return
	}

	config, err := kubearchiveapi.ConvertUnstructuredToNamespaceVacuumConfig(obj)
	if err != nil {
		slog.Error("Unable to convert to NamespaceVacuumConfig", "error", err, "name", configName)
		return
	}

	slog.Info("Started publishing sink events", "namespace", namespace)
	results := map[sourcesv1.APIVersionKind][]publisher.SinkCloudEventPublisherResult{}
	if len(config.Spec.Resources) == 0 {
		results, err = scep.SendByNamespace(context.Background(), namespace)
		if err != nil {
			slog.Error("Unable to send events for namespace", "error", err, "namespace", namespace, "name", configName)
			return
		}
	} else {
		for _, avk := range config.Spec.Resources {
			results[avk] = scep.SendByAPIVersionKind(context.Background(), namespace, &avk)
		}
	}
	slog.Info("Finished publishing sink events", "namespace", namespace)

	pretty := map[string][]publisher.SinkCloudEventPublisherResult{}
	for key, value := range results {
		pretty[fmt.Sprintf("%v", key)] = value
	}
	jsonString, err := json.MarshalIndent(pretty, "", "  ")
	if err != nil {
		slog.Error("Unable to marshal results into JSON string", "error", err)
	} else {
		slog.Info("Namespace vacuum results:\n" + string(jsonString))
	}
}
