// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package filters

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"sync"

	"github.com/google/cel-go/cel"
	kubearchiveapi "github.com/kubearchive/kubearchive/cmd/operator/api/v1alpha1"
	ocel "github.com/kubearchive/kubearchive/pkg/cel"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	toolsWatch "k8s.io/client-go/tools/watch"
)

const (
	globalKey     = "kubearchive"
	k9eNamespace  = "kubearchive"
	filtersCmName = "sink-filters"
)

var ErrNoGlobal = errors.New("no global expressions exist")

type Filters struct {
	*sync.RWMutex
	clientset       kubernetes.Interface
	archive         map[NamespaceGroupVersionKind]cel.Program
	delete          map[NamespaceGroupVersionKind]cel.Program
	archiveOnDelete map[NamespaceGroupVersionKind]cel.Program
}

// NewFilters creates a Filters struct with empty archive, delete, and archiveOnDelete slices.
func NewFilters(clientset kubernetes.Interface) *Filters {
	return &Filters{
		RWMutex:         &sync.RWMutex{},
		clientset:       clientset,
		archive:         make(map[NamespaceGroupVersionKind]cel.Program),
		delete:          make(map[NamespaceGroupVersionKind]cel.Program),
		archiveOnDelete: make(map[NamespaceGroupVersionKind]cel.Program),
	}

}

// getGlobalCelExprs returns three maps that have GroupVersionKind strings as keys and cel expression strings as values.
// The first map is archive cel expressions, the second map is delete cel expressions, and the third map is archive on
// delete cel expression. If no global key exists, it returns ErrNoGlobal. Otherwise it will return any error it
// encounters to yaml.Unmarshal the []KubeArchiveConfigResources for the global key.
func getGlobalCelExprs(stringResources map[string]string) (map[string]string, map[string]string, map[string]string, error) {
	archiveExprs := make(map[string]string)
	deleteExprs := make(map[string]string)
	archiveOnDelete := make(map[string]string)
	kacResources, exists := stringResources[globalKey]
	if !exists {
		return archiveExprs, deleteExprs, archiveOnDelete, ErrNoGlobal
	}
	resources, err := kubearchiveapi.LoadFromString(kacResources)
	if err != nil {
		return archiveExprs, deleteExprs, archiveOnDelete, err
	}
	for _, resource := range resources {
		gvk := schema.FromAPIVersionAndKind(resource.Selector.APIVersion, resource.Selector.Kind)
		archiveExprs[gvk.String()] = resource.ArchiveWhen
		deleteExprs[gvk.String()] = resource.DeleteWhen
		archiveOnDelete[gvk.String()] = resource.ArchiveOnDelete
	}
	return archiveExprs, deleteExprs, archiveOnDelete, nil
}

// changeGlobalFilters must be called when global filters for f have changed. This includes when f is first created.
func (f *Filters) changeGlobalFilters(stringResources map[string]string) error {
	errList := []error{}
	globalArchive, globalDelete, globalArchiveOnDelete, err := getGlobalCelExprs(stringResources)
	if err != nil && !errors.Is(err, ErrNoGlobal) {
		errList = append(errList, err)
	}
	delete(stringResources, globalKey)
	archiveMap := make(map[NamespaceGroupVersionKind]cel.Program)
	deleteMap := make(map[NamespaceGroupVersionKind]cel.Program)
	archiveOnDeleteMap := make(map[NamespaceGroupVersionKind]cel.Program)
	for namespace, kacResources := range stringResources {
		nsArchive, nsDelete, nsArchiveOnDelete, err := f.createFilters(namespace, kacResources, globalArchive, globalDelete, globalArchiveOnDelete)
		maps.Copy(archiveMap, nsArchive)
		maps.Copy(deleteMap, nsDelete)
		maps.Copy(archiveOnDeleteMap, nsArchiveOnDelete)
		if err != nil {
			errList = append(errList, err)
		}
	}
	err = errors.Join(errList...)
	f.Lock()
	defer f.Unlock()
	f.archive = archiveMap
	f.delete = deleteMap
	f.archiveOnDelete = archiveOnDeleteMap
	return err
}

// createFilters returns three maps representing all of the filters for namespace created by compiling cel expressions
// from file with matching cel expressions based on GroupVersionKind from globalArchive, globalDelete, and
// globalArchiveOnDelete. The first returned map represents the filters for archiving, the second returned map
// represents the filters for deleting, and the third map represents the filters for archiving on delete. createFilters
// wraps all errors it encounters while creating filters and returns the wrapped error. Even if the returned error is
// not nil, the maps returned can still be used.
func (f *Filters) createFilters(
	namespace, kacResources string,
	globalArchive, globalDelete, globalArchiveOnDelete map[string]string,
) (
	map[NamespaceGroupVersionKind]cel.Program,
	map[NamespaceGroupVersionKind]cel.Program,
	map[NamespaceGroupVersionKind]cel.Program,
	error,
) {
	archiveMap := make(map[NamespaceGroupVersionKind]cel.Program)
	deleteMap := make(map[NamespaceGroupVersionKind]cel.Program)
	archiveOnDeleteMap := make(map[NamespaceGroupVersionKind]cel.Program)
	resources, err := kubearchiveapi.LoadFromString(kacResources)
	if err != nil {
		return archiveMap, deleteMap, archiveOnDeleteMap, err
	}
	var errList []error
	for _, resource := range resources {
		gvk := schema.FromAPIVersionAndKind(resource.Selector.APIVersion, resource.Selector.Kind)
		namespaceGvk := NamespaceGVKFromNamespaceAndGvk(namespace, gvk)
		if resource.ArchiveWhen != "" {
			archiveExpr, err := ocel.CreateCelExprOr(resource.ArchiveWhen, globalArchive[gvk.String()])
			if err != nil {
				errList = append(errList, err)
			} else {
				archiveMap[namespaceGvk] = *archiveExpr
			}
		}
		if resource.DeleteWhen != "" {
			deleteExpr, err := ocel.CreateCelExprOr(resource.DeleteWhen, globalDelete[gvk.String()])
			if err != nil {
				errList = append(errList, err)
			} else {
				deleteMap[namespaceGvk] = *deleteExpr
			}
		}
		if resource.ArchiveOnDelete != "" {
			archiveOnDeleteExpr, err := ocel.CreateCelExprOr(resource.ArchiveOnDelete, globalArchiveOnDelete[gvk.String()])
			if err != nil {
				errList = append(errList, err)
			} else {
				archiveOnDeleteMap[namespaceGvk] = *archiveOnDeleteExpr
			}
		}
	}
	return archiveMap, deleteMap, archiveOnDeleteMap, errors.Join(errList...)
}

