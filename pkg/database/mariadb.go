// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package database

import (
	"database/sql"
	"fmt"

	_ "github.com/go-sql-driver/mysql"
	"github.com/huandu/go-sqlbuilder"
	"github.com/jmoiron/sqlx"
	"github.com/kubearchive/kubearchive/pkg/database/facade"
)

func init() {
	RegisteredDatabases["mariadb"] = NewMariaDBDatabase
	RegisteredDBCreators["mariadb"] = NewMariaDBCreator
}

type MariaDBDatabaseCreator struct {
	env map[string]string
}

func NewMariaDBCreator(env map[string]string) facade.DBCreator {
	return &MariaDBDatabaseCreator{env: env}
}

func (creator MariaDBDatabaseCreator) GetDriverName() string {
	return "mysql"
}

func (creator MariaDBDatabaseCreator) GetConnectionString() string {
	return fmt.Sprintf("%s:%s@tcp(%s:%s)/%s", creator.env[DbUserEnvVar],
		creator.env[DbPasswordEnvVar], creator.env[DbHostEnvVar], creator.env[DbPortEnvVar], creator.env[DbNameEnvVar])
}

type MariaDBSelector struct {
	facade.PartialDBSelectorImpl
}

func (MariaDBSelector) ResourceSelector() *sqlbuilder.SelectBuilder {
	sb := sqlbuilder.NewSelectBuilder()
	return sb.Select(
		sb.As("JSON_VALUE(data, '$.metadata.creationTimestamp')", "created_at"),
		"id",
		"data",
	).From("resource")
}

func (MariaDBSelector) OwnedResourceSelector() *sqlbuilder.SelectBuilder {
	sb := sqlbuilder.NewSelectBuilder()
	return sb.Select(
		"uuid",
		"kind",
		sb.As("JSON_VALUE(data, '$.metadata.creationTimestamp')", "created_at"),
	).From("resource")
}

type MariaDBFilter struct {
	facade.PartialDBFilterImpl
}

func (MariaDBFilter) CreationTSAndIDFilter(cond sqlbuilder.Cond, continueDate, continueId string) string {
	return cond.Var(sqlbuilder.Build(
		"(CONVERT(JSON_VALUE(data, '$.metadata.creationTimestamp'), datetime), uuid) < ($?, $?)",
		continueDate, continueId,
	))
}

func (MariaDBFilter) OwnerFilter(cond sqlbuilder.Cond, uuids []string) string {
	return cond.Var(sqlbuilder.Build(
		"JSON_OVERLAPS(JSON_EXTRACT(data, '$.metadata.ownerReferences.**.uid'), JSON_ARRAY($?))",
		sqlbuilder.List(uuids),
	))
}

type MariaDBSorter struct{}

func (MariaDBSorter) CreationTSAndIDSorter(sb *sqlbuilder.SelectBuilder) *sqlbuilder.SelectBuilder {
	return sb.OrderBy("CONVERT(JSON_VALUE(data, '$.metadata.creationTimestamp'), datetime) DESC", "id DESC")
}

type MariaDBInserter struct {
	facade.PartialDBInserterImpl
}

func (MariaDBInserter) ResourceInserter(
	uuid, apiVersion, kind, name, namespace, version string,
	clusterDeletedTs sql.NullString,
	data []byte,
) *sqlbuilder.InsertBuilder {
	ib := sqlbuilder.NewInsertBuilder()
	ib.InsertInto("resource")
	ib.Cols("uuid", "api_version", "kind", "name", "namespace", "resource_version", "cluster_deleted_ts", "data")
	ib.Values(uuid, apiVersion, kind, name, namespace, version, clusterDeletedTs, data)
	ib.SQL(ib.Var(sqlbuilder.Build(
		"ON DUPLICATE KEY UPDATE name=$?, namespace=$?, resource_version=$?, cluster_deleted_ts=$?, data=$?",
		name, namespace, version, clusterDeletedTs, data,
	)))
	return ib
}

type MariaDBDatabase struct {
	*Database
}

func NewMariaDBDatabase(conn *sqlx.DB) DBInterface {
	return MariaDBDatabase{&Database{
		DB:       conn,
		Flavor:   sqlbuilder.MySQL,
		Selector: MariaDBSelector{},
		Filter:   MariaDBFilter{},
		Sorter:   MariaDBSorter{},
		Inserter: MariaDBInserter{},
		Deleter:  facade.DBDeleterImpl{},
	}}
}
