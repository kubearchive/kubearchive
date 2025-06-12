// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package fake

import (
	"context"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// Implements filters.Interface and filters based on if that resource kind is registered with the filter. Meant only for
// testing
type Filters struct {
	ArchiveKinds         []string
	DeleteKinds          []string
	ArchiveOnDeleteKinds []string
}

func NewFilters(archiveTypes []string, deleteTypes []string, archiveOnDeleteTypes []string) *Filters {
	return &Filters{
		ArchiveKinds:         archiveTypes,
		DeleteKinds:          deleteTypes,
		ArchiveOnDeleteKinds: archiveOnDeleteTypes,
	}
}

func (f *Filters) MustArchive(ctx context.Context, obj *unstructured.Unstructured) bool {
	for _, kind := range f.ArchiveKinds {
		if obj.GetKind() == kind {
			return true
		}
	}
	return f.MustDelete(ctx, obj)
}

func (f *Filters) MustDelete(ctx context.Context, obj *unstructured.Unstructured) bool {
	for _, kind := range f.DeleteKinds {
		if obj.GetKind() == kind {
			return true
		}
	}
	return false
}

func (f *Filters) MustArchiveOnDelete(ctx context.Context, obj *unstructured.Unstructured) bool {
	for _, kind := range f.ArchiveOnDeleteKinds {
		if obj.GetKind() == kind {
			return true
		}
	}
	return false
}

func (f *Filters) IsConfigured(ctx context.Context, obj *unstructured.Unstructured) bool {
	return f.MustArchiveOnDelete(ctx, obj) || f.MustArchive(ctx, obj) || f.MustDelete(ctx, obj)
}
