// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"log/slog"
	"sort"
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

type Keeper struct {
	when   *filters.KeepLastWhenRule
	bucket []*unstructured.Unstructured // Array of resources sorted by "sort" field, one greater than count
}

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

func (vcep *VacuumCloudEventPublisher) SendByNamespace(ctx context.Context, eventTypePrefix string, namespace string) {
	for kindAPIVersion := range vcep.filters {
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
	key := avk.Kind + "-" + avk.APIVersion
	resourceFilters, hasFilters := vcep.filters[key]
	if !hasFilters {
		return
	}

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

	globalCel, globalExists := resourceFilters[constants.SinkFilterGlobalNamespace]
	namespaceCel, namespaceExists := resourceFilters[namespace]

	keepers := vcep.createKeepers(globalCel, namespaceCel)
	all := make(map[string]int)

	for _, item := range list.Items {
		metadata := item.Object["metadata"].(map[string]interface{})
		name := metadata["name"].(string)

		if (globalExists && kcel.ExecuteBooleanCEL(context.Background(), globalCel.DeleteWhen, &item)) ||
			(namespaceExists && kcel.ExecuteBooleanCEL(context.Background(), namespaceCel.DeleteWhen, &item)) {
			vcep.sendCloudEvent(ctx, eventTypePrefix+".delete-when", avk, namespace, name, &item)
			continue
		}
		if len(keepers) > 0 {
			slog.Info("Processing resource in ProcessKeepLastWhen", "resourceName", item.GetName())
			resourceUID := string(item.GetUID())
			match := false
			for _, keeper := range keepers {
				if kcel.ExecuteBooleanCEL(ctx, keeper.when.When, &item) {
					match = true
					keeper.bucket[len(keeper.bucket)-1] = &item
					vcep.sortResources(keeper.when.Sort, keeper.bucket)

					if keeper.bucket[len(keeper.bucket)-1] != &item {
						all[resourceUID]++

						drop := keeper.bucket[keeper.when.Count]
						if drop != nil {
							dropUID := string(drop.GetUID())
							all[dropUID]--
							if all[dropUID] == 0 {
								dropNamespace := drop.GetNamespace()
								dropName := drop.GetName()
								vcep.sendCloudEvent(ctx, eventTypePrefix+".keep-last-when-delete", avk, dropNamespace, dropName, drop)
								delete(all, dropUID)
							}
						}
					}
					keeper.bucket[len(keeper.bucket)-1] = nil
				}
			}

			if match {
				if _, exists := all[resourceUID]; !exists {
					vcep.sendCloudEvent(ctx, eventTypePrefix+".keep-last-when-delete", avk, namespace, name, &item)
				}
				continue
			}
		}
		if (globalExists && kcel.ExecuteBooleanCEL(context.Background(), globalCel.ArchiveWhen, &item)) ||
			(namespaceExists && kcel.ExecuteBooleanCEL(context.Background(), namespaceCel.ArchiveWhen, &item)) {
			vcep.sendCloudEvent(ctx, eventTypePrefix+".archive-when", avk, namespace, name, &item)
			continue
		}
		slog.Info("No event sent", "apiversion", avk.APIVersion, "kind", avk.Kind, "namespace", namespace, "name", name)
	}

	sort.Slice(keepers, func(i, j int) bool {
		return keepers[i].when.WhenText < keepers[j].when.WhenText
	})

	for _, keeper := range keepers {
		var keptResources []string
		for _, item := range keeper.bucket {
			if item != nil {
				keptResources = append(keptResources, item.GetName())
			}
		}

		slog.Info("Resources kept by keeper",
			"apiversion", avk.APIVersion,
			"kind", avk.Kind,
			"namespace", namespace,
			"when", keeper.when.WhenText,
			"count", keeper.when.Count,
			"resources-kept", strings.Join(keptResources, ","))
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

func (vcep *VacuumCloudEventPublisher) uniquifyRules(ruleMap map[string]*filters.KeepLastWhenRule, rules []filters.KeepLastWhenRule) {
	if len(rules) == 0 {
		return
	}

	for i := range rules {
		rule := &rules[i]
		if existing, exists := ruleMap[rule.WhenText]; exists {
			// Keep the rule with lower count
			if rule.Count >= existing.Count {
				continue
			}
		}
		ruleMap[rule.WhenText] = rule
	}
}

func (vcep *VacuumCloudEventPublisher) createKeepers(globalCel filters.CelExpressions, namespaceCel filters.CelExpressions) []*Keeper {
	ruleMap := make(map[string]*filters.KeepLastWhenRule)

	vcep.uniquifyRules(ruleMap, globalCel.KeepLastWhen)
	vcep.uniquifyRules(ruleMap, namespaceCel.KeepLastWhen)

	keepers := make([]*Keeper, 0, len(ruleMap))
	for _, rule := range ruleMap {
		keeper := &Keeper{
			when:   rule,
			bucket: make([]*unstructured.Unstructured, rule.Count+1),
		}
		keepers = append(keepers, keeper)
	}

	return keepers
}

func (vcep *VacuumCloudEventPublisher) sendCloudEvent(ctx context.Context, eventType string, avk *kubearchiveapi.APIVersionKind, namespace string, name string, item *unstructured.Unstructured) {
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
}

func (vcep *VacuumCloudEventPublisher) sortResources(sortField string, bucket []*unstructured.Unstructured) {
	for i := 0; i < len(bucket)-1; i++ {
		for j := i + 1; j < len(bucket); j++ {
			if vcep.compareResources(sortField, bucket[i], bucket[j]) {
				bucket[i], bucket[j] = bucket[j], bucket[i]
			}
		}
	}
}

func (vcep *VacuumCloudEventPublisher) compareResources(sortField string, i, j *unstructured.Unstructured) bool {
	// Handle nil values - nil resources are considered "oldest" and should sort to the end
	if i == nil && j == nil {
		return false
	}
	if i == nil {
		return true
	}
	if j == nil {
		return false
	}

	if sortField == "" || sortField == "metadata.creationTimestamp" {
		iTime := i.GetCreationTimestamp()
		jTime := j.GetCreationTimestamp()
		return iTime.Before(&jTime)
	}

	// Handle other sort fields by extracting the value using field path.
	iValue, iFound, _ := unstructured.NestedString(i.Object, strings.Split(sortField, ".")...)
	jValue, jFound, _ := unstructured.NestedString(j.Object, strings.Split(sortField, ".")...)

	if !iFound || !jFound {
		// Fall back to creation timestamp if either field is not found.
		iTime := i.GetCreationTimestamp()
		jTime := j.GetCreationTimestamp()
		return iTime.Before(&jTime)
	}

	return iValue < jValue
}
