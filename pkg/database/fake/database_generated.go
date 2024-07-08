package fake

import (
	"context"
	"fmt"
	"github.com/kubearchive/kubearchive/pkg/models"
)

type Database struct {
	resources []models.Resource
}

func NewFakeDatabase(testResources []models.Resource) *Database {
	return &Database{testResources}
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
