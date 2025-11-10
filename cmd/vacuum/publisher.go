// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"log/slog"
	"strings"

	ce "github.com/cloudevents/sdk-go/v2"
	cehttp "github.com/cloudevents/sdk-go/v2/protocol/http"
	kubearchiveapi "github.com/kubearchive/kubearchive/cmd/operator/api/v1"
	kcel "github.com/kubearchive/kubearchive/pkg/cel"
	publisher "github.com/kubearchive/kubearchive/pkg/cloudevents"
	"github.com/kubearchive/kubearchive/pkg/constants"
	"github.com/kubearchive/kubearchive/pkg/filters"
	"github.com/kubearchive/kubearchive/pkg/k8sclient"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/restmapper"
)

type VacuumCloudEventPublisher struct {
	publisher  *publisher.SinkCloudEventPublisher
	dynaClient *dynamic.DynamicClient
	mapper     meta.RESTMapper
	filters    map[string]map[string]filters.CelExpressions
}

func NewVacuumCloudEventPublisher(source string, filters map[string]map[string]filters.CelExpressions) (*VacuumCloudEventPublisher, error) {
	vcep := &VacuumCloudEventPublisher{
		filters: filters,
	}

	var err error
	if vcep.publisher, err = publisher.NewSinkCloudEventPublisher(source); err != nil {
		return nil, err
	}

	discoveryClient, err := k8sclient.NewInstrumentedDiscoveryClient()
	if err != nil {
		slog.Error("Unable to get discoveryClient", "error", err)
		return nil, err
	}

	var groupResources []*restmapper.APIGroupResources
	if groupResources, err = restmapper.GetAPIGroupResources(discoveryClient); err != nil {
		slog.Error("Unable to get groupResources", "error", err)
		return nil, err
	}

	vcep.mapper = restmapper.NewDiscoveryRESTMapper(groupResources)

	if vcep.dynaClient, err = k8sclient.NewInstrumentedDynamicClient(); err != nil {
		slog.Error("Unable to get dynamic client", "error", err)
		return nil, err
	}

	return vcep, nil
}

func (vcep *VacuumCloudEventPublisher) SendByGVK(ctx context.Context, eventTypePrefix string, avk *kubearchiveapi.APIVersionKind, namespace string) {
	// This method is currently unused but kept for potential future use
}

func (vcep *VacuumCloudEventPublisher) SendByNamespace(ctx context.Context, eventTypePrefix string, namespace string) {
	// Iterate over filters to determine which resource types to process
	for kindAPIVersion := range vcep.filters {
		// Parse the kindAPIVersion back into APIVersionKind
		parts := strings.Split(kindAPIVersion, "-")
		if len(parts) != 2 {
			continue
		}
		avk := kubearchiveapi.APIVersionKind{
			Kind:       parts[0],
			APIVersion: parts[1],
		}
		vcep.sendByAPIVersionKind(ctx, eventTypePrefix, namespace, &avk)
	}
}

func (vcep *VacuumCloudEventPublisher) SendByAPIVersionKind(ctx context.Context, eventTypePrefix string, namespace string, avk *kubearchiveapi.APIVersionKind) {
	vcep.sendByAPIVersionKind(ctx, eventTypePrefix, namespace, avk)
}

func (vcep *VacuumCloudEventPublisher) sendByAPIVersionKind(ctx context.Context, eventTypePrefix string, namespace string, avk *kubearchiveapi.APIVersionKind) {
	gvr, err := getGVR(vcep.mapper, avk)
	if err != nil {
		slog.Error("Unable to get GVR", "error", err)
		return
	}

	list, err := vcep.dynaClient.Resource(gvr).Namespace(namespace).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		slog.Error("Unable to list resources", "error", err)
		return
	}

	for _, item := range list.Items {
		metadata := item.Object["metadata"].(map[string]interface{})
		name := metadata["name"].(string)

		eventTypeSuffix := vcep.shouldSend(avk, &item)
		if eventTypeSuffix != "" {
			eventType := eventTypePrefix + "." + eventTypeSuffix
			sendResult := vcep.publisher.Send(ctx, eventType, item.Object)
			var httpResult *cehttp.Result
			statusCode := 0
			if ce.ResultAs(sendResult, &httpResult) {
				statusCode = httpResult.StatusCode
			}
			var msg string
			if ce.IsACK(sendResult) {
				msg = "Event sent successfully"
			} else {
				msg = "Event send failed"
			}
			slog.Info(msg, "apiversion", avk.APIVersion, "kind", avk.Kind, "namespace", namespace, "name", name, "eventType", eventType, "code", statusCode)
		} else {
			slog.Info("No event sent", "apiversion", avk.APIVersion, "kind", avk.Kind, "namespace", namespace, "name", name)
		}
	}
}

func getGVR(mapper meta.RESTMapper, avk *kubearchiveapi.APIVersionKind) (schema.GroupVersionResource, error) {
	apiGroup := ""
	apiVersion := avk.APIVersion
	data := strings.Split(apiVersion, "/")
	if len(data) > 1 {
		apiGroup = data[0]
		apiVersion = data[1]
	}
	mapping, err := mapper.RESTMapping(schema.GroupKind{Group: apiGroup, Kind: avk.Kind}, apiVersion)
	if err != nil {
		return schema.GroupVersionResource{}, err
	}
	return mapping.Resource, nil
}

func (vcep *VacuumCloudEventPublisher) shouldSend(avk *kubearchiveapi.APIVersionKind, resource *unstructured.Unstructured) string {
	key := avk.Key()
	resourceFilters, hasFilters := vcep.filters[key]
	if hasFilters {
		namespace := resource.GetNamespace()

		globalCel, globalExists := resourceFilters[constants.SinkFilterGlobalNamespace]
		namespaceCel, namespaceExists := resourceFilters[namespace]

		if (globalExists && kcel.ExecuteBooleanCEL(context.Background(), globalCel.DeleteWhen, resource)) ||
			(namespaceExists && kcel.ExecuteBooleanCEL(context.Background(), namespaceCel.DeleteWhen, resource)) {
			return "delete-when"
		}

		if (globalExists && kcel.ExecuteBooleanCEL(context.Background(), globalCel.ArchiveWhen, resource)) ||
			(namespaceExists && kcel.ExecuteBooleanCEL(context.Background(), namespaceCel.ArchiveWhen, resource)) {
			return "archive-when"
		}
	}

	return ""
}
