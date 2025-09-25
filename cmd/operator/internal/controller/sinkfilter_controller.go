// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"fmt"
	"sync"

	ce "github.com/cloudevents/sdk-go/v2"
	"k8s.io/apimachinery/pkg/api/errors"
	meta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/tools/cache"
	sourcesv1 "knative.dev/eventing/pkg/apis/sources/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	kubearchivev1 "github.com/kubearchive/kubearchive/cmd/operator/api/v1"
	"github.com/kubearchive/kubearchive/pkg/cloudevents"
	"github.com/kubearchive/kubearchive/pkg/constants"
	"github.com/kubearchive/kubearchive/pkg/k8sclient"
)

type InformerInfo struct {
	Factory dynamicinformer.DynamicSharedInformerFactory
	StopCh  chan struct{}
}

type SinkFilterReconciler struct {
	Client              client.Client
	Scheme              *runtime.Scheme
	Mapper              meta.RESTMapper
	dynamicClient       dynamic.Interface
	cloudEventPublisher *cloudevents.SinkCloudEventPublisher

	// Mutex to protect informer operations
	mu sync.RWMutex
	// Map of GVK string to informer info (factory + stop channel)
	informers map[string]*InformerInfo
}

//+kubebuilder:rbac:groups=kubearchive.org,resources=sinkfilters,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=kubearchive.org,resources=sinkfilters/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=kubearchive.org,resources=sinkfilters/finalizers,verbs=update
//+kubebuilder:rbac:groups="*",resources="*",verbs=get;list;watch
//+kubebuilder:rbac:groups=eventing.knative.dev,resources=brokers,verbs=get;list;watch
//+kubebuilder:rbac:groups=kubearchive.org,resources=kubearchiveconfigs;clusterkubearchiveconfigs,verbs=get;list;watch

func (r *SinkFilterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	log.Info("Reconciling SinkFilter", "name", req.Name, "namespace", req.Namespace)

	// Fetch the SinkFilter instance
	sinkFilter := &kubearchivev1.SinkFilter{}
	err := r.Client.Get(ctx, req.NamespacedName, sinkFilter)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Info("SinkFilter resource not found. Ignoring since object must be deleted")
			// Clear all informers when the resource is deleted by calling recreateInformers with empty maps
			if err = r.recreateInformers(ctx, map[string]sourcesv1.APIVersionKindSelector{}, map[string][]kubearchivev1.KubeArchiveConfigResource{}); err != nil {
				log.Error(err, "Failed to clear informers on delete")
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get SinkFilter")
		return ctrl.Result{}, err
	}

	// Extract unique kinds from all namespaces
	uniqueKinds := r.extractUniqueKinds(sinkFilter)

	// Recreate informers based on the new resource configuration
	if err := r.recreateInformers(ctx, uniqueKinds, sinkFilter.Spec.Namespaces); err != nil {
		log.Error(err, "Failed to recreate informers")
		return ctrl.Result{}, err
	}

	log.Info("Successfully reconciled SinkFilter", "uniqueKinds", len(uniqueKinds))
	return ctrl.Result{}, nil
}

func (r *SinkFilterReconciler) extractUniqueKinds(sinkFilter *kubearchivev1.SinkFilter) map[string]sourcesv1.APIVersionKindSelector {
	uniqueKinds := make(map[string]sourcesv1.APIVersionKindSelector)

	for _, resources := range sinkFilter.Spec.Namespaces {
		for _, resource := range resources {
			key := resource.Selector.Kind + "-" + resource.Selector.APIVersion
			uniqueKinds[key] = sourcesv1.APIVersionKindSelector{
				Kind:       resource.Selector.Kind,
				APIVersion: resource.Selector.APIVersion,
			}
		}
	}

	return uniqueKinds
}

