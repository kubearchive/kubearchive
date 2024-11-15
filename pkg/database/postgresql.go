// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package database

import (
	"database/sql"
	"fmt"

	"github.com/huandu/go-sqlbuilder"
	"github.com/jmoiron/sqlx"
	"github.com/kubearchive/kubearchive/pkg/database/facade"
	"github.com/lib/pq"
	_ "github.com/lib/pq"
)

func init() {
	RegisteredDatabases["postgresql"] = NewPostgreSQLDatabase
	RegisteredDBCreators["postgresql"] = NewPostgreSQLCreator
}

type PostgreSQLCreator struct {
	env map[string]string
}

func NewPostgreSQLCreator(env map[string]string) facade.DBCreator {
	return PostgreSQLCreator{env: env}
}

func (creator PostgreSQLCreator) GetDriverName() string {
	return "postgres"
}

func (creator PostgreSQLCreator) GetConnectionString() string {
	return fmt.Sprintf("user=%s password=%s dbname=%s host=%s port=%s sslmode=disable", creator.env[DbUserEnvVar],
		creator.env[DbPasswordEnvVar], creator.env[DbNameEnvVar], creator.env[DbHostEnvVar], creator.env[DbPortEnvVar])
}

type PostgreSQLSelector struct {
	facade.PartialDBSelectorImpl
}

func (PostgreSQLSelector) ResourceSelector() *sqlbuilder.SelectBuilder {
	sb := sqlbuilder.NewSelectBuilder()
	return sb.Select(
		sb.As("data->'metadata'->>'creationTimestamp'", "created_at"),
		"id",
		"data",
	).From("resource")
}

type PostgreSQLFilter struct {
	facade.PartialDBFilterImpl
}

func (PostgreSQLFilter) CreationTSAndIDFilter(cond sqlbuilder.Cond, continueDate, continueId string) string {
	return cond.Var(sqlbuilder.Build(
		"(data->'metadata'->>'creationTimestamp', id) < ($?, $?)",
		continueDate, continueId,
	))
}

func (PostgreSQLFilter) OwnerFilter(cond sqlbuilder.Cond, owners []string) string {
	return cond.Var(sqlbuilder.Build(
		"jsonb_path_query_array(data->'metadata'->'ownerReferences', '$[*].uid') ?| $?",
		pq.Array(owners),
	))
}

type PostgreSQLSorter struct{}

func (PostgreSQLSorter) CreationTSAndIDSorter(sb *sqlbuilder.SelectBuilder) *sqlbuilder.SelectBuilder {
	return sb.OrderBy("data->'metadata'->>'creationTimestamp' DESC", "id DESC")
}

type PostgreSQLInserter struct {
	facade.PartialDBInserterImpl
}

func (PostgreSQLInserter) ResourceInserter(
	uuid, apiVersion, kind, name, namespace, version string,
	clusterDeletedTs sql.NullString,
	data []byte,
) *sqlbuilder.InsertBuilder {
	ib := sqlbuilder.NewInsertBuilder()
	ib.InsertInto("resource")
	ib.Cols("uuid", "api_version", "kind", "name", "namespace", "resource_version", "cluster_deleted_ts", "data")
	ib.Values(uuid, apiVersion, kind, name, namespace, version, clusterDeletedTs, data)
	ib.SQL(ib.Var(sqlbuilder.Build(
		"ON CONFLICT(uuid) DO UPDATE SET name=$?, namespace=$?, resource_version=$?, cluster_deleted_ts=$?, data=$?",
		name, namespace, version, clusterDeletedTs, data,
	)))
	return ib
}

type PostgreSQLDatabase struct {
	*Database
}

func NewPostgreSQLDatabase(conn *sqlx.DB) DBInterface {
	return PostgreSQLDatabase{&Database{
		DB:       conn,
		Flavor:   sqlbuilder.PostgreSQL,
		Selector: PostgreSQLSelector{},
		Filter:   PostgreSQLFilter{},
		Sorter:   PostgreSQLSorter{},
		Inserter: PostgreSQLInserter{},
		Deleter:  facade.DBDeleterImpl{},
	}}
}
