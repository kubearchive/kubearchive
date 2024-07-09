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

func (f *Database) QueryResourceId(ctx context.Context, entry *models.ResourceEntry) (int64, error) {
	for id, dbEntry := range f.entries {
		if dbEntry.Kind == entry.Kind && dbEntry.ApiVersion == entry.ApiVersion && dbEntry.Cluster == entry.Cluster && dbEntry.ClusterUid == entry.ClusterUid && dbEntry.Name == entry.Name && dbEntry.Namespace == entry.Namespace {
			return int64(id), nil
		}
	}
	return 0, fmt.Errorf("no entry found")
}

func (f *Database) UpdateResource(ctx context.Context, id int64, entry *models.ResourceEntry) error {
	if len(f.entries) <= int(id) {
		return fmt.Errorf("id %d not in database", id)
	}
	f.entries[id] = *entry
	return nil
}

func (f *Database) WriteResource(ctx context.Context, entry *models.ResourceEntry) error {
	f.entries = append(f.entries, *entry)
	return nil
}
