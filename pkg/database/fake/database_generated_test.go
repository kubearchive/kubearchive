// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package fake

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/kubearchive/kubearchive/pkg/models"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
)

var testResources = CreateTestResources()
var testLogUrls = CreateTestLogUrls()
var testJsonPath = "$."

func TestNewFakeDatabase(t *testing.T) {
	tests := []struct {
		name      string
		resources []*unstructured.Unstructured
		logUrls   []LogUrlRow
	}{
		{
			name:      "the database is created with no resources",
			resources: nil,
			logUrls:   nil,
		},
		{
			name:      "the database is created with test resources",
			resources: testResources,
			logUrls:   testLogUrls,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := NewFakeDatabase(tt.resources, tt.logUrls, testJsonPath)
			assert.Equal(t, tt.resources, db.resources)
			assert.Equal(t, tt.logUrls, db.logUrl)
		})
	}
}

func TestQueryResourcesWithoutNamespace(t *testing.T) {
	existingKind := testResources[1].GetKind()
	existingVersion := testResources[1].GetAPIVersion()
	existingNamespace := testResources[1].GetNamespace()

	tests := []struct {
		name      string
		kind      string
		namespace string
		version   string
		expected  []*unstructured.Unstructured
	}{
		{
			name:     "No matching resources by kind",
			kind:     "NotFound",
			version:  existingVersion,
			expected: []*unstructured.Unstructured{},
		},
		{
			name:     "No matching resources by version",
			kind:     existingKind,
			version:  "v2",
			expected: []*unstructured.Unstructured{},
		},
		{
			name:     "Matching resources",
			kind:     existingKind,
			version:  existingVersion,
			expected: testResources[0:4],
		},
		{
			name:      "No matching resources by kind",
			kind:      "NotFound",
			version:   existingVersion,
			namespace: existingNamespace,
			expected:  nil,
		},
		{
			name:      "No matching resources by version",
			kind:      existingKind,
			version:   "v2",
			namespace: existingNamespace,
			expected:  []*unstructured.Unstructured{},
		},
		{
			name:      "No matching resources by namespace",
			kind:      existingKind,
			version:   existingVersion,
			namespace: "notfound",
			expected:  []*unstructured.Unstructured{},
		},
		{
			name:      "Matching resources",
			kind:      existingKind,
			version:   existingVersion,
			namespace: existingNamespace,
			expected:  testResources[0:4],
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := NewFakeDatabase(testResources, testLogUrls, testJsonPath)
			filteredResources, err := db.QueryResources(context.TODO(), tt.kind, tt.version, tt.namespace, "", "", "", &models.LabelFilters{}, nil, nil, 100)
			expectedUids := make([]string, 0)
			for _, resource := range tt.expected {
				expectedUids = append(expectedUids, string(resource.GetUID()))
			}

			returnedUids := make([]string, 0)
			for _, resource := range filteredResources {
				var resourceObj unstructured.Unstructured
				deserializeErr := json.Unmarshal([]byte(resource.Data), &resourceObj)
				if deserializeErr != nil {
					t.Fatal(deserializeErr)
				}

				returnedUids = append(returnedUids, string(resourceObj.GetUID()))
			}

			assert.Equal(t, expectedUids, returnedUids)
			assert.Nil(t, err)
		})
	}
}