type UpdateStopper func()

// noopUpdateStopper implements UpdateStopper so that filters.Update can return an UpdateStopper if it fails to create a
// watcher for the ConfigMap
func noopUpdateStopper() {}

// Update updates the archive, delete, and archiveOnDelete filters when the ConfigMap changes.
func (f *Filters) Update() (UpdateStopper, error) {
	watcher := func(options metav1.ListOptions) (watch.Interface, error) {
		return f.clientset.CoreV1().ConfigMaps(k9eNamespace).Watch(
			context.Background(),
			metav1.SingleObject(metav1.ObjectMeta{Name: filtersCmName, Namespace: k9eNamespace}),
		)
	}
	retryWatcher, err := toolsWatch.NewRetryWatcher("1", &cache.ListWatch{WatchFunc: watcher})
	if err != nil {
		return noopUpdateStopper, fmt.Errorf("Could not create a watcher for the %s ConfigMap: %s", filtersCmName, err)
	}
	go f.handleUpdates(retryWatcher)
	return retryWatcher.Stop, nil
}

// handleUpdates handles the logic for updating filters when a watch.Event is received from the watcher.
func (f *Filters) handleUpdates(watcher watch.Interface) {
	for event := range watcher.ResultChan() {
		switch event.Type {
		case watch.Added, watch.Modified:
			slog.Info("Received watch event of type. Updating filters", "event type", string(event.Type))
			configMap, ok := event.Object.(*corev1.ConfigMap)
			if !ok {
				slog.Error("Could not convert object from the event to a ConfigMap")
				continue
			}
			err := f.changeGlobalFilters(configMap.Data)
			if err != nil {
				slog.Error("Error encountered while updating filters", "error", err)
			}
		case watch.Deleted:
			slog.Info("Received watch event of type delete. Updating filters")
			err := f.changeGlobalFilters(make(map[string]string))
			if err != nil {
				slog.Error("Error encountered while updating filters", "error", err)
			}
		default:
			slog.Info("Ignoring watch event of type", "event type", string(event.Type))
		}
	}
}

// MustArchive returns whether obj needs to be archived. Obj needs to be archived if any of the cel programs in archive
// return true or if obj needs to be deleted. If obj is nil, it returns false.
func (f *Filters) MustArchive(ctx context.Context, obj *unstructured.Unstructured) bool {
	if obj == nil {
		return false
	}
	f.RLock()
	defer f.RUnlock()
	ngvk := NamespaceGVKFromObject(obj)
	program, exists := f.archive[ngvk]
	return f.mustDelete(ctx, obj) || (exists && ocel.ExecuteBooleanCEL(ctx, program, obj))
}

// MustDelete returns whether obj needs to be deleted. Obj needs to be deleted if the cel program in delete that matches
// the NamespaceGroupVersionKind of obj returns true. If obj is nil, it returns false.
func (f *Filters) MustDelete(ctx context.Context, obj *unstructured.Unstructured) bool {
	if obj == nil {
		return false
	}
	f.RLock()
	defer f.RUnlock()
	return f.mustDelete(ctx, obj)
}

// mustDelete is called by MustDelete and MustArchive. This method allows MustArchive to call MustDelete without
// creating a deadlock by recursively calling RLock() when a call to Lock() is blocked. Methods that call this function
// must call f.RLock() and f.RUnlock() themselves.
func (f *Filters) mustDelete(ctx context.Context, obj *unstructured.Unstructured) bool {
	if obj == nil {
		return false
	}
	ngvk := NamespaceGVKFromObject(obj)
	program, exists := f.delete[ngvk]
	return exists && ocel.ExecuteBooleanCEL(ctx, program, obj)
}

// MustArchiveOnDelete returns whether obj needs to be archived if it was already deleted. Obj need to be archived if
// any of the cel programs in archiveOnDelete return true. If obj is nil, it returns false.
func (f *Filters) MustArchiveOnDelete(ctx context.Context, obj *unstructured.Unstructured) bool {
	if obj == nil {
		return false
	}
	f.RLock()
	defer f.RUnlock()
	ngvk := NamespaceGVKFromObject(obj)
	program, exists := f.archiveOnDelete[ngvk]
	return exists && ocel.ExecuteBooleanCEL(ctx, program, obj)
}
