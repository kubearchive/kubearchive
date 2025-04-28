// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package sql

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/huandu/go-sqlbuilder"
	"github.com/kubearchive/kubearchive/pkg/database/env"
	"github.com/kubearchive/kubearchive/pkg/database/interfaces"
	"github.com/kubearchive/kubearchive/pkg/database/sql/facade"
	"github.com/kubearchive/kubearchive/pkg/models"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type mariaDBDatabaseCreator struct{}

func (mariaDBDatabaseCreator) GetDriverName() string {
	return "mysql"
}

func (mariaDBDatabaseCreator) GetConnectionString(e map[string]string) string {
	return fmt.Sprintf("%s:%s@tcp(%s:%s)/%s", e[env.DbUserEnvVar],
		e[env.DbPasswordEnvVar], e[env.DbHostEnvVar], e[env.DbPortEnvVar], e[env.DbNameEnvVar])
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

func (mariaDBFilter) ExistsLabelFilter(cond sqlbuilder.Cond, labels []string, clause *sqlbuilder.WhereClause) string {
	// TODO
	return ""
}

func (mariaDBFilter) NotExistsLabelFilter(cond sqlbuilder.Cond, labels []string, clause *sqlbuilder.WhereClause) string {
	// TODO
	return ""
}

func (mariaDBFilter) EqualsLabelFilter(cond sqlbuilder.Cond, labels map[string]string, clause *sqlbuilder.WhereClause) string {
	// TODO
	return ""
}

func (mariaDBFilter) NotEqualsLabelFilter(cond sqlbuilder.Cond, labels map[string]string, clause *sqlbuilder.WhereClause) string {
	// TODO
	return ""
}

func (mariaDBFilter) InLabelFilter(cond sqlbuilder.Cond, labels map[string][]string, clause *sqlbuilder.WhereClause) string {
	// TODO
	return ""
}

func (mariaDBFilter) NotInLabelFilter(cond sqlbuilder.Cond, labels map[string][]string, clause *sqlbuilder.WhereClause) string {
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
	*sqlDatabaseImpl
}

// FIXME: the WriteResourceResult return value must indicate if the query resulted in an insertion, update, nothing or an error
func (db *mariaDBDatabase) WriteResource(ctx context.Context, k8sObj *unstructured.Unstructured, data []byte, lastUpdated time.Time) (interfaces.WriteResourceResult, error) {
	tx, err := db.db.BeginTx(ctx, nil)
	if err != nil {
		return interfaces.WriteResourceResultError, fmt.Errorf("could not begin transaction for resource %s: %s", k8sObj.GetUID(), err)
	}

	query, args := db.inserter.ResourceInserter(
		string(k8sObj.GetUID()),
		k8sObj.GetAPIVersion(),
		k8sObj.GetKind(),
		k8sObj.GetName(),
		k8sObj.GetNamespace(),
		k8sObj.GetResourceVersion(),
		lastUpdated,
		models.OptionalTimestamp(k8sObj.GetDeletionTimestamp()),
		data,
	).BuildWithFlavor(db.flavor)

	_, execErr := tx.ExecContext(
		ctx,
		query,
		args...,
	)
	if execErr != nil {
		rollbackErr := tx.Rollback()
		if rollbackErr != nil {
			return interfaces.WriteResourceResultError, fmt.Errorf("write to database failed: %s and unable to roll back transaction: %s", execErr, rollbackErr)
		}
		return interfaces.WriteResourceResultError, fmt.Errorf("write to database failed: %s", execErr)
	}

	execErr = tx.Commit()
	if execErr != nil {
		rollbackErr := tx.Rollback()
		if rollbackErr != nil {
			return interfaces.WriteResourceResultError, fmt.Errorf("commit to database failed: %s and unable to roll back transaction: %s", execErr, rollbackErr)
		}
		return interfaces.WriteResourceResultError, fmt.Errorf("commit to database failed and the transactions was rolled back: %s", execErr)
	}

	return interfaces.WriteResourceResultInserted, nil
}

func NewMariaDBDatabase() *mariaDBDatabase {
	return &mariaDBDatabase{&sqlDatabaseImpl{
		flavor:   sqlbuilder.MySQL,
		selector: mariaDBSelector{},
		filter:   mariaDBFilter{},
		sorter:   mariaDBSorter{},
		inserter: mariaDBInserter{},
		deleter:  facade.DBDeleterImpl{},
		creator:  mariaDBDatabaseCreator{},
	}}
}
