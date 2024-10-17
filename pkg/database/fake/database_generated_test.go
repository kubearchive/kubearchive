package fake

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

var testResources = CreateTestResources()

func TestNewFakeDatabase(t *testing.T) {
	tests := []struct {
		name      string
		resources []*unstructured.Unstructured
	}{
		{
			name:      "the database is created with no resources",
			resources: nil,
		},
		{
			name:      "the database is created with test resources",
			resources: testResources,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.resources, NewFakeDatabase(tt.resources).resources)
		})
	}
}

func TestQueryResources(t *testing.T) {
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
			expected: nil,
		},
		{
			name:     "No matching resources by version",
			kind:     existingKind,
			version:  "v2",
			expected: nil,
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
			db := NewFakeDatabase(testResources)
			filteredResources, _, _, err := db.QueryResources(context.TODO(), tt.kind, tt.version, "", "", "")
			assert.Equal(t, tt.expected, filteredResources)
			assert.Nil(t, err)
		})
	}
}

func TestQueryNamespacedResources(t *testing.T) {
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
			expected:  nil,
		},
		{
			name:      "No matching resources by namespace",
			kind:      existingKind,
			version:   existingVersion,
			namespace: "notfound",
			expected:  nil,
		},
		{
			name:      "Matching resources",
			kind:      existingKind,
			version:   existingVersion,
			namespace: existingNamespace,
			expected:  testResources[1:2],
		},
	}
	db := NewFakeDatabase(testResources)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filteredResources, _, _, err := db.QueryNamespacedResources(context.TODO(), tt.kind, tt.version, tt.namespace, "", "", "")
			assert.Equal(t, tt.expected, filteredResources)
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
		expected     *unstructured.Unstructured
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
			expected:     testResources[1],
		},
		{
			name:         "More than one matching resources",
			kind:         existingKind,
			version:      existingVersion,
			namespace:    existingNamespace,
			resourceName: existingName,
			testData:     append(testResources, testResources...),
			err:          fmt.Errorf("More than one resource found"),
			expected:     nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := NewFakeDatabase(tt.testData)
			filteredResources, err := db.QueryNamespacedResourceByName(context.TODO(), tt.kind, tt.version, tt.namespace, tt.resourceName)
			assert.Equal(t, tt.expected, filteredResources)
			assert.Equal(t, tt.err, err)
		})
	}
}
