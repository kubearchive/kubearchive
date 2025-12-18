// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"log/slog"
	"slices"
	"sort"
	"strings"

	ce "github.com/cloudevents/sdk-go/v2"
	cehttp "github.com/cloudevents/sdk-go/v2/protocol/http"
	kubearchiveapi "github.com/kubearchive/kubearchive/cmd/operator/api/v1"
	kcel "github.com/kubearchive/kubearchive/pkg/cel"
	publisher "github.com/kubearchive/kubearchive/pkg/cloudevents"
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
	publisher        *publisher.SinkCloudEventPublisher
	dynaClient       *dynamic.DynamicClient
	mapper           meta.RESTMapper
	clusterFilters   map[string]filters.CelExpressions
	namespaceFilters map[string]map[string]filters.CelExpressions
}

func NewVacuumCloudEventPublisher(source string, clusterFilters map[string]filters.CelExpressions, namespaceFilters map[string]map[string]filters.CelExpressions) (*VacuumCloudEventPublisher, error) {
	vcep := &VacuumCloudEventPublisher{
		clusterFilters:   clusterFilters,
		namespaceFilters: namespaceFilters,
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
	kindAPIVersions := make(map[string]struct{})

	for kindAPIVersion := range vcep.clusterFilters {
		kindAPIVersions[kindAPIVersion] = struct{}{}
	}

	for kindAPIVersion := range vcep.namespaceFilters {
		kindAPIVersions[kindAPIVersion] = struct{}{}
	}

	sortedKeys := make([]string, 0, len(kindAPIVersions))
	for k := range kindAPIVersions {
		sortedKeys = append(sortedKeys, k)
	}
	sort.Strings(sortedKeys)

	for _, kindAPIVersion := range sortedKeys {
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
	key := avk.Key()

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

	clusterCel, clusterExists := vcep.clusterFilters[key]

	var namespaceCel filters.CelExpressions
	namespaceExists := false
	if resourceFilters, hasFilters := vcep.namespaceFilters[key]; hasFilters {
		namespaceCel, namespaceExists = resourceFilters[namespace]
	}

	keepers := vcep.createKeepers(clusterCel, namespaceCel, namespace, key)
	all := make(map[string]int)

	for _, item := range list.Items {
		metadata := item.Object["metadata"].(map[string]interface{})
		name := metadata["name"].(string)

		if len(keepers) > 0 {
			slog.Info("Processing resource in ProcessKeepLastWhen", "name", item.GetName(), "apiversion", avk.APIVersion, "kind", avk.Kind)
			resourceUID := string(item.GetUID())
			match := false
			for _, keeper := range keepers {
				if kcel.ExecuteBooleanCEL(ctx, keeper.when.When, &item) {
					match = true
					keeper.bucket[len(keeper.bucket)-1] = &item
					vcep.sortResources(keeper.when.SortBy, keeper.bucket)

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

		// If keepers didn't match we should enter here
		sentSometing := false
		if (clusterExists && kcel.ExecuteBooleanCEL(context.Background(), clusterCel.ArchiveWhen, &item)) ||
			(namespaceExists && kcel.ExecuteBooleanCEL(context.Background(), namespaceCel.ArchiveWhen, &item)) {
			vcep.sendCloudEvent(ctx, eventTypePrefix+".archive-when", avk, namespace, name, &item)
			sentSometing = true
		}

		if (clusterExists && kcel.ExecuteBooleanCEL(context.Background(), clusterCel.DeleteWhen, &item)) ||
			(namespaceExists && kcel.ExecuteBooleanCEL(context.Background(), namespaceCel.ArchiveOnDelete, &item)) {
			vcep.sendCloudEvent(ctx, eventTypePrefix+"delete-when", avk, namespace, name, &item)
			sentSometing = true
		}

		if !sentSometing {
			slog.Info("No event sent", "apiversion", avk.APIVersion, "kind", avk.Kind, "namespace", namespace, "name", name)
		}
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

func (vcep *VacuumCloudEventPublisher) unifyRules(clusterRules []filters.KeepLastWhenRule, namespaceRules []filters.KeepLastWhenRule, namespace string, kind string) []filters.KeepLastWhenRule {
	unifiedRules := make([]filters.KeepLastWhenRule, 0, len(clusterRules)+len(namespaceRules))
	ruleNames := make(map[string]int)
	celTexts := make(map[string]bool)

	for _, clusterRule := range clusterRules {
		unifiedRules = append(unifiedRules, clusterRule)
		ruleNames[clusterRule.Name] = len(unifiedRules) - 1
		celTexts[clusterRule.WhenText] = true
	}

	for _, nsRule := range namespaceRules {
		if nsRule.Name != "" {
			// Rule has a name, it is an override
			if idx, found := ruleNames[nsRule.Name]; found {
				// If namespace count is lower, update the rule
				if nsRule.Count < unifiedRules[idx].Count {
					oldCount := unifiedRules[idx].Count
					unifiedRules[idx].Count = nsRule.Count
					slog.Info("Override rule count is lower, updating", "namespace", namespace, "kind", kind, "name", nsRule.Name, "newCount", nsRule.Count, "oldCount", oldCount)
				} else {
					slog.Error("Override rule count is not lower than cluster rule, ignoring", "namespace", namespace, "kind", kind, "name", nsRule.Name, "namespaceCount", nsRule.Count, "clusterCount", unifiedRules[idx].Count)
				}
			} else {
				slog.Error("Override rule not found in cluster rules, ignoring", "namespace", namespace, "kind", kind, "name", nsRule.Name)
			}
		} else {
			if celTexts[nsRule.WhenText] {
				slog.Error("Duplicate keepLastWhen rule detected, ignoring namespace rule", "namespace", namespace, "kind", kind, "when", nsRule.WhenText)
			} else {
				unifiedRules = append(unifiedRules, nsRule)
				celTexts[nsRule.WhenText] = true
			}
		}
	}

	return unifiedRules
}

func (vcep *VacuumCloudEventPublisher) createKeepers(clusterCel filters.CelExpressions, namespaceCel filters.CelExpressions, namespace string, kind string) []*Keeper {
	unifiedRules := vcep.unifyRules(clusterCel.KeepLastWhen, namespaceCel.KeepLastWhen, namespace, kind)

	keepers := make([]*Keeper, 0, len(unifiedRules))
	for i := range unifiedRules {
		keeper := &Keeper{
			when:   &unifiedRules[i],
			bucket: make([]*unstructured.Unstructured, unifiedRules[i].Count+1),
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
	if len(bucket) <= 1 {
		return
	}

	newItem := bucket[len(bucket)-1]

	i, _ := slices.BinarySearchFunc(bucket[:len(bucket)-1], newItem, func(elem, target *unstructured.Unstructured) int {
		if elem == nil && target == nil {
			return 0
		} else if elem == nil {
			return 1
		} else if target == nil {
			return -1
		} else if vcep.compareResources(sortField, elem, target) {
			return -1
		} else if vcep.compareResources(sortField, target, elem) {
			return 1
		}
		return 0
	})

	if i < len(bucket)-1 {
		copy(bucket[i+1:], bucket[i:len(bucket)-1])
		bucket[i] = newItem
	}
}

func (vcep *VacuumCloudEventPublisher) compareResources(sortField string, i, j *unstructured.Unstructured) bool {
	// Handle nil values - nil resources should sort to the end
	if i == nil && j == nil {
		return false
	}
	if i == nil {
		return false // i (nil) should come after j (non-nil)
	}
	if j == nil {
		return true // i (non-nil) should come before j (nil)
	}

	if sortField == "" || sortField == "metadata.creationTimestamp" {
		iTime := i.GetCreationTimestamp()
		jTime := j.GetCreationTimestamp()
		return jTime.Before(&iTime)
	}

	// Handle other sort fields by extracting the value using field path.
	iValue, iFound, _ := unstructured.NestedString(i.Object, strings.Split(sortField, ".")...)
	jValue, jFound, _ := unstructured.NestedString(j.Object, strings.Split(sortField, ".")...)

	if !iFound || !jFound {
		slog.Warn("Sort field not found in resource, falling back to creation timestamp", "sortField", sortField, "iName", i.GetName(), "jName", j.GetName())
		iTime := i.GetCreationTimestamp()
		jTime := j.GetCreationTimestamp()
		return jTime.Before(&iTime)
	}

	return iValue > jValue
}
