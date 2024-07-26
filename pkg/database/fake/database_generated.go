package fake

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func CreateTestResources() []*unstructured.Unstructured {
	ret := []*unstructured.Unstructured{}
	obj := &unstructured.Unstructured{}
	obj.SetKind("Crontab")
	obj.SetAPIVersion("stable.example.com/v1")
	obj.SetName("test")
	obj.SetNamespace("test")
	ret = append(ret, obj)
	return ret
}

type Database struct {
	resources []*unstructured.Unstructured
}

func NewFakeDatabase(testResources []*unstructured.Unstructured) *Database {
	return &Database{testResources}
}

func (f *Database) QueryResources(ctx context.Context, kind, group, version string) ([]*unstructured.Unstructured, error) {
	var filteredResources []*unstructured.Unstructured
	for _, resource := range f.resources {
		if resource.GetKind() == kind && resource.GetAPIVersion() == fmt.Sprintf("%s/%s", group, version) {
			filteredResources = append(filteredResources, resource)
		}
	}
	return filteredResources, nil
}

func (f *Database) QueryNamespacedResources(ctx context.Context, kind, group, version, namespace string) ([]*unstructured.Unstructured, error) {
	var filteredResources []*unstructured.Unstructured
	for _, resource := range f.resources {
		if resource.GetKind() == kind && resource.GetAPIVersion() == fmt.Sprintf("%s/%s", group, version) && resource.GetNamespace() == namespace {
			filteredResources = append(filteredResources, resource)
		}
	}
	return filteredResources, nil
}

func (f *Database) WriteResource(ctx context.Context, k8sObj *unstructured.Unstructured, data []byte) error {
	f.resources = append(f.resources, k8sObj)
	return nil
}
