// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package filters

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/google/cel-go/cel"
	kubearchiveapi "github.com/kubearchive/kubearchive/cmd/operator/api/v1alpha1"
	ocel "github.com/kubearchive/kubearchive/pkg/cel"
	"github.com/kubearchive/kubearchive/pkg/constants"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/cache"
	toolsWatch "k8s.io/client-go/tools/watch"
)

type Interface interface {
	MustArchive(context.Context, *unstructured.Unstructured) bool
	MustDelete(context.Context, *unstructured.Unstructured) bool
	MustArchiveOnDelete(context.Context, *unstructured.Unstructured) bool
}

type Filters struct {
	*sync.RWMutex
	dynclient       dynamic.Interface
	archive         map[NamespaceGroupVersionKind]cel.Program
	delete          map[NamespaceGroupVersionKind]cel.Program
	archiveOnDelete map[NamespaceGroupVersionKind]cel.Program
}

// NewFilters creates a Filters struct with empty archive, delete, and archiveOnDelete slices.
func NewFilters(dynclient dynamic.Interface) *Filters {
	return &Filters{
		RWMutex:         &sync.RWMutex{},
		dynclient:       dynclient,
		archive:         make(map[NamespaceGroupVersionKind]cel.Program),
		delete:          make(map[NamespaceGroupVersionKind]cel.Program),
		archiveOnDelete: make(map[NamespaceGroupVersionKind]cel.Program),
	}
}

// changeFilters must be called when global filters for f have changed. This includes when f is first created.
func (f *Filters) changeFilters(namespaces map[string][]kubearchiveapi.KubeArchiveConfigResource) error {
	errList := []error{}
	archiveMap := make(map[NamespaceGroupVersionKind]cel.Program)
	deleteMap := make(map[NamespaceGroupVersionKind]cel.Program)
	archiveOnDeleteMap := make(map[NamespaceGroupVersionKind]cel.Program)
	for namespace, kacResources := range namespaces {
		nsErr := f.createFilters(namespace, kacResources, archiveMap, deleteMap, archiveOnDeleteMap)
		if nsErr != nil {
			errList = append(errList, nsErr)
		}
	}
	err := errors.Join(errList...)
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
	namespace string, kacResources []kubearchiveapi.KubeArchiveConfigResource,
	archiveMap, deleteMap, archiveOnDeleteMap map[NamespaceGroupVersionKind]cel.Program,
) error {
	var errList []error
	for _, resource := range kacResources {
		gvk := schema.FromAPIVersionAndKind(resource.Selector.APIVersion, resource.Selector.Kind)
		namespaceGvk := NamespaceGVKFromNamespaceAndGvk(namespace, gvk)

		errList = addLocalFilters(resource.ArchiveWhen, archiveMap, namespaceGvk, errList)
		errList = addLocalFilters(resource.DeleteWhen, deleteMap, namespaceGvk, errList)
		errList = addLocalFilters(resource.ArchiveOnDelete, archiveOnDeleteMap, namespaceGvk, errList)
	}

	return errors.Join(errList...)
}

func addLocalFilters(expression string, localMap map[NamespaceGroupVersionKind]cel.Program, namespaceGvk NamespaceGroupVersionKind, errList []error) []error {
	if expression != "" {
		expressionCEL, err := ocel.CompileCELExpr(expression)
		if err != nil {
			errList = append(errList, err)
		} else {
			localMap[namespaceGvk] = *expressionCEL
		}
	}
	return errList
}

type UpdateStopper func()

// noopUpdateStopper implements UpdateStopper so that filters. Update can return an UpdateStopper if it fails to create a
// watcher for the SinkFilter resource.
func noopUpdateStopper() {}

// Update updates the archive, delete, and archiveOnDelete filters when the SinkFilter resouce changes.
func (f *Filters) Update() (UpdateStopper, error) {
	watcher := func(options metav1.ListOptions) (watch.Interface, error) {
		return f.dynclient.Resource(kubearchiveapi.SinkFilterGVR).Namespace(constants.KubeArchiveNamespace).Watch(
			context.Background(),
			metav1.SingleObject(metav1.ObjectMeta{Name: constants.SinkFilterResourceName, Namespace: constants.KubeArchiveNamespace}),
		)
	}
	retryWatcher, err := toolsWatch.NewRetryWatcherWithContext(context.Background(), "1", &cache.ListWatch{WatchFunc: watcher})
	if err != nil {
		return noopUpdateStopper, fmt.Errorf("could not create a watcher for the %s SinkFilter: %s", constants.SinkFilterResourceName, err)
	}
	go f.handleUpdates(retryWatcher)
	return retryWatcher.Stop, nil
}

// handleUpdates handles the logic for updating filters when a watch.Event is received from the watcher.
func (f *Filters) handleUpdates(watcher watch.Interface) {
	for event := range watcher.ResultChan() {
		switch event.Type {
		case watch.Added, watch.Modified:
			slog.Info("Received watch event, updating filters", "event type", string(event.Type))

			sinkFilter, err := kubearchiveapi.ConvertObjectToSinkFilter(event.Object)
			if err != nil {
				slog.Error("Unable to convert to SinkFilter", "error", err)
				continue
			}

			err = f.changeFilters(sinkFilter.Spec.Namespaces)
			if err != nil {
				slog.Error("Error encountered while updating filters", "error", err)
			}
		case watch.Deleted:
			slog.Info("Received watch event, updating filters", "event type", string(event.Type))
			err := f.changeFilters(make(map[string][]kubearchiveapi.KubeArchiveConfigResource))
			if err != nil {
				slog.Error("Error encountered while updating filters", "error", err)
			}
		default:
			slog.Info("Ignoring watch event", "event type", string(event.Type))
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

	if f.mustDelete(ctx, obj) {
		return true
	}

	ngvk := NamespaceGVKFromObject(obj)
	program, exists := f.archive[ngvk]
	if exists && ocel.ExecuteBooleanCEL(ctx, program, obj) {
		return true
	}

	ngvk = GlobalNGVKFromObject(obj)
	program, exists = f.archive[ngvk]
	return exists && ocel.ExecuteBooleanCEL(ctx, program, obj)
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
	if exists && ocel.ExecuteBooleanCEL(ctx, program, obj) {
		return true
	}

	ngvk = GlobalNGVKFromObject(obj)
	program, exists = f.delete[ngvk]
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
	if exists && ocel.ExecuteBooleanCEL(ctx, program, obj) {
		return true
	}

	ngvk = GlobalNGVKFromObject(obj)
	program, exists = f.archiveOnDelete[ngvk]
	return exists && ocel.ExecuteBooleanCEL(ctx, program, obj)
}
