package fake

import (
	"context"
	"fmt"

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

func (f *Database) QueryResources(ctx context.Context, kind, group, version string) ([]*unstructured.Unstructured, error) {
	apiVersion := fmt.Sprintf("%s/%s", group, version)
	return f.filterResoucesByKindAndApiVersion(kind, apiVersion), f.err
}

func (f *Database) QueryNamespacedResources(ctx context.Context, kind, group, version, namespace string) ([]*unstructured.Unstructured, error) {
	apiVersion := fmt.Sprintf("%s/%s", group, version)
	return f.filterResourcesByKindApiVersionAndNamespace(kind, apiVersion, namespace), f.err
}

func (f *Database) QueryCoreResources(ctx context.Context, kind, version string) ([]*unstructured.Unstructured, error) {
	return f.filterResoucesByKindAndApiVersion(kind, version), f.err
}

func (f *Database) QueryNamespacedCoreResources(ctx context.Context, kind, version, namespace string) ([]*unstructured.Unstructured, error) {
	return f.filterResourcesByKindApiVersionAndNamespace(kind, version, namespace), f.err
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

func (f *Database) WriteResource(ctx context.Context, k8sObj *unstructured.Unstructured, data []byte) error {
	f.resources = append(f.resources, k8sObj)
	return nil
}
