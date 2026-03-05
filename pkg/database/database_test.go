// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package database

import (
	"sync"
	"testing"

	"github.com/kubearchive/kubearchive/pkg/database/fake"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestNewDatabase(t *testing.T) {
	tests := []struct {
		name          string
		schemaVersion string
		err           string
	}{
		{
			name:          "zero schema version",
			schemaVersion: "0",
			err:           "database schema version 0 is outside accepted range [4, 4]",
		},
		{
			name:          "current schema version",
			schemaVersion: "4",
		},
		{
			name:          "version above max",
			schemaVersion: "5",
			err:           "database schema version 5 is outside accepted range [4, 4]",
		},
		{
			name:          "non-numeric schema version",
			schemaVersion: "v9",
			err:           `invalid database schema version 'v9': expected an integer: strconv.Atoi: parsing "v9": invalid syntax`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			once = sync.Once{}
			db = nil

			t.Setenv("DATABASE_KIND", "fake")
			t.Setenv("DATABASE_DB", "kubearchive")
			t.Setenv("DATABASE_USER", "kubearchive")
			t.Setenv("DATABASE_PASSWORD", "kubearchive")
			t.Setenv("DATABASE_URL", "kubearchive")
			t.Setenv("DATABASE_PORT", "5432")

			fakeDB := fake.NewFakeDatabase([]*unstructured.Unstructured{}, []fake.LogUrlRow{}, "jsonPath")
			fakeDB.CurrentSchemaVersion = test.schemaVersion
			DatabaseSchemaVersions["fake"] = SchemaVersionRange{Min: 4, Max: 4}
			RegisteredDatabases["fake"] = fakeDB

			_, err := newDatabase()
			if test.err != "" {
				assert.EqualError(t, err, test.err)
			} else {
				assert.NoError(t, err)
			}

		})
	}
}
