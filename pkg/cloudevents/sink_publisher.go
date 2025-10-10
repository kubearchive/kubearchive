// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package cloudevents

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	otelObs "github.com/cloudevents/sdk-go/observability/opentelemetry/v2/client"
	ce "github.com/cloudevents/sdk-go/v2"
	"github.com/cloudevents/sdk-go/v2/client"
	cehttp "github.com/cloudevents/sdk-go/v2/protocol/http"
	kubearchiveapi "github.com/kubearchive/kubearchive/cmd/operator/api/v1"
	"github.com/kubearchive/kubearchive/pkg/constants"
	"github.com/kubearchive/kubearchive/pkg/k8sclient"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/restmapper"
	sourcesv1 "knative.dev/eventing/pkg/apis/sources/v1"
)

type SinkCloudEventPublisherResult struct {
	Name       string
	Message    string
	StatusCode int
}

type SinkCloudEventPublisher struct {
	httpClient      client.Client
	dynaClient      *dynamic.DynamicClient
	mapper          meta.RESTMapper
	target          string
	source          string
	globalResources map[sourcesv1.APIVersionKind]kubearchiveapi.KubeArchiveConfigResource
}

func NewSinkCloudEventPublisher(source string) (*SinkCloudEventPublisher, error) {
	scep := &SinkCloudEventPublisher{source: source}

	var err error
	if scep.httpClient, err = otelObs.NewClientHTTP([]cehttp.Option{}, []client.Option{}); err != nil {
		slog.Error("Failed to create client", "error", err)
		return nil, err
	}

	// Always use sink service URL since we're no longer using Knative
	scep.target = getSinkServiceUrl()

	var discoveryClient *discovery.DiscoveryClient
	if discoveryClient, err = k8sclient.NewInstrumentedDiscoveryClient(); err != nil {
		slog.Error("Unable to get discoveryClient", "error", err)
		return nil, err
	}

	var groupResources []*restmapper.APIGroupResources
	if groupResources, err = restmapper.GetAPIGroupResources(discoveryClient); err != nil {
		slog.Error("Unable to get groupResources", "error", err)
		return nil, err
	}

	scep.mapper = restmapper.NewDiscoveryRESTMapper(groupResources)

	if scep.dynaClient, err = k8sclient.NewInstrumentedDynamicClient(); err != nil {
		slog.Error("Unable to get dynamic client", "error", err)
		return nil, err
	}

	if scep.globalResources, err = scep.getKubeArchiveConfigResources(""); err != nil {
		slog.Error("Unable to get global resources", "error", err)
		return nil, err
	}

	return scep, nil
}

func (scep *SinkCloudEventPublisher) SendByGVK(ctx context.Context, eventType string, avk *sourcesv1.APIVersionKind, namespace string) []SinkCloudEventPublisherResult {

	return []SinkCloudEventPublisherResult{}
}

func (scep *SinkCloudEventPublisher) SendByNamespace(ctx context.Context, eventType string, namespace string) (map[sourcesv1.APIVersionKind][]SinkCloudEventPublisherResult, error) {

	localResources, err := scep.getKubeArchiveConfigResources(namespace)
	if err != nil {
		slog.Error("Unable to get local KubeArchiveConfig resources", "error", err)
		return nil, err
	}

	results := map[sourcesv1.APIVersionKind][]SinkCloudEventPublisherResult{}
	allResources := mergeResources(scep.globalResources, localResources)

	for avk := range allResources {
		results[avk] = scep.sendByAPIVersionKind(ctx, eventType, namespace, localResources, &avk)
	}

	return results, nil
}

func (scep *SinkCloudEventPublisher) SendByAPIVersionKind(ctx context.Context, eventType string, namespace string, avk *sourcesv1.APIVersionKind) []SinkCloudEventPublisherResult {
	localResources, err := scep.getKubeArchiveConfigResources(namespace)
	if err != nil {
		slog.Error("Unable to get local KubeArchiveConfig resources", "error", err)
		return []SinkCloudEventPublisherResult{}
	}

	return scep.sendByAPIVersionKind(ctx, eventType, namespace, localResources, avk)
}

func (scep *SinkCloudEventPublisher) sendByAPIVersionKind(ctx context.Context, eventType string, namespace string, localResources map[sourcesv1.APIVersionKind]kubearchiveapi.KubeArchiveConfigResource, avk *sourcesv1.APIVersionKind) []SinkCloudEventPublisherResult {
	results := []SinkCloudEventPublisherResult{}

	gvr, err := getGVR(scep.mapper, avk)
	if err != nil {
		slog.Error("Unable to get GVR", "error", err)
		return results
	}

	list, err := scep.dynaClient.Resource(gvr).Namespace(namespace).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		slog.Error("Unable to list resources", "error", err)
		return results
	}

	for _, item := range list.Items {
		metadata := item.Object["metadata"].(map[string]interface{})
		name := metadata["name"].(string)

		result := SinkCloudEventPublisherResult{Name: name, Message: "No event sent"}
		if shouldSend(avk, scep.globalResources, localResources) {
			sendResult := scep.Send(ctx, eventType, item.Object)
			if ce.IsACK(sendResult) {
				result.Message = "Event sent successfully"
			} else {
				result.Message = "Event send failed"
			}
			var httpResult *cehttp.Result
			result.StatusCode = 0
			if ce.ResultAs(sendResult, &httpResult) {
				result.StatusCode = httpResult.StatusCode
			}
			slog.Info(result.Message, "apiversion", avk.APIVersion, "kind", avk.Kind, "namespace", namespace, "name", name, "code", result.StatusCode)
		} else {
			slog.Info(result.Message, "apiversion", avk.APIVersion, "kind", avk.Kind, "namespace", namespace, "name", name)
		}
		results = append(results, result)
	}

	return results
}