func (r *SinkFilterReconciler) recreateInformers(ctx context.Context, uniqueKinds map[string]sourcesv1.APIVersionKindSelector, namespaces map[string][]kubearchivev1.KubeArchiveConfigResource) error {
	log := log.FromContext(ctx)

	r.mu.Lock()
	defer r.mu.Unlock()

	// Find informers that need to be stopped (exist but not in new uniqueKinds)
	toStop := r.findInformersToStop(uniqueKinds)

	// Find informers that need to be created (in new uniqueKinds but don't exist)
	toCreate := r.findInformersToCreate(uniqueKinds)

	// Stop informers that are no longer needed
	for _, key := range toStop {
		if informer, exists := r.informers[key]; exists {
			close(informer.StopCh)
			delete(r.informers, key)
			log.Info("Stopped informer for resource", "key", key)
		}
	}

	// Create new informers for each new unique kind
	for _, key := range toCreate {
		kindSelector := uniqueKinds[key]
		gvr, err := r.getGVRFromKindSelector(kindSelector)
		if err != nil {
			log.Error(err, "Failed to get GVR for kind", "kind", kindSelector.Kind, "apiVersion", kindSelector.APIVersion)
			continue
		}

		if err := r.createInformerForGVR(ctx, key, gvr, namespaces, kindSelector); err != nil {
			log.Error(err, "Failed to create informer", "gvr", gvr.String())
			continue
		}

		log.Info("Created informer for resource", "gvr", gvr.String())
	}

	// Log summary of changes
	log.Info("Informer update complete",
		"stopped", len(toStop),
		"created", len(toCreate),
		"unchanged", len(r.informers)-len(toCreate))

	return nil
}

func (r *SinkFilterReconciler) findInformersToStop(uniqueKinds map[string]sourcesv1.APIVersionKindSelector) []string {
	var toStop []string
	for existingKey := range r.informers {
		if _, stillNeeded := uniqueKinds[existingKey]; !stillNeeded {
			toStop = append(toStop, existingKey)
		}
	}
	return toStop
}

func (r *SinkFilterReconciler) findInformersToCreate(uniqueKinds map[string]sourcesv1.APIVersionKindSelector) []string {
	var toCreate []string
	for newKey := range uniqueKinds {
		if _, exists := r.informers[newKey]; !exists {
			toCreate = append(toCreate, newKey)
		}
	}
	return toCreate
}

func (r *SinkFilterReconciler) getGVRFromKindSelector(kindSelector sourcesv1.APIVersionKindSelector) (schema.GroupVersionResource, error) {
	var gv schema.GroupVersion
	var err error

	if kindSelector.APIVersion == "" {
		return schema.GroupVersionResource{}, fmt.Errorf("APIVersion is required")
	}

	gv, err = schema.ParseGroupVersion(kindSelector.APIVersion)
	if err != nil {
		return schema.GroupVersionResource{}, fmt.Errorf("failed to parse APIVersion %s: %w", kindSelector.APIVersion, err)
	}

	// Use the REST mapper to get the resource name
	gvk := schema.GroupVersionKind{
		Group:   gv.Group,
		Version: gv.Version,
		Kind:    kindSelector.Kind,
	}

	mapping, err := r.Mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return schema.GroupVersionResource{}, fmt.Errorf("failed to get REST mapping for %v: %w", gvk, err)
	}

	return mapping.Resource, nil
}

func (r *SinkFilterReconciler) createInformerForGVR(ctx context.Context, key string, gvr schema.GroupVersionResource, namespaces map[string][]kubearchivev1.KubeArchiveConfigResource, kindSelector sourcesv1.APIVersionKindSelector) error {
	log := log.FromContext(ctx)

	factory := dynamicinformer.NewDynamicSharedInformerFactory(r.dynamicClient, 0)
	informer := factory.ForResource(gvr)
	predicate := r.createResourcePredicate(namespaces, kindSelector)

	_, err := informer.Informer().AddEventHandler(cache.FilteringResourceEventHandler{
		FilterFunc: predicate,
		Handler: cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				log.Info("Resource added", "gvr", gvr.String())
				r.sendCloudEvent(ctx, obj, "add", gvr, kindSelector.Kind)
			},
			UpdateFunc: func(oldObj, newObj interface{}) {
				log.Info("Resource updated", "gvr", gvr.String())
				r.sendCloudEvent(ctx, newObj, "update", gvr, kindSelector.Kind)
			},
			DeleteFunc: func(obj interface{}) {
				log.Info("Resource deleted", "gvr", gvr.String())
				r.sendCloudEvent(ctx, obj, "delete", gvr, kindSelector.Kind)
			},
		},
	})
	if err != nil {
		return fmt.Errorf("failed to add event handler to informer: %w", err)
	}

	stopCh := make(chan struct{})
	go factory.Start(stopCh)

	go func() {
		if !cache.WaitForCacheSync(stopCh, informer.Informer().HasSynced) {
			log.Error(nil, "Failed to sync cache for informer", "gvr", gvr.String())
			return
		}
		log.Info("Cache synced for informer", "gvr", gvr.String())
	}()

	r.informers[key] = &InformerInfo{
		Factory: factory,
		StopCh:  stopCh,
	}

	return nil
}

