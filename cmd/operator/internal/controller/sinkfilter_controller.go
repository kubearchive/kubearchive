// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	ce "github.com/cloudevents/sdk-go/v2"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	meta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/log"

	kubearchivev1 "github.com/kubearchive/kubearchive/cmd/operator/api/v1"
	kcel "github.com/kubearchive/kubearchive/pkg/cel"
	"github.com/kubearchive/kubearchive/pkg/cloudevents"
	"github.com/kubearchive/kubearchive/pkg/constants"
	"github.com/kubearchive/kubearchive/pkg/filters"
	"github.com/kubearchive/kubearchive/pkg/k8sclient"
)

type WatchInfo struct {
	GVR             schema.GroupVersionResource
	KindSelector    kubearchivev1.APIVersionKind
	ClusterCel      *filters.CelExpressions
	Namespaces      map[string]filters.CelExpressions
	StopCh          chan struct{}
	WatchInterface  watch.Interface
	ResourceVersion string // Current resource version for efficient reconnections
	Queue           workqueue.TypedRateLimitingInterface[watch.Event]
	WorkerWg        sync.WaitGroup
}

type SinkFilterReconciler struct {
	Client              client.Client
	Scheme              *runtime.Scheme
	Mapper              meta.RESTMapper
	dynamicClient       dynamic.Interface
	cloudEventPublisher *cloudevents.SinkCloudEventPublisher

	// Mutex to protect watch operations
	mu sync.RWMutex
	// Map of GVK string to watch info
	watches map[string]*WatchInfo
}

//+kubebuilder:rbac:groups=kubearchive.org,resources=sinkfilters,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=kubearchive.org,resources=sinkfilters/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=kubearchive.org,resources=sinkfilters/finalizers,verbs=update
//+kubebuilder:rbac:groups=kubearchive.org,resources=kubearchiveconfigs;clusterkubearchiveconfigs,verbs=get;list;watch
//+kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterroles;clusterrolebindings,verbs=get;list;watch;create;update;patch;delete

func (r *SinkFilterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	log.Info("Reconciling SinkFilter", "name", req.Name, "namespace", req.Namespace)

	sinkFilter := &kubearchivev1.SinkFilter{}
	err := r.Client.Get(ctx, req.NamespacedName, sinkFilter)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Info("SinkFilter resource not found. Ignoring since object must be deleted")
			// Clear all watches when the resource is deleted by calling generateWatches with empty maps.
			if err = r.generateWatches(ctx, map[string]filters.CelExpressions{}, map[string]map[string]filters.CelExpressions{}); err != nil {
				log.Error(err, "Failed to clear watches on delete")
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get SinkFilter")
		return ctrl.Result{}, err
	}

	if err := r.reconcileClusterRole(ctx, sinkFilter); err != nil {
		log.Error(err, "Failed to reconcile ClusterRole")
		return ctrl.Result{}, err
	}

	if err := r.reconcileClusterRoleBinding(ctx); err != nil {
		log.Error(err, "Failed to reconcile ClusterRoleBinding")
		return ctrl.Result{}, err
	}

	clusterFilters := filters.ExtractClusterCELExpressionsByKind(sinkFilter, filters.Controller)
	namespacesByKinds := filters.ExtractNamespacesByKind(sinkFilter, filters.Controller)

	if err := r.generateWatches(ctx, clusterFilters, namespacesByKinds); err != nil {
		log.Error(err, "Failed to generate watches")
		return ctrl.Result{}, err
	}

	log.Info("Successfully reconciled SinkFilter", "namespacesByKinds", len(namespacesByKinds))
	return ctrl.Result{}, nil
}

func (r *SinkFilterReconciler) parseKindAndAPIVersionFromKey(key string) (string, string) {
	// Key format is "Kind-APIVersion", so parse it back
	parts := strings.Split(key, "-")
	if len(parts) >= 2 {
		kind := parts[0]
		apiVersion := strings.Join(parts[1:], "-") // In case APIVersion contains dashes.
		return kind, apiVersion
	}
	return "", ""
}

