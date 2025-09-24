// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package sql

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"os"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/huandu/go-sqlbuilder"
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
		err  error
		logs []models.LogTuple
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
		{
			name: "insert objects successfully with logs",
			inserts: []struct {
				string
				time.Time
			}{
				{
					"../testdata/pod-3-containers.json",
					time.Now(),
				},
			},
			logs: []models.LogTuple{
				{
					ContainerName: "hello",
					Url:           "https://example.com/logs/hello",
				},
			},
			err: nil,
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
				k8sObj, err := models.UnstructuredFromByteSlice(data)
				if err != nil {
					t.Fatal(err)
				}

				mock.ExpectBegin()
				query, args := database.getInserter().ResourceInserter(
					string(k8sObj.GetUID()),
					k8sObj.GetAPIVersion(),
					k8sObj.GetKind(),
					k8sObj.GetName(),
					k8sObj.GetNamespace(),
					k8sObj.GetResourceVersion(),
					insert.Time,
					sql.NullString{
						Valid: false,
					},
					data,
				).BuildWithFlavor(database.getFlavor())

				if test.err != nil {
					mock.ExpectQuery(regexp.QuoteMeta(query)).WithArgs(sliceOfAny2sliceOfValue(args)...).WillReturnError(test.err)
				} else {
					rows := sqlmock.NewRows([]string{"inserted"})
					rows.AddRow(true)
					mock.ExpectQuery(regexp.QuoteMeta(query)).WithArgs(sliceOfAny2sliceOfValue(args)...).WillReturnRows(rows)
				}

				if k8sObj.GetKind() == "Pod" {
					delBuilder := database.getDeleter().UrlDeleter()
					delBuilder.Where(database.filter.UuidFilter(delBuilder.Cond, string(k8sObj.GetUID())))
					query, args := delBuilder.BuildWithFlavor(database.flavor)
					mock.ExpectExec(regexp.QuoteMeta(query)).WithArgs(sliceOfAny2sliceOfValue(args)...).WillReturnResult(driver.ResultNoRows)
				}

				if len(test.logs) >= 1 {
					for _, log := range test.logs {
						logQuery, logArgs := database.getInserter().UrlInserter(
							string(k8sObj.GetUID()),
							log.Url,
							log.ContainerName,
							"jsonPath",
						).BuildWithFlavor(database.flavor)

						mock.ExpectExec(regexp.QuoteMeta(logQuery)).WithArgs(sliceOfAny2sliceOfValue(logArgs)...).WillReturnResult(driver.ResultNoRows)
					}
				}

				mock.ExpectCommit()

				inserted, dbErr := database.WriteResource(context.Background(), k8sObj, data, insert.Time, "jsonPath", test.logs...)
				if test.err == nil {
					assert.Nil(t, dbErr)
					assert.Equal(t, inserted, interfaces.WriteResourceResultInserted)
				} else {
					assert.NotNil(t, dbErr)
					assert.Equal(t, inserted, interfaces.WriteResourceResultError)
				}
			}
		})
	}
}

func TestConvertFieldPathToJSONPath(t *testing.T) {
	tests := []struct {
		name      string
		fieldPath string
		expected  string
	}{
		{
			name:      "simple field",
			fieldPath: "metadata.name",
			expected:  "$.metadata.name",
		},
		{
			name:      "nested field",
			fieldPath: "status.phase",
			expected:  "$.status.phase",
		},
		{
			name:      "deeply nested field",
			fieldPath: "spec.containers.name",
			expected:  "$.spec.containers.name",
		},
		{
			name:      "array access",
			fieldPath: "spec.containers[0].name",
			expected:  "$.spec.containers[0].name",
		},
		{
			name:      "multiple array access",
			fieldPath: "spec.containers[0].env[1].name",
			expected:  "$.spec.containers[0].env[1].name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test the JsonPathPredicateCheck function
			filter := postgreSQLFilter{}
			cond := sqlbuilder.NewCond()
			result := filter.JsonPathPredicateCheck(*cond, tt.fieldPath, "==", "test-value")

			// The result should contain the @@ operator
			assert.Contains(t, result, "data @@")
			// The result should be a valid SQL expression
			assert.NotEmpty(t, result)
			// Verify that the method generates SQL for the given field path
			assert.True(t, len(result) > 0, "JsonPathPredicateCheck should generate SQL")
		})
	}
}
