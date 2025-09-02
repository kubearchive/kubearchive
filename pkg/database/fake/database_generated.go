// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package fake

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/kubearchive/kubearchive/pkg/database/interfaces"
	"github.com/kubearchive/kubearchive/pkg/models"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/types"
)

func CreateTestResources() []*unstructured.Unstructured {
	var ret []*unstructured.Unstructured

	// Create a Crontab with status.phase
	crontab := &unstructured.Unstructured{}
	crontab.SetKind("Crontab")
	crontab.SetAPIVersion("stable.example.com/v1")
	crontab.SetName("test")
	crontab.SetNamespace("test")
	crontab.SetUID("crontab-uid-123")
	// Add status.phase for field selector testing
	crontab.Object["status"] = map[string]interface{}{
		"phase": "Running",
	}
	ret = append(ret, crontab)

	// Create a Pod with status.phase and spec.nodeName
	pod := &unstructured.Unstructured{}
	pod.SetKind("Pod")
	pod.SetAPIVersion("v1")
	pod.SetName("test")
	pod.SetNamespace("test")
	pod.SetUID("pod-uid-456")
	// Add status.phase and spec.nodeName for field selector testing
	pod.Object["status"] = map[string]interface{}{
		"phase": "Running",
	}
	pod.Object["spec"] = map[string]interface{}{
		"nodeName": "worker-1",
	}
	ret = append(ret, pod)

	// Create another Pod with different values for testing inequalities
	pod2 := &unstructured.Unstructured{}
	pod2.SetKind("Pod")
	pod2.SetAPIVersion("v1")
	pod2.SetName("test-pod-2")
	pod2.SetNamespace("test")
	pod2.SetUID("pod-uid-789")
	pod2.Object["status"] = map[string]interface{}{
		"phase": "Pending",
	}
	pod2.Object["spec"] = map[string]interface{}{
		"nodeName": "worker-2",
	}
	ret = append(ret, pod2)

	// Create a Deployment for testing
	deployment := &unstructured.Unstructured{}
	deployment.SetKind("Deployment")
	deployment.SetAPIVersion("apps/v1")
	deployment.SetName("test-deployment")
	deployment.SetNamespace("default")
	deployment.SetUID("deployment-uid-101")
	deployment.Object["status"] = map[string]interface{}{
		"phase": "Running",
	}
	ret = append(ret, deployment)

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

func (f *fakeDatabase) queryResources(_ context.Context, kind, version, _, _ string, _ int) []*unstructured.Unstructured {
	return f.filterResourcesByKindAndApiVersion(kind, version)
}

func (f *fakeDatabase) QueryLogURL(_ context.Context, _, _, _, _, _ string) (string, string, error) {
	if len(f.logUrl) == 0 {
		return "", "", f.err
	}
	return f.logUrl[0].Url, f.jsonPath, f.err
}

func (f *fakeDatabase) QueryResources(
	ctx context.Context,
	kind, apiVersion, namespace, name, continueId, continueDate string,
	_ *models.LabelFilters,
	fieldReqs []fields.Requirement,
	limit int,
) ([]string, int64, string, error) {
	var resources []*unstructured.Unstructured

	if name != "" {
		resources = f.queryNamespacedResourceByName(ctx, kind, apiVersion, namespace, name)
	} else if namespace != "" {
		resources = f.filterResourcesByKindApiVersionAndNamespace(kind, apiVersion, namespace)
	} else {
		resources = f.queryResources(ctx, kind, apiVersion, continueId, continueDate, limit)
	}

	// Apply field selector filters if provided
	if len(fieldReqs) > 0 {
		resources = f.filterResourcesByFieldRequirements(resources, fieldReqs)
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

func (f *fakeDatabase) filterResourcesByKindApiVersionAndNamespace(
	kind, apiVersion, namespace string,
) []*unstructured.Unstructured {
	var filteredResources []*unstructured.Unstructured
	for _, resource := range f.resources {
		if resource.GetKind() == kind &&
			resource.GetAPIVersion() == apiVersion &&
			resource.GetNamespace() == namespace {
			filteredResources = append(filteredResources, resource)
		}
	}
	return filteredResources
}

func (f *fakeDatabase) filterResourceByKindApiVersionNamespaceAndName(
	kind, apiVersion, namespace, name string,
) []*unstructured.Unstructured {
	var filteredResources []*unstructured.Unstructured
	for _, resource := range f.resources {
		if resource.GetKind() == kind &&
			resource.GetAPIVersion() == apiVersion &&
			resource.GetNamespace() == namespace &&
			resource.GetName() == name {
			filteredResources = append(filteredResources, resource)
		}
	}
	return filteredResources
}

func (f *fakeDatabase) filterResourcesByFieldRequirements(
	resources []*unstructured.Unstructured,
	fieldReqs []fields.Requirement,
) []*unstructured.Unstructured {
	var filteredResources []*unstructured.Unstructured

	for _, resource := range resources {
		matches := true
		for _, req := range fieldReqs {
			if !f.resourceMatchesFieldRequirement(resource, req) {
				matches = false
				break
			}
		}
		if matches {
			filteredResources = append(filteredResources, resource)
		}
	}

	return filteredResources
}

func (f *fakeDatabase) resourceMatchesFieldRequirement(
	resource *unstructured.Unstructured,
	req fields.Requirement,
) bool {
	// Convert the resource to a map for easier field access
	resourceMap := resource.UnstructuredContent()

	// Get the field value using the field path
	fieldValue := f.getFieldValue(resourceMap, req.Field)

	// Apply the operator
	switch req.Operator {
	case selection.Equals:
		return fieldValue == req.Value
	case selection.NotEquals:
		return fieldValue != req.Value
	default:
		// For unsupported operators, return false
		return false
	}
}

func (f *fakeDatabase) getFieldValue(
	resourceMap map[string]interface{},
	fieldPath string,
) string {
	// Simple field path resolution for common cases
	// This is a simplified implementation for testing purposes

	switch fieldPath {
	case "metadata.name":
		if metadata, ok := resourceMap["metadata"].(map[string]interface{}); ok {
			if nameVal, nameOk := metadata["name"].(string); nameOk {
				return nameVal
			}
		}
	case "metadata.namespace":
		if metadata, ok := resourceMap["metadata"].(map[string]interface{}); ok {
			if ns, nsOk := metadata["namespace"].(string); nsOk {
				return ns
			}
		}
	case "status.phase":
		if status, ok := resourceMap["status"].(map[string]interface{}); ok {
			if phase, phaseOk := status["phase"].(string); phaseOk {
				return phase
			}
		}
	case "spec.nodeName":
		if spec, ok := resourceMap["spec"].(map[string]interface{}); ok {
			if nodeName, nodeOk := spec["nodeName"].(string); nodeOk {
				return nodeName
			}
		}
	}

	// Return empty string for unknown fields
	return ""
}

func (f *fakeDatabase) WriteResource(
	_ context.Context,
	k8sObj *unstructured.Unstructured,
	_ []byte,
	_ time.Time,
	jsonPath string,
	logs ...models.LogTuple,
) (interfaces.WriteResourceResult, error) {
	if f.err != nil {
		return interfaces.WriteResourceResultError, f.err
	}
	if k8sObj == nil {
		return interfaces.WriteResourceResultError,
			errors.New("kubernetes object was 'nil', something went wrong")
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
			f.logUrl = append(f.logUrl, LogUrlRow{
				Uuid:          k8sObj.GetUID(),
				Url:           url.Url,
				ContainerName: url.ContainerName,
				JsonPath:      jsonPath,
			})
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