func (r *SinkFilterReconciler) generateWatches(ctx context.Context, clusterFilters map[string]filters.CelExpressions, namespacesByKinds map[string]map[string]filters.CelExpressions) error {
	log := log.FromContext(ctx)

	r.mu.Lock()
	defer r.mu.Unlock()

	allKinds := make(map[string]struct{})
	for key := range clusterFilters {
		allKinds[key] = struct{}{}
	}
	for key := range namespacesByKinds {
		allKinds[key] = struct{}{}
	}

	toStop := r.findWatchesToStop(allKinds)
	toCreate := r.findWatchesToCreate(allKinds)
	toUpdate := r.findWatchesToUpdate(allKinds, toStop)

	for key := range toStop {
		if watchInfo, exists := r.watches[key]; exists {
			close(watchInfo.StopCh)
			if watchInfo.WatchInterface != nil {
				watchInfo.WatchInterface.Stop()
			}
			watchInfo.Queue.ShutDown()
			watchInfo.WorkerWg.Wait()
			delete(r.watches, key)
			log.Info("Stopped watch for resource", "key", key)
		}
	}

	for key := range toUpdate {
		if watchInfo, exists := r.watches[key]; exists {
			if clusterCel, ok := clusterFilters[key]; ok {
				watchInfo.ClusterCel = &clusterCel
			} else {
				watchInfo.ClusterCel = nil
			}
			watchInfo.Namespaces = namespacesByKinds[key]
			log.Info("Updated watch filters", "key", key, "hasCluster", watchInfo.ClusterCel != nil, "namespaceCount", len(watchInfo.Namespaces))
		}
	}

	for key := range toCreate {
		kind, apiVersion := r.parseKindAndAPIVersionFromKey(key)
		gvr, _, _, err := r.getGVRFromKindAndAPIVersion(kind, apiVersion)
		if err != nil {
			log.Error(err, "Failed to get GVR for kind", "kind", kind, "apiVersion", apiVersion)
			continue
		}

		var clusterCel *filters.CelExpressions
		if cel, ok := clusterFilters[key]; ok {
			clusterCel = &cel
		}
		r.createWatchForGVR(ctx, key, gvr, clusterCel, namespacesByKinds[key])
		log.Info("Created watch for resource", "gvr", gvr.String())
	}

	log.Info("Watch update complete",
		"stopped", len(toStop),
		"updated", len(toUpdate),
		"created", len(toCreate))

	return nil
}

func (r *SinkFilterReconciler) findWatchesToStop(allKinds map[string]struct{}) map[string]struct{} {
	toStop := make(map[string]struct{})
	for existingKey := range r.watches {
		if _, stillNeeded := allKinds[existingKey]; !stillNeeded {
			toStop[existingKey] = struct{}{}
		}
	}
	return toStop
}

func (r *SinkFilterReconciler) findWatchesToCreate(allKinds map[string]struct{}) map[string]struct{} {
	toCreate := make(map[string]struct{})
	for newKey := range allKinds {
		if _, exists := r.watches[newKey]; !exists {
			toCreate[newKey] = struct{}{}
		}
	}
	return toCreate
}

func (r *SinkFilterReconciler) findWatchesToUpdate(allKinds map[string]struct{}, toStop map[string]struct{}) map[string]struct{} {
	toUpdate := make(map[string]struct{})

	for existingKey := range r.watches {
		if _, stillNeeded := allKinds[existingKey]; stillNeeded {
			if _, stopping := toStop[existingKey]; !stopping {
				toUpdate[existingKey] = struct{}{}
			}
		}
	}

	return toUpdate
}

