// Copyright Kronicler Authors
// SPDX-License-Identifier: Apache-2.0

package fake

import (
	"context"
	"testing"

	"github.com/kronicler/kronicler/cmd/sink/filters"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func createUnstructured(t *testing.T, kind string) *unstructured.Unstructured {
	t.Helper()
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"kind": kind,
		},
	}
}

func TestFakeFilters(t *testing.T) {
	var filter filters.Interface
	tests := []struct {
		name                  string
		archiveKinds          []string
		deleteKinds           []string
		archiveOnDeleteKinds  []string
		kinds                 []string
		archiveResult         bool
		deleteResult          bool
		archiveOnDeleteResult bool
	}{
		{
			name:                  "All objects should not match",
			archiveKinds:          []string{},
			deleteKinds:           []string{},
			archiveOnDeleteKinds:  []string{},
			kinds:                 []string{"Job", "Service", "Pod", "Deployment"},
			archiveResult:         false,
			deleteResult:          false,
			archiveOnDeleteResult: false,
		},
		{
			name:                  "Only archive should match",
			archiveKinds:          []string{"Job", "Service"},
			deleteKinds:           []string{},
			archiveOnDeleteKinds:  []string{},
			kinds:                 []string{"Job", "Service"},
			archiveResult:         true,
			deleteResult:          false,
			archiveOnDeleteResult: false,
		},
		{
			name:                  "Archive does not match",
			archiveKinds:          []string{"Deployment", "Pod"},
			deleteKinds:           []string{},
			archiveOnDeleteKinds:  []string{},
			kinds:                 []string{"Job", "Service"},
			archiveResult:         false,
			deleteResult:          false,
			archiveOnDeleteResult: false,
		},
		{
			name:                  "delete should match and cause archive to match",
			archiveKinds:          []string{},
			deleteKinds:           []string{"Deployment", "ConfigMap"},
			archiveOnDeleteKinds:  []string{},
			kinds:                 []string{"Deployment", "ConfigMap"},
			archiveResult:         true,
			deleteResult:          true,
			archiveOnDeleteResult: false,
		},
		{
			name:                  "Delete does not match",
			archiveKinds:          []string{},
			deleteKinds:           []string{"Job", "Service"},
			archiveOnDeleteKinds:  []string{},
			kinds:                 []string{"Deployment", "ConfigMap"},
			archiveResult:         false,
			deleteResult:          false,
			archiveOnDeleteResult: false,
		},
		{
			name:                  "Only archiveOnDelete should match",
			archiveKinds:          []string{},
			deleteKinds:           []string{},
			archiveOnDeleteKinds:  []string{"ConfigMap", "Secret"},
			kinds:                 []string{"Secret", "ConfigMap"},
			archiveResult:         false,
			deleteResult:          false,
			archiveOnDeleteResult: true,
		},
		{
			name:                  "ArchiveOnDelete does not match",
			archiveKinds:          []string{},
			deleteKinds:           []string{},
			archiveOnDeleteKinds:  []string{"ConfigMap", "Secret"},
			kinds:                 []string{"Deployment", "Pod"},
			archiveResult:         false,
			deleteResult:          false,
			archiveOnDeleteResult: false,
		},
		{
			name:                  "All should match",
			archiveKinds:          []string{"ConfigMap", "Secret"},
			deleteKinds:           []string{"ConfigMap", "Secret"},
			archiveOnDeleteKinds:  []string{"ConfigMap", "Secret"},
			kinds:                 []string{"ConfigMap", "Secret"},
			archiveResult:         true,
			deleteResult:          true,
			archiveOnDeleteResult: true,
		},
		{
			name:                  "None match",
			archiveKinds:          []string{"ConfigMap", "Secret"},
			deleteKinds:           []string{"ConfigMap", "Secret"},
			archiveOnDeleteKinds:  []string{"ConfigMap", "Secret"},
			kinds:                 []string{"Pod", "Job"},
			archiveResult:         false,
			deleteResult:          false,
			archiveOnDeleteResult: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter = NewFilters(tt.archiveKinds, tt.deleteKinds, tt.archiveOnDeleteKinds)
			for _, kind := range tt.kinds {
				obj := createUnstructured(t, kind)
				assert.Equal(t, tt.archiveResult, filter.MustArchive(context.Background(), obj))
				assert.Equal(t, tt.deleteResult, filter.MustDelete(context.Background(), obj))
				assert.Equal(t, tt.archiveOnDeleteResult, filter.MustArchiveOnDelete(context.Background(), obj))
			}
		})
	}
}
