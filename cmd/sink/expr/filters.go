// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package expr

import (
	"context"
	"errors"
	"fmt"
	"log"
	"maps"
	"os"
	"strings"
	"sync"

	"github.com/fsnotify/fsnotify"
	"github.com/google/cel-go/cel"
	"github.com/kubearchive/kubearchive/cmd/sink/k8s"
	"github.com/kubearchive/kubearchive/pkg/files"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	globalKey       = "_global"
	mountPathEnvVar = "MOUNT_PATH"
)

var ErrNoGlobal = errors.New("no global expressions exist")

type Filters struct {
	*sync.RWMutex
	archive map[NamespaceGroupVersionKind]cel.Program
	delete  map[NamespaceGroupVersionKind]cel.Program
	path    string
}

// EmptyFilters returns a Filters struct with an empty archive and delete slice.
func EmptyFilters() *Filters {
	return &Filters{
		RWMutex: &sync.RWMutex{},
		archive: make(map[NamespaceGroupVersionKind]cel.Program),
		delete:  make(map[NamespaceGroupVersionKind]cel.Program),
		path:    os.Getenv(mountPathEnvVar),
	}
}

// NewFilters creates a Filters struct from the path to a directory where a ConfigMap was mounted. If path is empty
// string or path does not exist, it returns a Filters struct with empty archive and delete slices. It will attempt to
// create all the cel programs that it can from the ConfigMap. Any errors that are encountered are wrapped together and
// returned. Even if this function returns an error, the Filters struct returned can still be used and will not be nil.
func NewFilters() (*Filters, error) {
	var errList []error
	filters := EmptyFilters()
	exists, err := files.PathExists(filters.path)
	if err != nil {
		return filters, fmt.Errorf("cannot determine if ConfigMap is mounted at path %s: %s", filters.path, err)
	}
	if filters.path == "" || !exists {
		return filters, fmt.Errorf("cannot create Filters. ConfigMap is not mounted at path %s", filters.path)
	}
	filterFiles, err := files.DirectoryFiles(filters.path)
	if err != nil {
		return filters, fmt.Errorf(
			"cannot create Filters. Could not read files created from ConfigMap mounted at path %s: %s",
			filters.path,
			err,
		)
	}
	globalArchive, globalDelete, err := getGlobalCelExprs(filterFiles)
	// We don't need to report an error if no global filters were provided
	if err != nil && !errors.Is(err, ErrNoGlobal) {
		errList = append(errList, err)
	}
	errList = append(errList, filters.changeGlobalFilters(filterFiles, globalArchive, globalDelete))

	return filters, errors.Join(errList...)
}

// getGlobalCelExprs returns a two maps that both have GroupVersionKind strings as keys and cel expression strings as
// values. The first map is archive cel expressions and the second map is delete cel expressions. If no global file
// exists, it returns ErrNoGlobal. Otherwise it will return any error it encounters trying to read the global file or
// trying to yaml.Unmarshal the global file into a []KubeArchiveConfigResources.
func getGlobalCelExprs(filterFiles map[string]string) (map[string]string, map[string]string, error) {
	archiveExprs := make(map[string]string)
	deleteExprs := make(map[string]string)
	globalFile, exists := filterFiles[globalKey]
	if !exists {
		return archiveExprs, deleteExprs, ErrNoGlobal
	}
	resources, err := k8s.KACResourceSliceFromFile(globalFile)
	if err != nil {
		return archiveExprs, deleteExprs, err
	}
	for _, resource := range resources {
		gvk := schema.FromAPIVersionAndKind(resource.Selector.APIVersion, resource.Selector.Kind)
		archiveExprs[gvk.String()] = resource.ArchiveWhen
		deleteExprs[gvk.String()] = resource.DeleteWhen
	}
	return archiveExprs, deleteExprs, nil
}

