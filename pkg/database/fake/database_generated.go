package fake

import (
	"context"
	"fmt"
	"time"

	"github.com/kubearchive/kubearchive/pkg/models"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
)

func CreateTestResources() []*unstructured.Unstructured {
	var ret []*unstructured.Unstructured
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

func CreateTestLogUrls() []LogUrlRow {
	ret := make([]LogUrlRow, 0)
	ret = append(ret, LogUrlRow{Uuid: types.UID("abc-123-xyz"), Url: "fake.com", ContainerName: "container-1"})
	ret = append(ret, LogUrlRow{Uuid: types.UID("abc-123-xyz"), Url: "fake.org", ContainerName: "container-2"})
	ret = append(ret, LogUrlRow{Uuid: types.UID("asdf-1234-fdsa"), Url: "fake.org", ContainerName: "foo"})
	return ret
}

type LogUrlRow struct {
	Uuid          types.UID
	Url           string
	ContainerName string
}

type Database struct {
	resources []*unstructured.Unstructured
	logUrl    []LogUrlRow
	err       error
}

func NewFakeDatabase(testResources []*unstructured.Unstructured, testLogs []LogUrlRow) *Database {
	return &Database{testResources, testLogs, nil}
}

func NewFakeDatabaseWithError(err error) *Database {
	var (
		resources []*unstructured.Unstructured
		logUrls   []LogUrlRow
	)
	return &Database{resources, logUrls, err}
}

func (f *Database) Ping(ctx context.Context) error {
	return f.err
}

func (f *Database) TestConnection(env map[string]string) error {
	return f.err
}

func (f *Database) QueryResources(ctx context.Context, kind, version, limit, continueId, continueDate string) ([]*unstructured.Unstructured, int64, string, error) {
	resources := f.filterResourcesByKindAndApiVersion(kind, version)
	var date string
	var id int64
	if len(resources) > 0 {
		date = resources[len(resources)-1].GetCreationTimestamp().Format(time.RFC3339)
		id = int64(len(resources))
	}
	return resources, id, date, f.err
}

func (f *Database) QueryLogURLs(ctx context.Context, kind, apiVersion, namespace, name string) ([]string, error) {
	if kind == "Pod" {
		return []string{f.logUrl[0].Url}, f.err
	}
	var urls []string
	for _, l := range f.logUrl {
		urls = append(urls, l.Url)
	}
	return urls, f.err
}

func (f *Database) QueryNamespacedResources(ctx context.Context, kind, version, namespace, limit, continueId, continueDate string) ([]*unstructured.Unstructured, int64, string, error) {
	resources := f.filterResourcesByKindApiVersionAndNamespace(kind, version, namespace)
	var date string
	var id int64
	if len(resources) > 0 {
		date = resources[len(resources)-1].GetCreationTimestamp().Format(time.RFC3339)
		id = int64(len(resources))
	}
	return resources, id, date, f.err
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

func (f *Database) filterResourcesByKindAndApiVersion(kind, apiVersion string) []*unstructured.Unstructured {
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

func (f *Database) WriteUrls(ctx context.Context, k8sObj *unstructured.Unstructured, logs ...models.LogTuple) error {
	newLogUrls := make([]LogUrlRow, 0)
	for _, row := range f.logUrl {
		if k8sObj.GetUID() != row.Uuid {
			newLogUrls = append(newLogUrls, row)
		}
	}
	f.logUrl = newLogUrls

	for _, url := range logs {
		f.logUrl = append(f.logUrl, LogUrlRow{Uuid: k8sObj.GetUID(), Url: url.Url, ContainerName: url.ContainerName})
	}
	return nil
}

func (f *Database) CloseDB() error {
	return f.err
}
