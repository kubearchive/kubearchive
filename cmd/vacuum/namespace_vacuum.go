// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"

	kubearchiveapi "github.com/kubearchive/kubearchive/cmd/operator/api/v1"
	"github.com/kubearchive/kubearchive/pkg/filters"
	"github.com/kubearchive/kubearchive/pkg/k8sclient"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	namespaceVacuumEventTypePrefix = "org.kubearchive.vacuum.namespace.resource"
)

func namespaceVacuum(configName string) error {
	namespace := os.Getenv("NAMESPACE")
	if namespace == "" {
		return errors.New("no NAMESPACE environment variable set")
	}

	// Get SinkFilter data for namespace vacuum (single namespace + global)
	filterReader, err := filters.NewSinkFilterReader()
	if err != nil {
		return fmt.Errorf("unable to create SinkFilter reader: %v", err)
	}

	filters, err := filterReader.ProcessSingleNamespace(context.Background(), namespace)
	if err != nil {
		return fmt.Errorf("unable to get SinkFilter data: %v", err)
	}

	client, err := k8sclient.NewInstrumentedDynamicClient()
	if err != nil {
		return fmt.Errorf("unable to get client: %v", err)
	}

	vcep, err := NewVacuumCloudEventPublisher("kubearchive.org/namespace-vacuum", filters)
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
	if len(config.Spec.Resources) == 0 {
		vcep.SendByNamespace(context.Background(), namespaceVacuumEventTypePrefix, namespace)
	} else {
		for _, avk := range config.Spec.Resources {
			vcep.SendByAPIVersionKind(context.Background(), namespaceVacuumEventTypePrefix, namespace, &avk)
		}
	}
	slog.Info("Finished publishing sink events", "namespace", namespace)

	return nil
}