func (r *SinkFilterReconciler) getGVRFromKindAndAPIVersion(kind, apiVersion string) (schema.GroupVersionResource, string, string, error) {
	var gv schema.GroupVersion
	var err error

	if apiVersion == "" {
		return schema.GroupVersionResource{}, "", "", fmt.Errorf("APIVersion is required")
	}

	gv, err = schema.ParseGroupVersion(apiVersion)
	if err != nil {
		return schema.GroupVersionResource{}, "", "", fmt.Errorf("failed to parse APIVersion %s: %w", apiVersion, err)
	}

	// Use the REST mapper to get the resource name
	gvk := schema.GroupVersionKind{
		Group:   gv.Group,
		Version: gv.Version,
		Kind:    kind,
	}

	mapping, err := r.Mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return schema.GroupVersionResource{}, "", "", fmt.Errorf("failed to get REST mapping for %v: %w", gvk, err)
	}

	return mapping.Resource, kind, apiVersion, nil
}

func (r *SinkFilterReconciler) createWatchForGVR(ctx context.Context, key string, gvr schema.GroupVersionResource, clusterCel *filters.CelExpressions, namespaces map[string]filters.CelExpressions) {
	log := log.FromContext(ctx)
	stopCh := make(chan struct{})

	kind, apiVersion := r.parseKindAndAPIVersionFromKey(key)
	kindSelector := kubearchivev1.APIVersionKind{
		Kind:       kind,
		APIVersion: apiVersion,
	}

	queue := workqueue.NewTypedRateLimitingQueueWithConfig(
		workqueue.DefaultTypedControllerRateLimiter[watch.Event](),
		workqueue.TypedRateLimitingQueueConfig[watch.Event]{
			Name:            key,
			MetricsProvider: OpenTelemetryWorkqueueMetricsProvider{},
		},
	)

	watchInfo := &WatchInfo{
		GVR:             gvr,
		KindSelector:    kindSelector,
		ClusterCel:      clusterCel,
		Namespaces:      namespaces,
		StopCh:          stopCh,
		ResourceVersion: "",
		Queue:           queue,
	}

	r.watches[key] = watchInfo

	resourceConfig, _ := GetResourceConfig(kindSelector)
	for i := 0; i < resourceConfig.Workers; i++ {
		watchInfo.WorkerWg.Add(1)
		go r.runWorker(ctx, watchInfo, key)
	}
	log.Info("Started workers for resource", "apiVersion", kindSelector.APIVersion, "kind", kindSelector.Kind, "workers", resourceConfig.Workers)

	go r.watchLoop(ctx, watchInfo, key)
}

func (r *SinkFilterReconciler) watchLoop(ctx context.Context, watchInfo *WatchInfo, key string) {
	log := log.Log.WithValues("fromReconcileID", controller.ReconcileIDFromContext(ctx))
	backoff := time.Second
	maxBackoff := 5 * time.Minute

	for {
		select {
		case <-watchInfo.StopCh:
			log.Info("Watch stopped", "key", key)
			return
		default:
		}

		var err error
		watchInfo.WatchInterface, err = r.createWatch(ctx, watchInfo.GVR, watchInfo.ResourceVersion)
		if err != nil {
			log.Error(err, "Failed to create watch, retrying", "key", key, "backoff", backoff)
			select {
			case <-time.After(backoff):
				backoff = time.Duration(float64(backoff) * 1.5)
				if backoff > maxBackoff {
					backoff = maxBackoff
				}
				continue
			case <-watchInfo.StopCh:
				return
			}
		}

		backoff = time.Second // Reset backoff on successful connection

		log.Info("Started watch", "key", key, "gvr", watchInfo.GVR.String(), "resourceVersion", watchInfo.ResourceVersion)

		r.processWatchEvents(ctx, watchInfo.WatchInterface, watchInfo, key)

		watchInfo.WatchInterface.Stop()
		watchInfo.WatchInterface = nil

		log.Info("Watch disconnected, will retry", "key", key)
	}
}

func (r *SinkFilterReconciler) createWatch(ctx context.Context, gvr schema.GroupVersionResource, resourceVersion string) (watch.Interface, error) {
	listOptions := metav1.ListOptions{
		TimeoutSeconds:  randomTimeout(), // Timeout between 5-10 minutes
		ResourceVersion: resourceVersion,
	}

	watchInterface, err := r.dynamicClient.Resource(gvr).Watch(ctx, listOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to create watch for %v: %w", gvr, err)
	}
	return watchInterface, nil
}