// changeGlobalFilters must be called when global filters for f have changed. This includes when f is first created.
func (f *Filters) changeGlobalFilters(filterFiles, globalArchive, globalDelete map[string]string) error {
	errList := []error{}
	delete(filterFiles, globalKey)
	archiveMap := make(map[NamespaceGroupVersionKind]cel.Program)
	deleteMap := make(map[NamespaceGroupVersionKind]cel.Program)
	for namespace, filePath := range filterFiles {
		nsArchive, nsDelete, err := f.createFilters(namespace, filePath, globalArchive, globalDelete)
		maps.Copy(archiveMap, nsArchive)
		maps.Copy(deleteMap, nsDelete)
		if err != nil {
			errList = append(errList, err)
		}
	}
	err := errors.Join(errList...)
	f.Lock()
	defer f.Unlock()
	f.archive = archiveMap
	f.delete = deleteMap
	return err
}

// createNamespaceFilters creates archive and delete filters for namespace from the KubeArchiveConfigResource stored in file. It
// tries to create all the filters it can even if it encounters errors. Any errors encountered are wrapped together and
// returned. createNamespaceFilters should only be called when no filters have been created for namespace. That case is
// handled by updateNamespaceFilters.
func (f *Filters) createNamespaceFilters(namespace, file string, globalArchive, globalDelete map[string]string) error {
	namespaceArchive, namespaceDelete, err := f.createFilters(namespace, file, globalArchive, globalDelete)
	f.Lock()
	defer f.Unlock()
	f.insertFilters(namespaceArchive, namespaceDelete)
	return err
}

// createFilters returns two maps representing all of the filters for namespace from created by compiling cel
// expressions from file with matching cel expressions based on GroupVersionKind from globalArchive and globalDelete.
// The first returned map represents the filters for archiving and the second returned map represents the filters for
// deleting. createFilters wraps all errors it encounters while creating filters and returns the wrapped error. Even if
// the returned error is not nil, the maps returned can still be used.
func (f *Filters) createFilters(
	namespace string,
	file string,
	globalArchive, globalDelete map[string]string,
) (map[NamespaceGroupVersionKind]cel.Program, map[NamespaceGroupVersionKind]cel.Program, error) {
	archiveMap := make(map[NamespaceGroupVersionKind]cel.Program)
	deleteMap := make(map[NamespaceGroupVersionKind]cel.Program)
	resources, err := k8s.KACResourceSliceFromFile(file)
	if err != nil {
		return archiveMap, deleteMap, err
	}
	var errList []error
	for _, resource := range resources {
		gvk := schema.FromAPIVersionAndKind(resource.Selector.APIVersion, resource.Selector.Kind)
		namespaceGvk := NamespaceGVKFromNamespaceAndGvk(namespace, gvk)
		archiveExpr, err := CreateCelExprOr(resource.ArchiveWhen, globalArchive[gvk.String()])
		if err != nil {
			errList = append(errList, err)
		} else {
			archiveMap[namespaceGvk] = *archiveExpr
		}
		deleteExpr, err := CreateCelExprOr(resource.DeleteWhen, globalDelete[gvk.String()])
		if err != nil {
			errList = append(errList, err)
		} else {
			deleteMap[namespaceGvk] = *deleteExpr
		}
	}
	return archiveMap, deleteMap, errors.Join(errList...)
}

// updateNamespaceFilters updates the cel filters for namespace, from file. It differs from createNamespaceFilters by
// deleting all filters for namespace before inserting the new ones.
func (f *Filters) updateNamespaceFilters(namespace, file string, globalArchive, globalDelete map[string]string) error {
	namespaceArchive, namespaceDelete, err := f.createFilters(namespace, file, globalArchive, globalDelete)
	matcher := NamespaceMatcherFromNamespace(namespace)
	f.Lock()
	defer f.Unlock()
	f.deleteFiltersWithMatcher(matcher)
	f.insertFilters(namespaceArchive, namespaceDelete)
	return err
}

// insertFilters copies filters from insertArchive into archive and copies insertDelete into delete. insertFilters
// does not call Lock() or Unlock() so this must be done by the method that calls it.
func (f *Filters) insertFilters(insertArchive, insertDelete map[NamespaceGroupVersionKind]cel.Program) {
	maps.Copy(f.archive, insertArchive)
	maps.Copy(f.delete, insertDelete)
}

// deleteNamespaceFilters deletes all filters for namespace.
func (f *Filters) deleteNamespaceFilters(namespace string) {
	matcher := NamespaceMatcherFromNamespace(namespace)
	f.Lock()
	defer f.Unlock()
	f.deleteFiltersWithMatcher(matcher)
}