func TestQueryNamespacedResourceByName(t *testing.T) {
	existingKind := testResources[1].GetKind()
	existingVersion := testResources[1].GetAPIVersion()
	existingNamespace := testResources[1].GetNamespace()
	existingName := testResources[1].GetName()

	tests := []struct {
		name         string
		kind         string
		version      string
		namespace    string
		resourceName string
		testData     []*unstructured.Unstructured
		err          error
		expected     []*unstructured.Unstructured
	}{
		{
			name:         "No matching resources by kind",
			kind:         "NotFound",
			version:      existingVersion,
			namespace:    existingNamespace,
			resourceName: existingName,
			testData:     testResources,
			err:          nil,
			expected:     nil,
		},
		{
			name:         "No matching resources by version",
			kind:         existingKind,
			version:      "v2",
			namespace:    existingNamespace,
			resourceName: existingName,
			testData:     testResources,
			err:          nil,
			expected:     nil,
		},
		{
			name:         "No matching resources by namespace",
			kind:         existingKind,
			version:      existingVersion,
			namespace:    "notfound",
			resourceName: existingName,
			testData:     testResources,
			err:          nil,
			expected:     nil,
		},
		{
			name:         "No matching resources by name",
			kind:         existingKind,
			version:      existingVersion,
			namespace:    existingNamespace,
			resourceName: "notfound",
			testData:     testResources,
			err:          nil,
			expected:     nil,
		},
		{
			name:         "Matching resources",
			kind:         existingKind,
			version:      existingVersion,
			namespace:    existingNamespace,
			resourceName: existingName,
			testData:     testResources,
			err:          nil,
			expected:     testResources[1:2],
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := NewFakeDatabase(tt.testData, testLogUrls, testJsonPath)
			filteredResources, err := db.QueryResources(context.TODO(), tt.kind, tt.version, tt.namespace,
				tt.resourceName, "", "", &models.LabelFilters{}, nil, nil, 100)

			expectedUids := make([]string, 0)
			for _, resource := range tt.expected {
				expectedUids = append(expectedUids, string(resource.GetUID()))
			}

			returnedUids := make([]string, 0)
			for _, resource := range filteredResources {
				var resourceObj unstructured.Unstructured
				deserializeErr := json.Unmarshal([]byte(resource.Data), &resourceObj)
				if deserializeErr != nil {
					t.Fatal(deserializeErr)
				}

				returnedUids = append(returnedUids, string(resourceObj.GetUID()))
			}

			assert.Equal(t, expectedUids, returnedUids)
			assert.Equal(t, tt.err, err)
		})
	}
}

func TestWriteUrls(t *testing.T) {
	t.Parallel()
	k8sObj := &unstructured.Unstructured{}
	k8sObj.SetKind("Pod")
	k8sObj.SetUID(types.UID("abc-123-xyz"))
	newUrls := []models.LogTuple{
		{Url: "https://github.com/kubearchive", ContainerName: "container-1"},
		{Url: "https://example.com", ContainerName: "container-2"},
	}
	jsonPath := "$.hits.hits[*]._source.message"
	tests := []struct {
		name           string
		initialLogUrls []LogUrlRow
		obj            *unstructured.Unstructured
		jsonPath       string
		newUrls        []models.LogTuple
		expected       []LogUrlRow
		error          error
		urlErr         error
	}{
		{
			name:           "Insert log urls into empty table",
			initialLogUrls: []LogUrlRow{},
			obj:            k8sObj,
			jsonPath:       jsonPath,
			newUrls:        newUrls,
			expected: []LogUrlRow{
				{Uuid: k8sObj.GetUID(), Url: newUrls[0].Url, ContainerName: newUrls[0].ContainerName, JsonPath: jsonPath},
				{Uuid: k8sObj.GetUID(), Url: newUrls[1].Url, ContainerName: newUrls[1].ContainerName, JsonPath: jsonPath},
			},
			error: nil,
		},
		{
			name: "Insert log urls into table with no uuid matches",
			initialLogUrls: []LogUrlRow{
				{Uuid: types.UID("asdf-1234-fdsa"), Url: "https://fake.com"},
			},
			obj:      k8sObj,
			jsonPath: jsonPath,
			newUrls:  newUrls,
			expected: []LogUrlRow{
				{Uuid: types.UID("asdf-1234-fdsa"), Url: "https://fake.com"},
				{Uuid: k8sObj.GetUID(), Url: newUrls[0].Url, ContainerName: newUrls[0].ContainerName, JsonPath: jsonPath},
				{Uuid: k8sObj.GetUID(), Url: newUrls[1].Url, ContainerName: newUrls[1].ContainerName, JsonPath: jsonPath},
			},
			error: nil,
		},
		{
			name:           "Insert log urls into table with uuid matches",
			initialLogUrls: testLogUrls,
			obj:            k8sObj,
			jsonPath:       jsonPath,
			newUrls:        newUrls,
			expected: []LogUrlRow{
				{Uuid: types.UID("asdf-1234-fdsa"), Url: "fake.org", ContainerName: "foo"},
				{Uuid: k8sObj.GetUID(), Url: newUrls[0].Url, ContainerName: newUrls[0].ContainerName, JsonPath: jsonPath},
				{Uuid: k8sObj.GetUID(), Url: newUrls[1].Url, ContainerName: newUrls[1].ContainerName, JsonPath: jsonPath},
			},
			error: nil,
		},
		{
			name:           "Nil k8sObj returns error with no change to database",
			initialLogUrls: testLogUrls,
			obj:            nil,
			jsonPath:       jsonPath,
			newUrls:        newUrls,
			expected:       testLogUrls,
			error:          errors.New("kubernetes object was 'nil', something went wrong"),
		},
		{
			name:           "WriteUrls fails when urlErr is not nil",
			initialLogUrls: []LogUrlRow{},
			obj:            k8sObj,
			jsonPath:       jsonPath,
			newUrls:        newUrls,
			expected:       []LogUrlRow{},
			error:          nil,
			urlErr:         errors.New("test error"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var db *fakeDatabase
			if tt.urlErr != nil {
				db = NewFakeDatabaseWithUrlError(tt.urlErr)
			} else {
				db = NewFakeDatabase(testResources, tt.initialLogUrls, testJsonPath)
			}
			_, err := db.WriteResource(context.Background(), tt.obj, []byte(""), time.Now(), tt.jsonPath, tt.newUrls...)
			if tt.urlErr != nil {
				assert.Equal(t, tt.urlErr, err)
			} else {
				assert.Equal(t, tt.expected, db.logUrl)
				assert.Equal(t, tt.error, err)
			}
		})
	}
}

