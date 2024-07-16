package fake

import (
	"context"
	"fmt"

	"github.com/kubearchive/kubearchive/pkg/models"
)

type Database struct {
	resources []models.Resource
	entries   []models.ResourceEntry
}

func NewFakeDatabase(testResources []models.Resource) *Database {
	return &Database{testResources, []models.ResourceEntry{}}
}

func (f *Database) QueryResources(ctx context.Context, kind, group, version string) ([]models.Resource, error) {
	var filteredResources []models.Resource
	for _, resource := range f.resources {
		if resource.Kind == kind && resource.ApiVersion == fmt.Sprintf("%s/%s", group, version) {
			filteredResources = append(filteredResources, resource)
		}
	}
	return filteredResources, nil
}

func (f *Database) QueryNamespacedResources(ctx context.Context, kind, group, version, namespace string) ([]models.Resource, error) {
	var filteredResources []models.Resource
	for _, resource := range f.resources {
		if resource.Kind == kind && resource.ApiVersion == fmt.Sprintf("%s/%s", group, version) && resource.Metadata["namespace"] == namespace {
			filteredResources = append(filteredResources, resource)
		}
	}
	return filteredResources, nil
}

func (f *Database) WriteResource(ctx context.Context, entry *models.ResourceEntry) error {
	f.entries = append(f.entries, *entry)
	return nil
}
