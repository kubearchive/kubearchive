// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package fake

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kubearchive/kubearchive/pkg/database/interfaces"
	"github.com/kubearchive/kubearchive/pkg/models"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
)

func matchesWildcard(name, pattern string) bool {
	if !strings.Contains(pattern, "*") {
		// Exact match, case-insensitive
		return strings.EqualFold(name, pattern)
	}

	nameLower := strings.ToLower(name)
	patternLower := strings.ToLower(pattern)

	if strings.HasPrefix(patternLower, "*") && strings.HasSuffix(patternLower, "*") {
		substring := patternLower[1 : len(patternLower)-1]
		return strings.Contains(nameLower, substring)
	} else if strings.HasPrefix(patternLower, "*") {
		suffix := patternLower[1:]
		return strings.HasSuffix(nameLower, suffix)
	} else if strings.HasSuffix(patternLower, "*") {
		prefix := patternLower[:len(patternLower)-1]
		return strings.HasPrefix(nameLower, prefix)
	}

	parts := strings.Split(patternLower, "*")
	pos := 0
	for i, part := range parts {
		if part == "" {
			continue
		}
		idx := strings.Index(nameLower[pos:], part)
		if idx == -1 {
			return false
		}
		if i == 0 && idx != 0 {
			// First part must match from beginning
			return false
		}
		pos += idx + len(part)
	}

	lastPart := parts[len(parts)-1]
	if lastPart != "" && !strings.HasSuffix(nameLower, lastPart) {
		return false
	}

	return true
}

