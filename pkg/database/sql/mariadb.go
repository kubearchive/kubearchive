// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package sql

import (
	"context"
	"database/sql"
	"errors"
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

func (mariaDBFilter) JsonPathPredicateCheck(cond sqlbuilder.Cond, field, operator, value string) string {
	// Add the $. prefix to create a valid JSONPath expression
	jsonPath := "$." + field

	// Convert = to == for MariaDB
	if operator == "=" {
		operator = "=="
	}

	// Determine if the value should be quoted based on its type
	var quotedValue string
	if isNumeric(value) {
		// Numbers don't need quotes
		quotedValue = value
	} else if isBoolean(value) {
		// Booleans don't need quotes
		quotedValue = value
	} else if isJSONArrayOrObject(value) {
		// JSON arrays/objects don't need quotes
		quotedValue = value
	} else {
		// Strings need to be quoted with single quotes for MariaDB
		quotedValue = fmt.Sprintf("'%s'", value)
	}

	// Use JSON_VALUE with the appropriate comparison operator
	return fmt.Sprintf("JSON_VALUE(data, %s) %s %s", cond.Var(jsonPath), operator, cond.Var(quotedValue))
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
func (db *mariaDBDatabase) WriteResource(
	ctx context.Context,
	k8sObj *unstructured.Unstructured,
	data []byte,
	lastUpdated time.Time,
	jsonPath string,
	logs ...models.LogTuple,
) (interfaces.WriteResourceResult, error) {
	if k8sObj == nil {
		return interfaces.WriteResourceResultError, errors.New("kubernetes object was 'nil', something went wrong")
	}

	tx, err := db.db.BeginTx(ctx, nil)
	if err != nil {
		return interfaces.WriteResourceResultError, fmt.Errorf("could not begin transaction for resource %s: %s", k8sObj.GetUID(), err)
	}

	// First we delete the URLs related with the pod
	if k8sObj.GetKind() == "Pod" {
		delBuilder := db.deleter.UrlDeleter()
		delBuilder.Where(db.filter.UuidFilter(delBuilder.Cond, string(k8sObj.GetUID())))
		query, args := delBuilder.BuildWithFlavor(db.flavor)
		_, execErr := tx.ExecContext(ctx, query, args...)
		if execErr != nil {
			rollbackErr := tx.Rollback()
			if rollbackErr != nil {
				return interfaces.WriteResourceResultError, fmt.Errorf(
					"delete to database failed: %w and unable to roll back transaction: %w",
					execErr,
					rollbackErr,
				)
			}
			return interfaces.WriteResourceResultError, fmt.Errorf("delete to database failed: %w", execErr)
		}

		for _, log := range logs {
			logQuery, logArgs := db.inserter.UrlInserter(
				string(k8sObj.GetUID()),
				log.Url,
				log.ContainerName,
				jsonPath,
			).BuildWithFlavor(db.flavor)
			_, logQueryErr := tx.ExecContext(ctx, logQuery, logArgs...)
			if logQueryErr != nil {
				rollbackErr := tx.Rollback()
				if rollbackErr != nil {
					return interfaces.WriteResourceResultError, fmt.Errorf(
						"write to database failed: %w and unable to roll back transaction: %w",
						execErr,
						rollbackErr,
					)
				}
				return interfaces.WriteResourceResultError, fmt.Errorf("write to database failed: %w", execErr)
			}
		}
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