func TestQueryLogURLs(t *testing.T) {

	tests := []struct {
		name     string
		kind     string
		expected string
	}{
		{
			name:     "Logs from one pod",
			kind:     "Pod",
			expected: "fake.com",
		},
		{
			name:     "Logs from another resource",
			kind:     "Job",
			expected: "fake.com",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := NewFakeDatabase(testResources, testLogUrls, testJsonPath)
			url, jsonPath, _ := db.QueryLogURL(context.Background(), tt.kind, "", "", "", "")
			assert.Equal(t, tt.expected, url)
			assert.Equal(t, testJsonPath, jsonPath)
		})
	}
}

func TestWriteResources(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		obj  *unstructured.Unstructured
		data []byte
		err  error
	}{
		{
			name: "Write to Database with no errors",
			obj:  &unstructured.Unstructured{},
			data: []byte{},
			err:  nil,
		},
		{
			name: "Write to Database with error",
			obj:  &unstructured.Unstructured{},
			data: []byte{},
			err:  errors.New("test error"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var db *fakeDatabase
			if tt.err != nil {
				db = NewFakeDatabaseWithError(tt.err)
			} else {
				db = NewFakeDatabase([]*unstructured.Unstructured{}, []LogUrlRow{}, "$.")
			}
			_, err := db.WriteResource(context.Background(), tt.obj, tt.data, time.Now(), "jsonPath", []models.LogTuple{}...)
			if tt.err != nil {
				assert.Error(t, err)
				assert.Equal(t, tt.err, err)
			} else {
				assert.Nil(t, err)
			}
		})
	}
}

func TestQueryResourcesWithWildcardName(t *testing.T) {
	// Create minimal test resources inline
	pod1 := &unstructured.Unstructured{}
	pod1.SetKind("Pod")
	pod1.SetAPIVersion("v1")
	pod1.SetName("test-e2e-pod")
	pod1.SetNamespace("test")

	pod2 := &unstructured.Unstructured{}
	pod2.SetKind("Pod")
	pod2.SetAPIVersion("v1")
	pod2.SetName("production-deployment")
	pod2.SetNamespace("test")

	testResources := []*unstructured.Unstructured{pod1, pod2}

	tests := []struct {
		namePattern   string
		expectedCount int
	}{
		{"*e2e*", 1},      // should match "test-e2e-pod"
		{"test-*", 1},     // should match "test-e2e-pod"
		{"*prod*", 1},     // should match "production-deployment"
		{"*notfound*", 0}, // should match nothing
	}

	for _, tt := range tests {
		t.Run(tt.namePattern, func(t *testing.T) {
			db := NewFakeDatabase(testResources, []LogUrlRow{}, "$.")
			resources, err := db.QueryResources(context.TODO(), "Pod", "v1", "test",
				tt.namePattern, "", "", &models.LabelFilters{}, nil, nil, 100)

			assert.NoError(t, err)
			assert.Equal(t, tt.expectedCount, len(resources))
		})
	}
}
