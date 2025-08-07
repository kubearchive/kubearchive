// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"

	kubearchiveapi "github.com/kubearchive/kubearchive/cmd/operator/api/v1"
	publisher "github.com/kubearchive/kubearchive/pkg/cloudevents"
	"github.com/kubearchive/kubearchive/pkg/k8sclient"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	sourcesv1 "knative.dev/eventing/pkg/apis/sources/v1"
)

func namespaceVacuum(configName string) error {
	namespace := os.Getenv("NAMESPACE")
	if namespace == "" {
		return errors.New("no NAMESPACE environment variable set")
	}

	client, err := k8sclient.NewInstrumentedDynamicClient()
	if err != nil {
		return fmt.Errorf("unable to get client: %v", err)
	}

	scep, err := publisher.NewSinkCloudEventPublisher("localhost:8080:/foo", "org.kubearchive.vacuum.update")
	if err != nil {
		return fmt.Errorf("unable to create sink cloudevent publisher: %v", err)
	}

	obj, err := client.Resource(kubearchiveapi.NamespaceVacuumConfigGVR).Namespace(namespace).Get(context.Background(), configName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("unable to get NamespaceVacuumConfig '%s': %v", configName, err)
	}

	config, err := kubearchiveapi.ConvertUnstructuredToNamespaceVacuumConfig(obj)
	if err != nil {
		return fmt.Errorf("unable to convert to NamespaceVacuumConfig '%s': %v", configName, err)
	}

	slog.Info("Started publishing sink events", "namespace", namespace)
	results := map[sourcesv1.APIVersionKind][]publisher.SinkCloudEventPublisherResult{}
	if len(config.Spec.Resources) == 0 {
		results, err = scep.SendByNamespace(context.Background(), namespace)
		if err != nil {
			slog.Error("Unable to send events for NamespaceVacuumConfig", "error", err, "namespace", namespace, "config", configName)
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

	return nil
}
