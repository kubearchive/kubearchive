// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package fake

import (
	"context"
	"encoding/json"
	"errors"
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
	JsonPath      string
}

type fakeDatabase struct {
	resources []*unstructured.Unstructured
	logUrl    []LogUrlRow
	jsonPath  string
	err       error
	urlErr    error
}

func NewFakeDatabase(testResources []*unstructured.Unstructured, testLogs []LogUrlRow, jsonPath string) *fakeDatabase {
	return &fakeDatabase{resources: testResources, logUrl: testLogs, jsonPath: jsonPath}
}

func NewFakeDatabaseWithError(err error) *fakeDatabase {
	var (
		resources []*unstructured.Unstructured
		logUrls   []LogUrlRow
	)
	return &fakeDatabase{resources: resources, logUrl: logUrls, err: err}
}

func NewFakeDatabaseWithUrlError(err error) *fakeDatabase {
	var (
		resources []*unstructured.Unstructured
		logUrls   []LogUrlRow
	)
	return &fakeDatabase{resources: resources, logUrl: logUrls, urlErr: err}
}

func (f *fakeDatabase) Init(_ map[string]string) error {
	return nil
}

func (f *fakeDatabase) Ping(_ context.Context) error {
	return f.err
}

func (f *fakeDatabase) TestConnection(_ map[string]string) error {
	return f.err
}

func (f *fakeDatabase) queryResources(_ context.Context, kind, version, _, _ string, _ int) []*unstructured.Unstructured {
	return f.filterResourcesByKindAndApiVersion(kind, version)
}

func (f *fakeDatabase) QueryLogURL(_ context.Context, _, _, _, _ string) (string, string, error) {
	if len(f.logUrl) == 0 {
		return "", "", f.err
	}
	return f.logUrl[0].Url, f.jsonPath, f.err
}

func (f *fakeDatabase) QueryResources(ctx context.Context, kind, version, namespace, name,
	continueId, continueDate string, _ *models.LabelFilters, limit int) ([]string, int64, string, error) {
	var resources []*unstructured.Unstructured

	if name != "" {
		resources = f.queryNamespacedResourceByName(ctx, kind, version, namespace, name)
	} else if namespace != "" {
		resources = f.filterResourcesByKindApiVersionAndNamespace(kind, version, namespace)
	} else {
		resources = f.queryResources(ctx, kind, version, continueId, continueDate, limit)
	}

	var date string
	var id int64
	if len(resources) > 0 {
		date = resources[len(resources)-1].GetCreationTimestamp().Format(time.RFC3339)
		id = int64(len(resources))
	}

	stringResources := make([]string, len(resources))
	for ix, resource := range resources {
		stringResource, err := json.Marshal(resource)
		if err != nil {
			// We can panic because this is meant for testing
			panic(err.Error())
		}
		stringResources[ix] = string(stringResource)
	}

	return stringResources, id, date, f.err
}

func (f *fakeDatabase) queryNamespacedResourceByName(_ context.Context, kind, version, namespace, name string,
) []*unstructured.Unstructured {
	return f.filterResourceByKindApiVersionNamespaceAndName(kind, version, namespace, name)
}

func (f *fakeDatabase) filterResourcesByKindAndApiVersion(kind, apiVersion string) []*unstructured.Unstructured {
	var filteredResources []*unstructured.Unstructured
	for _, resource := range f.resources {
		if resource.GetKind() == kind && resource.GetAPIVersion() == apiVersion {
			filteredResources = append(filteredResources, resource)
		}
	}
	return filteredResources
}

func (f *fakeDatabase) filterResourcesByKindApiVersionAndNamespace(kind, apiVersion, namespace string) []*unstructured.Unstructured {
	var filteredResources []*unstructured.Unstructured
	for _, resource := range f.resources {
		if resource.GetKind() == kind && resource.GetAPIVersion() == apiVersion && resource.GetNamespace() == namespace {
			filteredResources = append(filteredResources, resource)
		}
	}
	return filteredResources
}

func (f *fakeDatabase) filterResourceByKindApiVersionNamespaceAndName(kind, apiVersion, namespace, name string) []*unstructured.Unstructured {
	var filteredResources []*unstructured.Unstructured
	for _, resource := range f.resources {
		if resource.GetKind() == kind && resource.GetAPIVersion() == apiVersion && resource.GetNamespace() == namespace && resource.GetName() == name {
			filteredResources = append(filteredResources, resource)
		}
	}
	return filteredResources
}

func (f *fakeDatabase) WriteResource(_ context.Context, k8sObj *unstructured.Unstructured, _ []byte, _ time.Time) error {
	if f.err != nil {
		return f.err
	}
	f.resources = append(f.resources, k8sObj)
	return nil
}

func (f *fakeDatabase) WriteUrls(_ context.Context, k8sObj *unstructured.Unstructured, jsonPath string, logs ...models.LogTuple) error {
	if f.urlErr != nil {
		return f.urlErr
	}

	if k8sObj == nil {
		return errors.New("Cannot write log urls to the database when k8sObj is nil")
	}

	newLogUrls := make([]LogUrlRow, 0)
	for _, row := range f.logUrl {
		if k8sObj.GetUID() != row.Uuid {
			newLogUrls = append(newLogUrls, row)
		}
	}
	f.logUrl = newLogUrls

	for _, url := range logs {
		f.logUrl = append(f.logUrl, LogUrlRow{Uuid: k8sObj.GetUID(), Url: url.Url, ContainerName: url.ContainerName, JsonPath: jsonPath})
	}
	return nil
}

func (f *fakeDatabase) NumResources() int {
	return len(f.resources)
}

func (f *fakeDatabase) NumLogUrls() int {
	return len(f.logUrl)
}

func (f *fakeDatabase) CloseDB() error {
	return f.err
}