func CreateTestResources() []*unstructured.Unstructured {
	var ret []*unstructured.Unstructured
	crontab := &unstructured.Unstructured{}
	crontab.SetKind("Crontab")
	crontab.SetAPIVersion("stable.example.com/v1")
	crontab.SetName("test")
	crontab.SetNamespace("test")
	crontab.SetUID(types.UID(uuid.New().String()))
	ret = append(ret, crontab)

	crontab1 := &unstructured.Unstructured{}
	crontab1.SetKind("Crontab")
	crontab1.SetAPIVersion("stable.example.com/v1")
	crontab1.SetName("test-e2e-job")
	crontab1.SetNamespace("test")
	crontab.SetUID(types.UID(uuid.New().String()))
	ret = append(ret, crontab1)

	crontab2 := &unstructured.Unstructured{}
	crontab2.SetKind("Crontab")
	crontab2.SetAPIVersion("stable.example.com/v1")
	crontab2.SetName("my-e2e-service")
	crontab2.SetNamespace("test")
	crontab.SetUID(types.UID(uuid.New().String()))
	ret = append(ret, crontab2)

	crontab3 := &unstructured.Unstructured{}
	crontab3.SetKind("Crontab")
	crontab3.SetAPIVersion("stable.example.com/v1")
	crontab3.SetName("production-deployment")
	crontab3.SetNamespace("test")
	crontab.SetUID(types.UID(uuid.New().String()))
	ret = append(ret, crontab3)

	pod := &unstructured.Unstructured{}
	pod.SetKind("Pod")
	pod.SetAPIVersion("v1")
	pod.SetName("test")
	pod.SetNamespace("test")
	pod.SetUID(types.UID(uuid.New().String()))
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
	resources            []*unstructured.Unstructured
	logUrl               []LogUrlRow
	jsonPath             string
	err                  error
	urlErr               error
	CurrentSchemaVersion string
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

func (f *fakeDatabase) QueryDatabaseSchemaVersion(ctx context.Context) (string, error) {
	return f.CurrentSchemaVersion, nil
}

func (f *fakeDatabase) queryResources(_ context.Context, kind, version, _, _ string, _ int) []models.Resource {
	return f.filterResourcesByKindAndApiVersion(kind, version)
}

func (f *fakeDatabase) QueryLogURLByName(ctx context.Context, kind, apiVersion, namespace, name, containerName string) (string, string, error) {
	return f.queryLogURL(ctx, kind, apiVersion, namespace, name, containerName)
}

func (f *fakeDatabase) QueryLogURLByUID(ctx context.Context, kind, apiVersion, namespace, uid, containerName string) (string, string, error) {
	return f.queryLogURL(ctx, kind, apiVersion, namespace, uid, containerName)
}

func (f *fakeDatabase) queryLogURL(_ context.Context, _, _, _, _, _ string) (string, string, error) {
	if len(f.logUrl) == 0 {
		return "", "", f.err
	}
	return f.logUrl[0].Url, f.jsonPath, f.err
}

func (f *fakeDatabase) QueryResourceByUID(ctx context.Context, kind, apiVersion, namespace, uid string) (*models.Resource, error) {
	for _, resource := range f.resources {
		sameKind := resource.GetKind() == kind
		sameApiVersion := resource.GetAPIVersion() == apiVersion
		sameNamespace := resource.GetNamespace() == namespace
		sameUID := string(resource.GetUID()) == uid

		if sameKind && sameApiVersion && sameNamespace && sameUID {
			resourceString, err := json.Marshal(resource)
			if err != nil {
				panic(fmt.Sprintf("error while serializing resource: %s", resource))
			}
			return &models.Resource{Id: 0, Data: string(resourceString), Date: resource.GetCreationTimestamp().GoString()}, nil
		}
	}

	return nil, f.err
}

func (f *fakeDatabase) QueryResources(ctx context.Context, kind, version, namespace, name,
	continueId, continueDate string, _ *models.LabelFilters,
	creationTimestampAfter, creationTimestampBefore *time.Time, limit int) ([]models.Resource, error) {
	var resources []models.Resource

	if name != "" && strings.Contains(name, "*") {
		resources = f.filterResourcesByKindApiVersionNamespaceAndWildcardName(kind, version, namespace, name)
	} else if name != "" {
		resources = f.queryNamespacedResourceByName(ctx, kind, version, namespace, name)
	} else if namespace != "" {
		resources = f.filterResourcesByKindApiVersionAndNamespace(kind, version, namespace)
	} else {
		resources = f.queryResources(ctx, kind, version, continueId, continueDate, limit)
	}

	if creationTimestampAfter != nil || creationTimestampBefore != nil {
		resources = f.filterResourcesByTimestamp(resources, creationTimestampAfter, creationTimestampBefore)
	}

	return resources, f.err
}

// filterResourcesByTimestamp filters resources based on creation timestamp
func (f *fakeDatabase) filterResourcesByTimestamp(resources []models.Resource,
	creationTimestampAfter, creationTimestampBefore *time.Time) []models.Resource {
	var filteredResources []models.Resource

	for _, resource := range resources {
		creationTime, err := time.Parse(time.RFC3339, resource.Date)
		if err != nil {
			panic(fmt.Sprintf("error while deserializing timestamp: %s", err))
		}

		if creationTimestampAfter != nil && creationTime.Before(*creationTimestampAfter) {
			continue
		}

		if creationTimestampBefore != nil && creationTime.After(*creationTimestampBefore) {
			continue
		}

		filteredResources = append(filteredResources, resource)
	}

	return filteredResources
}

func (f *fakeDatabase) queryNamespacedResourceByName(_ context.Context, kind, version, namespace, name string,
) []models.Resource {
	return f.filterResourceByKindApiVersionNamespaceAndName(kind, version, namespace, name)
}

func (f *fakeDatabase) filterResourcesByKindAndApiVersion(kind, apiVersion string) []models.Resource {
	var filteredResources []models.Resource
	for _, resource := range f.resources {
		if resource.GetKind() == kind && resource.GetAPIVersion() == apiVersion {
			resourceString, err := json.Marshal(resource)
			if err != nil {
				panic(fmt.Sprintf("error while serializing resource: %s", resource))
			}
			filteredResources = append(filteredResources, models.Resource{Id: 0, Data: string(resourceString), Date: resource.GetCreationTimestamp().GoString()})
		}
	}
	return filteredResources
}

func (f *fakeDatabase) filterResourcesByKindApiVersionAndNamespace(kind, apiVersion, namespace string) []models.Resource {
	var filteredResources []models.Resource
	for _, resource := range f.resources {
		if resource.GetKind() == kind && resource.GetAPIVersion() == apiVersion && resource.GetNamespace() == namespace {
			resourceString, err := json.Marshal(resource)
			if err != nil {
				panic(fmt.Sprintf("error while serializing resource: %s", resource))
			}
			filteredResources = append(filteredResources, models.Resource{Id: 0, Data: string(resourceString), Date: resource.GetCreationTimestamp().GoString()})
		}
	}
	return filteredResources
}

func (f *fakeDatabase) filterResourceByKindApiVersionNamespaceAndName(kind, apiVersion, namespace, name string) []models.Resource {
	var filteredResources []models.Resource
	for _, resource := range f.resources {
		if resource.GetKind() == kind && resource.GetAPIVersion() == apiVersion && resource.GetNamespace() == namespace && resource.GetName() == name {
			resourceString, err := json.Marshal(resource)
			if err != nil {
				panic(fmt.Sprintf("error while serializing resource: %s", resource))
			}
			filteredResources = append(filteredResources, models.Resource{Id: 0, Data: string(resourceString), Date: resource.GetCreationTimestamp().GoString()})
		}
	}
	return filteredResources
}

func (f *fakeDatabase) filterResourcesByKindApiVersionNamespaceAndWildcardName(kind, apiVersion, namespace, namePattern string) []models.Resource {
	var filteredResources []models.Resource

	for _, resource := range f.resources {
		matchesKind := resource.GetKind() == kind
		matchesAPIVersion := resource.GetAPIVersion() == apiVersion
		matchesNamespaces := resource.GetNamespace() == namespace

		if matchesKind && matchesAPIVersion && matchesNamespaces && matchesWildcard(resource.GetName(), namePattern) {
			resourceBytes, err := json.Marshal(resource)
			if err != nil {
				panic(fmt.Sprintf("error while serializing resource: %s", resource))
			}

			filteredResources = append(filteredResources, models.Resource{Id: 0, Date: resource.GetCreationTimestamp().GoString(), Data: string(resourceBytes)})
		}
	}

	return filteredResources
}

func (f *fakeDatabase) WriteResource(_ context.Context, k8sObj *unstructured.Unstructured, _ []byte, _ time.Time, jsonPath string, logs ...models.LogTuple) (interfaces.WriteResourceResult, error) {
	if f.err != nil {
		return interfaces.WriteResourceResultError, f.err
	}
	if k8sObj == nil {
		return interfaces.WriteResourceResultError, errors.New("kubernetes object was 'nil', something went wrong")
	}

	if k8sObj.GetKind() == "Pod" {
		if f.urlErr != nil {
			return interfaces.WriteResourceResultError, f.urlErr
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
	}

	f.resources = append(f.resources, k8sObj)
	return interfaces.WriteResourceResultInserted, nil
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