// deleteFiltersWithMatcher uses matcher to delete all filters in archive and delete where matcher returns true. It does
// not call Lock() or Unlock() so this must be done by the method that calls it.
func (f *Filters) deleteFiltersWithMatcher(matcher NamespaceMatcher) {
	maps.DeleteFunc(f.archive, matcher)
	maps.DeleteFunc(f.delete, matcher)
}

// Update updates the archive and delete filters when a ConfigMap file changes.
func (f *Filters) Update(watcher *fsnotify.Watcher) {
	logger := log.New(os.Stderr, "", log.LstdFlags|log.Lmicroseconds|log.LUTC)
	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok { // watcher.Close() was called. We will not receive events anymore.
				return
			}
			f.handleFsEvent(logger, event)
		case err, ok := <-watcher.Errors:
			if !ok { // watcher.Close() was called. We will not receive errors anymore.
				return
			}
			logger.Println("Encountered an error while watching for changes to filters:", err)
		}
	}
}

// handleFsEvent handles the logic for updating filters when an fsnotify.Event is received.
func (f *Filters) handleFsEvent(logger *log.Logger, event fsnotify.Event) {
	switch event.Op {
	case fsnotify.Create, fsnotify.Write:
		if files.IsDirOrDne(event.Name) {
			break
		}
		logger.Printf("File %s changed. Updating filters accordingly\n", event.Name)
		fileName, dir := files.FileNameAndDirFromPath(event.Name)
		filePaths, err := files.DirectoryFiles(dir)
		if err != nil {
			logger.Printf("Could not get all files in directory %s: %s\n", dir, err)
			break
		}
		globalArchive, globalDelete, err := getGlobalCelExprs(filePaths)
		if err != nil && !errors.Is(err, ErrNoGlobal) {
			logger.Println("Could not read global filters:", err)
			break
		}
		if fileName == globalKey {
			logger.Println("Creating global filters")
			err = f.changeGlobalFilters(filePaths, globalArchive, globalDelete)
			if err != nil {
				logger.Println("Problem creating global filters:", err)
			}
			logger.Println("Successfully created global filters")
			break
		}
		logger.Println("Creating filters for namespace:", fileName)
		if event.Has(fsnotify.Write) {
			err = f.updateNamespaceFilters(fileName, event.Name, globalArchive, globalDelete)
		} else {
			err = f.createNamespaceFilters(fileName, event.Name, globalArchive, globalDelete)
		}
		if err != nil {
			logger.Printf("Problem creating filters for namespace %s: %s\n", fileName, err)
		}
		logger.Println("Created filters for namespace:", fileName)
	case fsnotify.Remove, fsnotify.Rename:
		// fsnotify.Rename contains the old file name. If it was renamed in the same directory we will receive
		// fsnotify.Create event for the new file name. Therefore we can treat it the same as fsnotify.Remove
		fileName, dir := files.FileNameAndDirFromPath(event.Name)
		if strings.HasPrefix(fileName, "..") {
			break
		}
		logger.Printf("File %s was deleted. Updating filters accordingly\n", event.Name)
		if fileName == globalKey {
			logger.Println("Removing global filters")
			filePaths, err := files.DirectoryFiles(dir)
			if err != nil {
				logger.Printf("Could not get all files in directory %s: %s\n", dir, err)
				break
			}
			globalArchive := make(map[string]string)
			globalDelete := make(map[string]string)
			err = f.changeGlobalFilters(filePaths, globalArchive, globalDelete)
			if err != nil {
				logger.Println("Problem removing global filters:", err)
				break
			}
			logger.Println("Removed global filters")
			break
		}
		logger.Println("Removing filters for namespace:", fileName)
		f.deleteNamespaceFilters(fileName)
		logger.Println("Removed filters for namespace:", fileName)
	default:
		// fsnotify.Chmod is the only case not handled, but we don't care if file permissions change
		logger.Println("Ignoring file system event:", event.String())
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
	return f.mustDelete(ctx, obj) || (exists && executeCel(ctx, program, obj))
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
	return exists && executeCel(ctx, program, obj)
}

// Path returns f.path, which is the directory where the ConfigMap is mounted.
func (f *Filters) Path() string {
	return f.path
}
