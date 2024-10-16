package fake

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func CreateTestResources() []*unstructured.Unstructured {
	ret := []*unstructured.Unstructured{}
	crontab := &unstructured.Unstructured{}
	crontab.SetKind("Crontab")
	crontab.SetAPIVersion("stable.example.com/v1")
	crontab.SetName("test")
	crontab.SetNamespace("test")
	ret = append(ret, crontab)
	pod := &unstructured.Unstructured{}
	pod.SetKind("Pod")
	pod.SetAPIVersion("v1")
	pod.SetName("test")
	pod.SetNamespace("test")
	ret = append(ret, pod)
	return ret
}

type Database struct {
	resources []*unstructured.Unstructured
	err       error
}

func NewFakeDatabase(testResources []*unstructured.Unstructured) *Database {
	return &Database{testResources, nil}
}

func NewFakeDatabaseWithError(err error) *Database {
	var resources []*unstructured.Unstructured
	return &Database{resources, err}
}

func (f *Database) Ping(ctx context.Context) error {
	return f.err
}

func (f *Database) TestConnection(env map[string]string) error {
	return f.err
}

func (f *Database) QueryResources(ctx context.Context, kind, version, limit, continueUUID, continueDate string) ([]*unstructured.Unstructured, string, string, error) {
	resources := f.filterResoucesByKindAndApiVersion(kind, version)
	var date string
	var uuid string
	if len(resources) > 0 {
		date = resources[len(resources)-1].GetCreationTimestamp().Format(time.RFC3339)
		uuid = string(resources[len(resources)-1].GetUID())
	}
	return resources, date, uuid, f.err
}

func (f *Database) QueryNamespacedResources(ctx context.Context, kind, version, namespace, limit, continueUUID, continueDate string) ([]*unstructured.Unstructured, string, string, error) {
	resources := f.filterResourcesByKindApiVersionAndNamespace(kind, version, namespace)
	var date string
	var uuid string
	if len(resources) > 0 {
		date = resources[len(resources)-1].GetCreationTimestamp().Format(time.RFC3339)
		uuid = string(resources[len(resources)-1].GetUID())
	}
	return resources, date, uuid, f.err
}

func (f *Database) QueryNamespacedResourceByName(ctx context.Context, kind, version, namespace, name string) (*unstructured.Unstructured, error) {
	resources := f.filterResourceByKindApiVersionNamespaceAndName(kind, version, namespace, name)
	if len(resources) > 1 {
		return nil, fmt.Errorf("More than one resource found")
	}
	if len(resources) == 0 {
		return nil, f.err
	}
	return resources[0], f.err
}

func (f *Database) filterResoucesByKindAndApiVersion(kind, apiVersion string) []*unstructured.Unstructured {
	var filteredResources []*unstructured.Unstructured
	for _, resource := range f.resources {
		if resource.GetKind() == kind && resource.GetAPIVersion() == apiVersion {
			filteredResources = append(filteredResources, resource)
		}
	}
	return filteredResources
}

func (f *Database) filterResourcesByKindApiVersionAndNamespace(kind, apiVersion, namespace string) []*unstructured.Unstructured {
	var filteredResources []*unstructured.Unstructured
	for _, resource := range f.resources {
		if resource.GetKind() == kind && resource.GetAPIVersion() == apiVersion && resource.GetNamespace() == namespace {
			filteredResources = append(filteredResources, resource)
		}
	}
	return filteredResources
}

func (f *Database) filterResourceByKindApiVersionNamespaceAndName(kind, apiVersion, namespace, name string) []*unstructured.Unstructured {
	var filteredResources []*unstructured.Unstructured
	for _, resource := range f.resources {
		if resource.GetKind() == kind && resource.GetAPIVersion() == apiVersion && resource.GetNamespace() == namespace && resource.GetName() == name {
			filteredResources = append(filteredResources, resource)
		}
	}
	return filteredResources
}

func (f *Database) WriteResource(ctx context.Context, k8sObj *unstructured.Unstructured, data []byte) error {
	f.resources = append(f.resources, k8sObj)
	return nil
}

func (f *Database) CloseDB() error {
	return f.err
}
