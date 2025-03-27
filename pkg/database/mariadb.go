// Copyright Kronicler Authors
// SPDX-License-Identifier: Apache-2.0

package database

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/huandu/go-sqlbuilder"
	"github.com/jmoiron/sqlx"
	"github.com/kronicler/kronicler/pkg/database/facade"
)

func init() {
	RegisteredDatabases["mariadb"] = newMariaDBDatabase
	RegisteredDBCreators["mariadb"] = newMariaDBCreator
}

type mariaDBDatabaseCreator struct {
	env map[string]string
}

func newMariaDBCreator(env map[string]string) facade.DBCreator {
	return &mariaDBDatabaseCreator{env: env}
}

func (creator mariaDBDatabaseCreator) GetDriverName() string {
	return "mysql"
}

func (creator mariaDBDatabaseCreator) GetConnectionString() string {
	return fmt.Sprintf("%s:%s@tcp(%s:%s)/%s", creator.env[DbUserEnvVar],
		creator.env[DbPasswordEnvVar], creator.env[DbHostEnvVar], creator.env[DbPortEnvVar], creator.env[DbNameEnvVar])
}

type mariaDBSelector struct {
	facade.PartialDBSelectorImpl
}

func (mariaDBSelector) ResourceSelector() *sqlbuilder.SelectBuilder {
	sb := sqlbuilder.NewSelectBuilder()
	return sb.Select(
		sb.As("JSON_VALUE(data, '$.metadata.creationTimestamp')", "created_at"),
		"id",
		"data",
	).From("resource")
}

func (mariaDBSelector) OwnedResourceSelector() *sqlbuilder.SelectBuilder {
	sb := sqlbuilder.NewSelectBuilder()
	return sb.Select(
		"uuid",
		"kind",
		sb.As("JSON_VALUE(data, '$.metadata.creationTimestamp')", "created_at"),
	).From("resource")
}

type mariaDBFilter struct {
	facade.PartialDBFilterImpl
}

func (mariaDBFilter) CreationTSAndIDFilter(cond sqlbuilder.Cond, continueDate, continueId string) string {
	return fmt.Sprintf(
		"(CONVERT(JSON_VALUE(data, '$.metadata.creationTimestamp'), datetime), uuid) < (%s, %s)",
		cond.Var(continueDate), cond.Var(continueId),
	)
}

func (mariaDBFilter) OwnerFilter(cond sqlbuilder.Cond, uuids []string) string {
	return fmt.Sprintf(
		"JSON_OVERLAPS(JSON_EXTRACT(data, '$.metadata.ownerReferences.**.uid'), JSON_ARRAY(%s))",
		cond.Var(sqlbuilder.List(uuids)))
}

func (mariaDBFilter) ExistsLabelFilter(cond sqlbuilder.Cond, labels []string) string {
	// TODO
	return ""
}

func (mariaDBFilter) NotExistsLabelFilter(cond sqlbuilder.Cond, labels []string) string {
	// TODO
	return ""
}

func (mariaDBFilter) EqualsLabelFilter(cond sqlbuilder.Cond, labels map[string]string) string {
	// TODO
	return ""
}

func (mariaDBFilter) NotEqualsLabelFilter(cond sqlbuilder.Cond, labels map[string]string) string {
	// TODO
	return ""
}

func (mariaDBFilter) InLabelFilter(cond sqlbuilder.Cond, labels map[string][]string) string {
	// TODO
	return ""
}

func (mariaDBFilter) NotInLabelFilter(cond sqlbuilder.Cond, labels map[string][]string) string {
	// TODO
	return ""
}

type mariaDBSorter struct{}

func (mariaDBSorter) CreationTSAndIDSorter(sb *sqlbuilder.SelectBuilder) *sqlbuilder.SelectBuilder {
	return sb.OrderBy("CONVERT(JSON_VALUE(data, '$.metadata.creationTimestamp'), datetime) DESC", "id DESC")
}

type mariaDBInserter struct {
	facade.PartialDBInserterImpl
}

func (mariaDBInserter) ResourceInserter(
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
		"ON DUPLICATE KEY UPDATE name=$?, namespace=$?, resource_version=$?, cluster_updated_ts=$?, cluster_deleted_ts=$?, data=$?",
		name, namespace, version, clusterUpdatedTs, clusterDeletedTs, data,
	)))
	ib.SQL(ib.Var(sqlbuilder.Build(
		"WHERE resource.cluster_deleted_ts < $?",
		clusterUpdatedTs,
	)))
	return ib
}

type mariaDBDatabase struct {
	*DatabaseImpl
}

func newMariaDBDatabase(conn *sqlx.DB) Database {
	return mariaDBDatabase{&DatabaseImpl{
		db:       conn,
		flavor:   sqlbuilder.MySQL,
		selector: mariaDBSelector{},
		filter:   mariaDBFilter{},
		sorter:   mariaDBSorter{},
		inserter: mariaDBInserter{},
		deleter:  facade.DBDeleterImpl{},
	}}
}