func (r *SinkFilterReconciler) processWatchEvents(ctx context.Context, watchInterface watch.Interface, watchInfo *WatchInfo, key string) {
	log := log.Log.WithValues("fromReconcileID", controller.ReconcileIDFromContext(ctx))

	resultChan := watchInterface.ResultChan()

	for {
		select {
		case <-watchInfo.StopCh:
			log.Info("Stopping watch event processing", "key", key)
			return
		case event, ok := <-resultChan:
			if !ok {
				log.Info("Watch channel closed", "key", key)
				return
			}

			// If watch event type is ERROR, exit the loop to recreate the watch.
			if event.Type == watch.Error {
				r.logWatchError(ctx, event, watchInfo, key)
				if r.shouldClearResourceVersion(event) {
					log.Info("Watch received Gone error, clearing resource version for full resync", "key", key)
					watchInfo.ResourceVersion = ""
				}
				return
			}

			if unstructuredObj, ok := event.Object.(*unstructured.Unstructured); ok {
				if resourceVersion := unstructuredObj.GetResourceVersion(); resourceVersion != "" {
					watchInfo.ResourceVersion = resourceVersion
				}
			}

			watchInfo.Queue.Add(event)
		}
	}
}

func (r *SinkFilterReconciler) runWorker(ctx context.Context, watchInfo *WatchInfo, key string) {
	defer watchInfo.WorkerWg.Done()
	log := log.Log.WithValues("fromReconcileID", controller.ReconcileIDFromContext(ctx))
	log.Info("Starting worker")

	for {
		select {
		case <-watchInfo.StopCh:
			log.Info("Stopping worker", "key", key)
			return
		default:
			event, shutdown := watchInfo.Queue.Get()
			if shutdown {
				return
			}

			func() {
				defer watchInfo.Queue.Done(event)

				if err := r.handleWatchEvent(ctx, event, watchInfo); err != nil {
					log.Error(err, "Failed to handle watch event", "key", key)
					watchInfo.Queue.AddRateLimited(event)
					return
				}

				watchInfo.Queue.Forget(event)
			}()
		}
	}
}

func (r *SinkFilterReconciler) logWatchError(ctx context.Context, event watch.Event, watchInfo *WatchInfo, key string) {
	log := log.Log.WithValues("fromReconcileID", controller.ReconcileIDFromContext(ctx))

	var errorMsg string
	var errorCode int32
	var errorReason metav1.StatusReason

	if status, ok := event.Object.(*metav1.Status); ok {
		errorMsg = status.Message
		errorCode = status.Code
		errorReason = status.Reason
	} else {
		errorMsg = fmt.Sprintf("unknown error format: %T", event.Object)
	}

	log.Info("Watch error event received",
		"key", key,
		"errorMessage", errorMsg,
		"errorCode", errorCode,
		"errorReason", errorReason,
		"gvr", watchInfo.GVR.String())
}

func (r *SinkFilterReconciler) shouldClearResourceVersion(event watch.Event) bool {
	var errorCode int32
	var errorReason metav1.StatusReason

	if status, ok := event.Object.(*metav1.Status); ok {
		errorCode = status.Code
		errorReason = status.Reason
	}

	return errorReason == metav1.StatusReasonGone || errorCode == http.StatusGone
}

