// Copyright Kronicler Authors
// SPDX-License-Identifier: Apache-2.0

package database

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/huandu/go-sqlbuilder"
	"github.com/jmoiron/sqlx"
	"github.com/kronicler/kronicler/pkg/database/facade"
	"github.com/lib/pq"
	_ "github.com/lib/pq"
)

func init() {
	RegisteredDatabases["postgresql"] = newPostgreSQLDatabase
	RegisteredDBCreators["postgresql"] = newPostgreSQLCreator
}

type postgreSQLCreator struct {
	env map[string]string
}

func newPostgreSQLCreator(env map[string]string) facade.DBCreator {
	return postgreSQLCreator{env: env}
}

func (creator postgreSQLCreator) GetDriverName() string {
	return "postgres"
}

func (creator postgreSQLCreator) GetConnectionString() string {
	return fmt.Sprintf("user=%s password=%s dbname=%s host=%s port=%s sslmode=require", creator.env[DbUserEnvVar],
		creator.env[DbPasswordEnvVar], creator.env[DbNameEnvVar], creator.env[DbHostEnvVar], creator.env[DbPortEnvVar])
}

type postgreSQLSelector struct {
	facade.PartialDBSelectorImpl
}

func (postgreSQLSelector) ResourceSelector() *sqlbuilder.SelectBuilder {
	sb := sqlbuilder.NewSelectBuilder()
	return sb.Select(
		sb.As("data->'metadata'->>'creationTimestamp'", "created_at"),
		"id",
		"data",
	).From("resource")
}

func (postgreSQLSelector) OwnedResourceSelector() *sqlbuilder.SelectBuilder {
	sb := sqlbuilder.NewSelectBuilder()
	return sb.Select(
		"uuid",
		"kind",
		sb.As("data->'metadata'->>'creationTimestamp'", "created_at"),
	).From("resource")
}

type postgreSQLFilter struct {
	facade.PartialDBFilterImpl
}

func (postgreSQLFilter) CreationTSAndIDFilter(cond sqlbuilder.Cond, continueDate, continueId string) string {
	return fmt.Sprintf(
		"(data->'metadata'->>'creationTimestamp', id) < (%s, %s)",
		cond.Var(continueDate), cond.Var(continueId),
	)
}

func (postgreSQLFilter) OwnerFilter(cond sqlbuilder.Cond, owners []string) string {
	return fmt.Sprintf(
		"jsonb_path_query_array(data->'metadata'->'ownerReferences', '$[*].uid') ?| %s",
		cond.Var(pq.Array(owners)),
	)
}

func (postgreSQLFilter) ExistsLabelFilter(cond sqlbuilder.Cond, labels []string) string {
	return fmt.Sprintf(
		"data->'metadata'->'labels' ?& %s",
		cond.Var(pq.Array(labels)),
	)
}

func (postgreSQLFilter) NotExistsLabelFilter(cond sqlbuilder.Cond, labels []string) string {
	return fmt.Sprintf(
		"NOT data->'metadata'->'labels' ?| %s",
		cond.Var(pq.Array(labels)),
	)
}

func (postgreSQLFilter) EqualsLabelFilter(cond sqlbuilder.Cond, labels map[string]string) string {
	jsonLabels, _ := json.Marshal(labels)
	return fmt.Sprintf(
		"data->'metadata'->'labels' @> %s",
		cond.Var(string(jsonLabels)),
	)
}

func (postgreSQLFilter) NotEqualsLabelFilter(cond sqlbuilder.Cond, labels map[string]string) string {
	jsons := make([]string, 0)
	for key, value := range labels {
		jsons = append(jsons, fmt.Sprintf("{\"%s\":\"%s\"}", key, value))
	}
	return fmt.Sprintf(
		"NOT data->'metadata'->'labels' @> ANY(%s::jsonb[])",
		cond.Var(pq.Array(jsons)),
	)
}

func (postgreSQLFilter) InLabelFilter(cond sqlbuilder.Cond, labels map[string][]string) string {
	clauses := make([]string, 0)
	for key, values := range labels {
		jsons := make([]string, 0)
		for _, value := range values {
			jsons = append(jsons, fmt.Sprintf("{\"%s\":\"%s\"}", key, value))
		}
		clauses = append(clauses, fmt.Sprintf(
			"data->'metadata'->'labels' @> ANY(%s::jsonb[])",
			cond.Var(pq.Array(jsons))))
	}
	return cond.And(clauses...)
}

func (f postgreSQLFilter) NotInLabelFilter(cond sqlbuilder.Cond, labels map[string][]string) string {
	keys := maps.Keys(labels)
	jsons := make([]string, 0)
	for key, values := range labels {
		for _, value := range values {
			jsons = append(jsons, fmt.Sprintf("{\"%s\":\"%s\"}", key, value))
		}
	}
	notContainsClause := fmt.Sprintf(
		"NOT data->'metadata'->'labels' @> ANY(%s::jsonb[])",
		cond.Var(pq.Array(jsons)))
	return cond.And(f.ExistsLabelFilter(cond, slices.Collect(keys)), notContainsClause)
}

type postgreSQLSorter struct{}

func (postgreSQLSorter) CreationTSAndIDSorter(sb *sqlbuilder.SelectBuilder) *sqlbuilder.SelectBuilder {
	return sb.OrderBy("data->'metadata'->>'creationTimestamp' DESC", "id DESC")
}

type postgreSQLInserter struct {
	facade.PartialDBInserterImpl
}

func (postgreSQLInserter) ResourceInserter(
	uuid, apiVersion, kind, name, namespace, version string,
	clusterUpdatedTs time.Time,
	clusterDeletedTs sql.NullString,
	data []byte,
) *sqlbuilder.InsertBuilder {
	ib := sqlbuilder.NewInsertBuilder()
	ib.InsertInto("resource")
	ib.Cols(
		"uuid", "api_version", "kind", "name", "namespace", "resource_version", "cluster_updated_ts",
		"cluster_deleted_ts", "data",
	)
	ib.Values(uuid, apiVersion, kind, name, namespace, version, clusterUpdatedTs, clusterDeletedTs, data)
	ib.SQL(ib.Var(sqlbuilder.Build(
		"ON CONFLICT(uuid) DO UPDATE SET name=$?, namespace=$?, resource_version=$?, cluster_updated_ts=$?, cluster_deleted_ts=$?, data=$?",
		name, namespace, version, clusterUpdatedTs, clusterDeletedTs, data,
	)))
	ib.SQL(ib.Var(sqlbuilder.Build(
		"WHERE resource.cluster_updated_ts < $?",
		clusterUpdatedTs,
	)))
	return ib
}

type postgreSQLDatabase struct {
	*DatabaseImpl
}

func newPostgreSQLDatabase(conn *sqlx.DB) Database {
	return postgreSQLDatabase{&DatabaseImpl{
		db:       conn,
		flavor:   sqlbuilder.PostgreSQL,
		selector: postgreSQLSelector{},
		filter:   postgreSQLFilter{},
		sorter:   postgreSQLSorter{},
		inserter: postgreSQLInserter{},
		deleter:  facade.DBDeleterImpl{},
	}}
}
