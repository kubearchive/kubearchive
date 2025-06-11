// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package database

import (
	"fmt"
	"testing"

	"github.com/kubearchive/kubearchive/pkg/database/fake"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestNewDatabase(t *testing.T) {
	tests := []struct {
		name          string
		schemaVersion string
		err           error
	}{
		{
			name:          "zero schema version",
			schemaVersion: "0",
			err:           fmt.Errorf("expected database schema version '%s', found '0'", CurrentDatabaseSchemaVersion),
		},
		{
			name:          "current schema version",
			schemaVersion: CurrentDatabaseSchemaVersion,
			err:           nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("DATABASE_KIND", "fake")
			t.Setenv("DATABASE_DB", "kubearchive")
			t.Setenv("DATABASE_USER", "kubearchive")
			t.Setenv("DATABASE_PASSWORD", "kubearchive")
			t.Setenv("DATABASE_URL", "kubearchive")
			t.Setenv("DATABASE_PORT", "5432")

			db := fake.NewFakeDatabase([]*unstructured.Unstructured{}, []fake.LogUrlRow{}, "jsonPath")
			db.CurrentSchemaVersion = test.schemaVersion
			RegisteredDatabases["fake"] = db

			_, err := newDatabase()
			if test.err != nil {
				assert.Equal(t, test.err, err)
			} else {
				assert.NoError(t, err)
			}

		})
	}
}