func (r *SinkFilterReconciler) handleWatchEvent(ctx context.Context, event watch.Event, watchInfo *WatchInfo) error {
	log := log.Log.WithValues("fromReconcileID", controller.ReconcileIDFromContext(ctx))
	unstructuredObj, ok := event.Object.(*unstructured.Unstructured)
	if !ok {
		return fmt.Errorf("unexpected object type: %T", event.Object)
	}

	objNamespace := unstructuredObj.GetNamespace()
	namespaceCel, namespaceExists := watchInfo.Namespaces[objNamespace]
	clusterExists := watchInfo.ClusterCel != nil

	if !clusterExists && !namespaceExists {
		return nil
	}

	switch event.Type {
	case watch.Added, watch.Modified:
		if (clusterExists && kcel.ExecuteBooleanCEL(ctx, watchInfo.ClusterCel.DeleteWhen, unstructuredObj)) ||
			(namespaceExists && kcel.ExecuteBooleanCEL(ctx, namespaceCel.DeleteWhen, unstructuredObj)) {
			return r.sendCloudEvent(ctx, "delete-when", event, watchInfo)
		} else if (clusterExists && kcel.ExecuteBooleanCEL(ctx, watchInfo.ClusterCel.ArchiveWhen, unstructuredObj)) ||
			(namespaceExists && kcel.ExecuteBooleanCEL(ctx, namespaceCel.ArchiveWhen, unstructuredObj)) {
			return r.sendCloudEvent(ctx, "archive-when", event, watchInfo)
		}
		return nil
	case watch.Deleted:
		if (clusterExists && kcel.ExecuteBooleanCEL(ctx, watchInfo.ClusterCel.ArchiveOnDelete, unstructuredObj)) ||
			(namespaceExists && kcel.ExecuteBooleanCEL(ctx, namespaceCel.ArchiveOnDelete, unstructuredObj)) {
			return r.sendCloudEvent(ctx, "archive-on-delete", event, watchInfo)
		}
		return nil
	default:
		log.Error(nil, "Ignoring unknown watch event type", "type", event.Type)
		return nil
	}
}

func (r *SinkFilterReconciler) sendCloudEvent(ctx context.Context, eventType string, event watch.Event, watchInfo *WatchInfo) error {
	log := log.Log.WithValues("fromReconcileID", controller.ReconcileIDFromContext(ctx))

	if r.cloudEventPublisher == nil {
		err := fmt.Errorf("CloudEvent publisher not available")
		log.Error(err, "Skipping event", "eventType", event.Type, "gvr", watchInfo.GVR.String())
		return err
	}

	unstructuredObj := event.Object.(*unstructured.Unstructured)
	resource := unstructuredObj.Object

	if resource["apiVersion"] == nil {
		if watchInfo.GVR.Group == "" {
			resource["apiVersion"] = watchInfo.GVR.Version
		} else {
			resource["apiVersion"] = watchInfo.GVR.Group + "/" + watchInfo.GVR.Version
		}
	}

	if resource["kind"] == nil && watchInfo.KindSelector.Kind != "" {
		resource["kind"] = watchInfo.KindSelector.Kind
	}

	uid := string(unstructuredObj.GetUID())
	namespace := unstructuredObj.GetNamespace()
	name := unstructuredObj.GetName()

	// Extract owner UUID from ownerReferences if present
	var owner string
	ownerRefs := unstructuredObj.GetOwnerReferences()
	if len(ownerRefs) > 0 {
		owner = string(ownerRefs[0].UID)
	}

	fullEventType := "org.kubearchive.sinkfilters.resource." + eventType
	result := r.cloudEventPublisher.Send(ctx, fullEventType, resource)

	if !ce.IsACK(result) {
		var err error
		if ce.IsNACK(result) {
			err = fmt.Errorf("cloud event was not acknowledged: %w", result)
		} else {
			err = fmt.Errorf("cloud event send failed: %w", result)
		}

		log.Error(err, "Failed to send cloud event",
			"uid", uid,
			"namespace", namespace,
			"name", name,
			"owner", owner,
			"eventType", eventType,
			"api_version", watchInfo.KindSelector.APIVersion,
			"kind", watchInfo.KindSelector.Kind)
		return err
	}

	log.V(1).Info("Cloud event sent successfully",
		"uid", uid,
		"namespace", namespace,
		"name", name,
		"owner", owner,
		"eventType", fullEventType,
		"api_version", watchInfo.KindSelector.APIVersion,
		"kind", watchInfo.KindSelector.Kind)
	return nil
}

