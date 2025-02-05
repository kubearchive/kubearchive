// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package fake

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

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

	tests := []struct {
		name     string
		kind     string
		version  string
		expected []*unstructured.Unstructured
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
			expected: testResources[1:2],
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := NewFakeDatabase(testResources, testLogUrls, testJsonPath)
			filteredResources, _, _, err := db.QueryResources(context.TODO(), tt.kind, tt.version, "", "", "", "", 100)
			expected := make([]string, 0)
			if len(tt.expected) != 0 {
				for _, resource := range tt.expected {
					b, jsonErr := json.Marshal(resource)
					if jsonErr != nil {
						t.Fatal(jsonErr)
					}
					expected = append(expected, string(b))
				}
			}

			assert.Equal(t, expected, filteredResources)
			assert.Nil(t, err)
		})
	}
}

func TestQueryResources(t *testing.T) {
	existingKind := testResources[1].GetKind()
	existingVersion := testResources[1].GetAPIVersion()
	existingNamespace := testResources[1].GetNamespace()

	tests := []struct {
		name      string
		kind      string
		version   string
		namespace string
		expected  []*unstructured.Unstructured
	}{
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
			expected:  testResources[1:2],
		},
	}
	db := NewFakeDatabase(testResources, testLogUrls, testJsonPath)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filteredResources, _, _, err := db.QueryResources(context.TODO(), tt.kind, tt.version, tt.namespace,
				"", "", "", 100)
			expected := make([]string, 0)
			if len(tt.expected) != 0 {
				for _, resource := range tt.expected {
					b, jsonErr := json.Marshal(resource)
					if jsonErr != nil {
						t.Fatal(jsonErr)
					}
					expected = append(expected, string(b))
				}
			}
			assert.Equal(t, expected, filteredResources)
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
			expected:     testResources[1:],
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := NewFakeDatabase(tt.testData, testLogUrls, testJsonPath)
			filteredResources, _, _, err := db.QueryResources(context.TODO(), tt.kind, tt.version, tt.namespace,
				tt.resourceName, "", "", 100)
			expected := make([]string, 0)
			if tt.expected != nil {
				for _, exp := range tt.expected {
					b, jsonErr := json.Marshal(exp)
					if jsonErr != nil {
						t.Fatal(jsonErr)
					}
					expected = append(expected, string(b))
				}
			}

			assert.Equal(t, expected, filteredResources)
			assert.Equal(t, tt.err, err)
		})
	}
}

func TestWriteUrls(t *testing.T) {
	t.Parallel()
	k8sObj := &unstructured.Unstructured{}
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
			error:          errors.New("Cannot write log urls to the database when k8sObj is nil"),
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
			var db *Database
			if tt.urlErr != nil {
				db = NewFakeDatabaseWithUrlError(tt.urlErr)
			} else {
				db = NewFakeDatabase(testResources, tt.initialLogUrls, testJsonPath)
			}
			err := db.WriteUrls(context.Background(), tt.obj, tt.jsonPath, tt.newUrls...)
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
			url, jsonPath, _ := db.QueryLogURL(context.Background(), tt.kind, "", "", "")
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
			name: "Write to Database with no errors",
			obj:  &unstructured.Unstructured{},
			data: []byte{},
			err:  errors.New("test error"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var db *Database
			if tt.err != nil {
				db = NewFakeDatabaseWithError(tt.err)
			} else {
				db = NewFakeDatabase([]*unstructured.Unstructured{}, []LogUrlRow{}, "$.")
			}
			err := db.WriteResource(context.Background(), tt.obj, tt.data)
			if tt.err != nil {
				assert.Error(t, err)
				assert.Equal(t, tt.err, err)
			} else {
				assert.Nil(t, err)
			}
		})
	}
}