func (r *SinkFilterReconciler) createResourcePredicate(namespaces map[string][]kubearchivev1.KubeArchiveConfigResource, kindSelector sourcesv1.APIVersionKindSelector) func(obj interface{}) bool {
	return func(obj interface{}) bool {
		unstructuredObj, ok := obj.(*unstructured.Unstructured)
		if !ok {
			return false
		}

		objNamespace := unstructuredObj.GetNamespace()

		if r.isResourceConfiguredInNamespace(namespaces, constants.SinkFilterGlobalNamespace, kindSelector) {
			return true
		}

		if r.isResourceConfiguredInNamespace(namespaces, objNamespace, kindSelector) {
			return true
		}

		return false
	}
}

func (r *SinkFilterReconciler) isResourceConfiguredInNamespace(namespaces map[string][]kubearchivev1.KubeArchiveConfigResource, namespace string, kindSelector sourcesv1.APIVersionKindSelector) bool {
	resources, namespaceExists := namespaces[namespace]
	if !namespaceExists {
		return false
	}

	for _, resource := range resources {
		if resource.Selector.Kind == kindSelector.Kind && resource.Selector.APIVersion == kindSelector.APIVersion {
			return true
		}
	}

	return false
}

func (r *SinkFilterReconciler) sendCloudEvent(ctx context.Context, obj interface{}, eventType string, gvr schema.GroupVersionResource, kind string) {
	log := log.FromContext(ctx)

	if r.cloudEventPublisher == nil {
		log.V(1).Info("CloudEvent publisher not available, skipping event", "eventType", eventType, "gvr", gvr.String())
		return
	}

	var unstructuredObj *unstructured.Unstructured
	switch v := obj.(type) {
	case *unstructured.Unstructured:
		unstructuredObj = v
	default:
		log.Error(nil, "Unable to convert object to unstructured", "eventType", eventType, "gvr", gvr.String(), "objectType", fmt.Sprintf("%T", obj))
		return
	}

	resource := unstructuredObj.Object
	if resource["apiVersion"] == nil {
		if gvr.Group == "" {
			resource["apiVersion"] = gvr.Version
		} else {
			resource["apiVersion"] = gvr.Group + "/" + gvr.Version
		}
	}

	if resource["kind"] == nil && kind != "" {
		resource["kind"] = kind
	}

	result := r.cloudEventPublisher.Send(ctx, "org.kubearchive.sinkfilters.resource."+eventType, resource)
	if !ce.IsACK(result) {
		if ce.IsNACK(result) {
			log.Error(nil, "Cloud event was not acknowledged", "eventType", eventType, "gvr", gvr.String(), "kind", kind, "result", result)
		} else {
			log.Error(nil, "Cloud event send failed", "eventType", eventType, "gvr", gvr.String(), "kind", kind, "result", result)
		}
		return
	}

	if metadata, ok := resource["metadata"].(map[string]interface{}); ok {
		name := "unknown"
		namespace := "unknown"
		if n, ok := metadata["name"].(string); ok {
			name = n
		}
		if ns, ok := metadata["namespace"].(string); ok {
			namespace = ns
		}
		log.Info("Cloud event sent successfully", "eventType", eventType, "gvr", gvr.String(), "kind", kind, "name", name, "namespace", namespace)
	} else {
		log.Info("Cloud event sent successfully", "eventType", eventType, "gvr", gvr.String(), "kind", kind)
	}
}

func (r *SinkFilterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	var err error
	r.dynamicClient, err = k8sclient.NewInstrumentedDynamicClient()
	if err != nil {
		return fmt.Errorf("failed to create dynamic client: %w", err)
	}

	r.cloudEventPublisher, err = cloudevents.NewSinkCloudEventPublisher(
		"kubearchive.org/sinkfilter-controller",
	)
	if err != nil {
		return fmt.Errorf("failed to create cloud event publisher: %w", err)
	}

	r.informers = make(map[string]*InformerInfo)

	return ctrl.NewControllerManagedBy(mgr).
		For(&kubearchivev1.SinkFilter{}).
		Complete(r)
}