func (r *SinkFilterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	var err error
	r.dynamicClient, err = k8sclient.NewInstrumentedDynamicClient()
	if err != nil {
		return fmt.Errorf("failed to create dynamic client: %w", err)
	}

	r.cloudEventPublisher, err = cloudevents.NewSinkCloudEventPublisher("kubearchive.org/sinkfilter-controller")
	if err != nil {
		return fmt.Errorf("failed to create cloud event publisher: %w", err)
	}

	r.watches = make(map[string]*WatchInfo)

	return ctrl.NewControllerManagedBy(mgr).
		For(&kubearchivev1.SinkFilter{}).
		Complete(r)
}

func (r *SinkFilterReconciler) extractResources(sinkFilter *kubearchivev1.SinkFilter) []kubearchivev1.APIVersionKind {
	resourcesMap := make(map[kubearchivev1.APIVersionKind]struct{})

	for _, resource := range sinkFilter.Spec.Cluster {
		resourcesMap[resource.Selector] = struct{}{}
	}

	for _, namespaceResources := range sinkFilter.Spec.Namespaces {
		for _, resource := range namespaceResources {
			resourcesMap[resource.Selector] = struct{}{}
		}
	}

	resources := make([]kubearchivev1.APIVersionKind, 0, len(resourcesMap))
	for resource := range resourcesMap {
		resources = append(resources, resource)
	}

	return resources
}

func (r *SinkFilterReconciler) reconcileClusterRole(ctx context.Context, sinkFilter *kubearchivev1.SinkFilter) error {
	log := log.FromContext(ctx)

	resources := r.extractResources(sinkFilter)
	rules := createPolicyRules(ctx, r.Mapper, resources, []string{"get", "list", "watch"})

	desired := desiredClusterRole(constants.KubeArchiveSinkFilterName, rules)

	existing := &rbacv1.ClusterRole{}
	err := r.Client.Get(ctx, types.NamespacedName{Name: constants.KubeArchiveSinkFilterName}, existing)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Info("Creating ClusterRole", "name", constants.KubeArchiveSinkFilterName)
			return r.Client.Create(ctx, desired)
		}
		return fmt.Errorf("failed to get ClusterRole: %w", err)
	}

	if !equalPolicyRules(existing.Rules, rules) {
		log.Info("Updating ClusterRole", "name", constants.KubeArchiveSinkFilterName)
		existing.Rules = rules
		return r.Client.Update(ctx, existing)
	}

	return nil
}

func (r *SinkFilterReconciler) reconcileClusterRoleBinding(ctx context.Context) error {
	log := log.FromContext(ctx)

	subjects := []rbacv1.Subject{
		{
			Kind:      "ServiceAccount",
			Name:      constants.KubeArchiveOperatorName,
			Namespace: constants.KubeArchiveNamespace,
		},
	}

	desired := desiredClusterRoleBinding(constants.KubeArchiveSinkFilterName, "ClusterRole", subjects...)

	existing := &rbacv1.ClusterRoleBinding{}
	err := r.Client.Get(ctx, types.NamespacedName{Name: constants.KubeArchiveSinkFilterName}, existing)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Info("Creating ClusterRoleBinding", "name", constants.KubeArchiveSinkFilterName)
			return r.Client.Create(ctx, desired)
		}
		return fmt.Errorf("failed to get ClusterRoleBinding: %w", err)
	}

	if !slices.Equal(existing.Subjects, desired.Subjects) ||
		existing.RoleRef != desired.RoleRef {
		log.Info("Updating ClusterRoleBinding", "name", constants.KubeArchiveSinkFilterName)
		existing.Subjects = desired.Subjects
		existing.RoleRef = desired.RoleRef
		return r.Client.Update(ctx, existing)
	}

	return nil
}

func randomTimeout() *int64 {
	// Generate a number of timeout seconds between 300 and 600.
	n := big.NewInt(300)
	n, err := rand.Int(rand.Reader, n)
	if err == nil {
		n.Add(n, big.NewInt(300))
	}
	result := n.Int64()
	return &result
}
