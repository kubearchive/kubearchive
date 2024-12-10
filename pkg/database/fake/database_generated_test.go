// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0
package fake

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kubearchive/kubearchive/pkg/models"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
)

var testResources = CreateTestResources()
var testLogUrls = CreateTestLogUrls()

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
			db := NewFakeDatabase(tt.resources, tt.logUrls)
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
			db := NewFakeDatabase(testResources, testLogUrls)
			filteredResources, _, _, err := db.QueryResources(context.TODO(), tt.kind, tt.version, "", "", "", "", "")
			expected := []string{}
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
	db := NewFakeDatabase(testResources, testLogUrls)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filteredResources, _, _, err := db.QueryResources(context.TODO(), tt.kind, tt.version, tt.namespace, "", "", "", "")
			expected := []string{}
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
			db := NewFakeDatabase(tt.testData, testLogUrls)
			filteredResources, _, _, err := db.QueryResources(context.TODO(), tt.kind, tt.version, tt.namespace, tt.resourceName, "", "", "")
			expected := []string{}
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
	k8sObj := &unstructured.Unstructured{}
	k8sObj.SetUID(types.UID("abc-123-xyz"))
	newUrls := []models.LogTuple{
		{Url: "https://github.com/kubearchive", ContainerName: "container-1"},
		{Url: "https://example.com", ContainerName: "container-2"},
	}
	tests := []struct {
		name           string
		initialLogUrls []LogUrlRow
		obj            *unstructured.Unstructured
		newUrls        []models.LogTuple
		expected       []LogUrlRow
	}{
		{
			name:           "Insert log urls into empty table",
			initialLogUrls: []LogUrlRow{},
			obj:            k8sObj,
			newUrls:        newUrls,
			expected: []LogUrlRow{
				{Uuid: k8sObj.GetUID(), Url: newUrls[0].Url, ContainerName: newUrls[0].ContainerName},
				{Uuid: k8sObj.GetUID(), Url: newUrls[1].Url, ContainerName: newUrls[1].ContainerName},
			},
		},
		{
			name: "Insert log urls into table with no uuid matches",
			initialLogUrls: []LogUrlRow{
				{Uuid: types.UID("asdf-1234-fdsa"), Url: "https://fake.com"},
			},
			obj:     k8sObj,
			newUrls: newUrls,
			expected: []LogUrlRow{
				{Uuid: types.UID("asdf-1234-fdsa"), Url: "https://fake.com"},
				{Uuid: k8sObj.GetUID(), Url: newUrls[0].Url, ContainerName: newUrls[0].ContainerName},
				{Uuid: k8sObj.GetUID(), Url: newUrls[1].Url, ContainerName: newUrls[1].ContainerName},
			},
		},
		{
			name:           "Insert log urls into table with uuid matches",
			initialLogUrls: testLogUrls,
			obj:            k8sObj,
			newUrls:        newUrls,
			expected: []LogUrlRow{
				{Uuid: types.UID("asdf-1234-fdsa"), Url: "fake.org", ContainerName: "foo"},
				{Uuid: k8sObj.GetUID(), Url: newUrls[0].Url, ContainerName: newUrls[0].ContainerName},
				{Uuid: k8sObj.GetUID(), Url: newUrls[1].Url, ContainerName: newUrls[1].ContainerName},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := NewFakeDatabase(testResources, tt.initialLogUrls)
			err := db.WriteUrls(context.Background(), tt.obj, tt.newUrls...)
			assert.Nil(t, err)
			assert.Equal(t, tt.expected, db.logUrl)
		})
	}
}

func TestQueryLogURLs(t *testing.T) {

	tests := []struct {
		name     string
		kind     string
		expected int
	}{
		{
			name:     "Logs from one pod",
			kind:     "Pod",
			expected: 1,
		},
		{
			name:     "Logs from another resource",
			kind:     "Job",
			expected: len(testLogUrls),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := NewFakeDatabase(testResources, testLogUrls)
			urls, _ := db.QueryLogURLs(context.Background(), tt.kind, "", "", "")
			assert.Equal(t, tt.expected, len(urls))
		})
	}
}