func (scep *SinkCloudEventPublisher) Send(ctx context.Context, eventType string, resource map[string]interface{}) ce.Result {
	event := ce.NewEvent()
	event.SetSource(scep.source)
	event.SetType(eventType)
	if err := event.SetData(ce.ApplicationJSON, resource); err != nil {
		slog.Error("Error setting cloudevent data", "error", err)
		return ce.NewResult(err.Error())
	}

	event.SetExtension("apiversion", resource["apiVersion"].(string))
	event.SetExtension("kind", resource["kind"].(string))
	metadata := resource["metadata"].(map[string]interface{})
	event.SetExtension("name", metadata["name"])
	event.SetExtension("namespace", metadata["namespace"])

	ectx := ce.ContextWithTarget(ctx, scep.target)

	return scep.httpClient.Send(ectx, event)
}

func (scep *SinkCloudEventPublisher) getKubeArchiveConfigResources(namespace string) (map[sourcesv1.APIVersionKind]kubearchiveapi.KubeArchiveConfigResource, error) {
	var resources = []kubearchiveapi.KubeArchiveConfigResource{}
	if namespace == "" {
		gvr := kubearchiveapi.GroupVersion.WithResource("clusterkubearchiveconfigs")
		obj, err := scep.dynaClient.Resource(gvr).Get(context.Background(), constants.KubeArchiveConfigResourceName, metav1.GetOptions{})
		if err == nil {
			kac, cerr := kubearchiveapi.ConvertUnstructuredToClusterKubeArchiveConfig(obj)
			if cerr != nil {
				return nil, err
			}
			resources = kac.Spec.Resources
		} else if !apierrors.IsNotFound(err) {
			return nil, err
		}
	} else {
		gvr := kubearchiveapi.GroupVersion.WithResource("kubearchiveconfigs")
		obj, err := scep.dynaClient.Resource(gvr).Namespace(namespace).Get(context.Background(), constants.KubeArchiveConfigResourceName, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		kac, cerr := kubearchiveapi.ConvertUnstructuredToKubeArchiveConfig(obj)
		if cerr != nil {
			return nil, err
		}
		resources = kac.Spec.Resources
	}

	var resourceMap = make(map[sourcesv1.APIVersionKind]kubearchiveapi.KubeArchiveConfigResource)
	for _, resource := range resources {
		avk := sourcesv1.APIVersionKind{APIVersion: resource.Selector.APIVersion, Kind: resource.Selector.Kind}
		resourceMap[avk] = resource
	}
	return resourceMap, nil
}

func mergeResources(globalRes map[sourcesv1.APIVersionKind]kubearchiveapi.KubeArchiveConfigResource, localRes map[sourcesv1.APIVersionKind]kubearchiveapi.KubeArchiveConfigResource) map[sourcesv1.APIVersionKind]struct{} {
	var resourceMap = make(map[sourcesv1.APIVersionKind]struct{})
	for avk := range globalRes {
		resourceMap[avk] = struct{}{}
	}
	for avk := range localRes {
		resourceMap[avk] = struct{}{}
	}
	return resourceMap
}

func getGVR(mapper meta.RESTMapper, avk *sourcesv1.APIVersionKind) (schema.GroupVersionResource, error) {
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

func shouldSend(avk *sourcesv1.APIVersionKind, globalResources map[sourcesv1.APIVersionKind]kubearchiveapi.KubeArchiveConfigResource, localResources map[sourcesv1.APIVersionKind]kubearchiveapi.KubeArchiveConfigResource) bool {
	resource, ok := globalResources[*avk]
	if ok && (resource.ArchiveWhen != "" || resource.DeleteWhen != "") {
		return true
	}

	resource, ok = localResources[*avk]
	if ok && (resource.ArchiveWhen != "" || resource.DeleteWhen != "") {
		return true
	}

	return false
}

// getSinkServiceUrl constructs the URL for the local sink service
func getSinkServiceUrl() string {
	// Construct the service URL: http://<service-name>.<namespace>.svc.cluster.local:<port>
	// The sink service runs on port 80 by default
	serviceUrl := fmt.Sprintf("http://%s.%s.svc.cluster.local:80", constants.KubeArchiveSinkName, constants.KubeArchiveNamespace)
	slog.Info("Using sink service URL", "url", serviceUrl)
	return serviceUrl
}
