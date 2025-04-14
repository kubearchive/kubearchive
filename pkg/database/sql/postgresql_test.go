// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package sql

import (
	"database/sql"
	"errors"
	"os"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"github.com/kubearchive/kubearchive/pkg/database/interfaces"
	"github.com/kubearchive/kubearchive/pkg/models"
	"github.com/stretchr/testify/assert"
)

func TestPostgreSQLWriteResource(t *testing.T) {
	tests := []struct {
		name    string
		inserts []struct {
			string
			time.Time
		}
		err error
	}{
		{
			name: "insert objects successfully",
			inserts: []struct {
				string
				time.Time
			}{
				{
					"../testdata/pod-3-containers.json",
					time.Now(),
				},
				{
					"../testdata/job.json",
					time.Now(),
				},
			},
			err: nil,
		},
		{
			name: "insert and update object",
			inserts: []struct {
				string
				time.Time
			}{
				{
					"../testdata/pod-3-containers.json",
					time.Now(),
				},
				{
					"../testdata/pod-3-containers.json",
					time.Now(),
				},
			},
			err: nil,
		},
		{
			name: "insert twice with no update due to older resource",
			inserts: []struct {
				string
				time.Time
			}{
				{
					"../testdata/pod-3-containers.json",
					time.Now(),
				},
				{
					"../testdata/pod-3-containers.json",
					time.Time{},
				},
			},
			err: nil,
		},
		{
			name: "handle write failure",
			inserts: []struct {
				string
				time.Time
			}{
				{
					"../testdata/pod-3-containers.json",
					time.Now(),
				},
			},
			err: errors.New("error writing to the database"),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for _, insert := range test.inserts {
				database := NewPostgreSQLDatabase()
				db, mock := NewMock()
				database.setConn(sqlx.NewDb(db, "sqlmock"))

				data, err := os.ReadFile(insert.string)
				if err != nil {
					t.Fatal(err)
				}
				obj, err := models.UnstructuredFromByteSlice(data)
				if err != nil {
					t.Fatal(err)
				}

				query, args := database.getInserter().ResourceInserter(
					string(obj.GetUID()),
					obj.GetAPIVersion(),
					obj.GetKind(),
					obj.GetName(),
					obj.GetNamespace(),
					obj.GetResourceVersion(),
					insert.Time,
					sql.NullString{
						Valid: false,
					},
					data,
				).BuildWithFlavor(database.getFlavor())

				mock.ExpectBegin()
				if test.err == nil {
					rows := sqlmock.NewRows([]string{"inserted"})
					rows.AddRow(true)
					mock.ExpectQuery(regexp.QuoteMeta(query)).WithArgs(sliceOfAny2sliceOfValue(args)...).WillReturnRows(rows)
					mock.ExpectCommit()
				} else {
					mock.ExpectQuery(regexp.QuoteMeta(query)).WithArgs(args).WillReturnError(test.err)
					mock.ExpectRollback()
				}

				inserted, dbErr := database.WriteResource(t.Context(), obj, data, insert.Time)
				if test.err == nil {
					assert.Nil(t, dbErr)
					assert.Equal(t, inserted, interfaces.WriteResourceResultInserted)
				} else {
					assert.NotNil(t, dbErr)
					assert.NotEqual(t, inserted, interfaces.WriteResourceResultInserted)
				}
			}
		})
	}
}
