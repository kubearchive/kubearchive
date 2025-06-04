// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package sql

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/huandu/go-sqlbuilder"
	"github.com/kubearchive/kubearchive/pkg/database/env"
	"github.com/kubearchive/kubearchive/pkg/database/interfaces"
	"github.com/kubearchive/kubearchive/pkg/database/sql/facade"
	"github.com/kubearchive/kubearchive/pkg/models"
	"github.com/lib/pq"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type postgreSQLCreator struct{}

func (postgreSQLCreator) GetDriverName() string {
	return "postgres"
}

func (creator postgreSQLCreator) GetConnectionString(e map[string]string) string {
	return fmt.Sprintf("user=%s password=%s dbname=%s host=%s port=%s sslmode=require", e[env.DbUserEnvVar],
		e[env.DbPasswordEnvVar], e[env.DbNameEnvVar], e[env.DbHostEnvVar], e[env.DbPortEnvVar])
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

func (postgreSQLFilter) ExistsLabelFilter(cond sqlbuilder.Cond, labels []string, _ *sqlbuilder.WhereClause) string {
	return fmt.Sprintf(
		"data->'metadata'->'labels' ?& %s",
		cond.Var(pq.Array(labels)),
	)
}

func (postgreSQLFilter) NotExistsLabelFilter(cond sqlbuilder.Cond, labels []string, _ *sqlbuilder.WhereClause) string {
	return fmt.Sprintf(
		"((NOT data->'metadata'->'labels' ?| %s) OR data->'metadata'->'labels' IS NULL)",
		cond.Var(pq.Array(labels)),
	)
}

func (postgreSQLFilter) EqualsLabelFilter(cond sqlbuilder.Cond, labels map[string]string, _ *sqlbuilder.WhereClause) string {
	jsonLabels, _ := json.Marshal(labels)
	return fmt.Sprintf(
		"data->'metadata'->'labels' @> %s",
		cond.Var(string(jsonLabels)),
	)
}

func (postgreSQLFilter) NotEqualsLabelFilter(cond sqlbuilder.Cond, labels map[string]string, clause *sqlbuilder.WhereClause) string {
	jsons := make([]string, 0)
	for key, value := range labels {
		jsons = append(jsons, fmt.Sprintf("{\"%s\":\"%s\"}", key, value))
	}

	uuidWithAnyLabelQuery := sqlbuilder.Select("uuid").From("resource")
	uuidWithAnyLabelQuery.AddWhereClause(clause)
	uuidWithAnyLabelQuery.Where(fmt.Sprintf(
		"data->'metadata'->'labels' @> ANY(%s::jsonb[])",
		uuidWithAnyLabelQuery.Var(pq.Array(jsons)),
	))

	return cond.NotIn("uuid", uuidWithAnyLabelQuery)
}

func (postgreSQLFilter) InLabelFilter(cond sqlbuilder.Cond, labels map[string][]string, _ *sqlbuilder.WhereClause) string {
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

func (f postgreSQLFilter) NotInLabelFilter(cond sqlbuilder.Cond, labels map[string][]string, _ *sqlbuilder.WhereClause) string {
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
	return cond.And(f.ExistsLabelFilter(cond, slices.Collect(keys), nil), notContainsClause)
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
	ib.Returning("(xmax = 0) AS inserted")
	return ib
}

type postgreSQLDatabase struct {
	*sqlDatabaseImpl
}

func (db *postgreSQLDatabase) WriteResource(
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

	tx, txErr := db.db.BeginTxx(ctx, nil)
	if txErr != nil {
		return interfaces.WriteResourceResultError, fmt.Errorf("could not begin transaction for resource %s: %s", k8sObj.GetUID(), txErr)
	}

	inserter := db.inserter.ResourceInserter(
		string(k8sObj.GetUID()),
		k8sObj.GetAPIVersion(),
		k8sObj.GetKind(),
		k8sObj.GetName(),
		k8sObj.GetNamespace(),
		k8sObj.GetResourceVersion(),
		lastUpdated,
		models.OptionalTimestamp(k8sObj.GetDeletionTimestamp()),
		data,
	)

	boolQueryPerformer := newQueryPerformer[bool](tx, db.flavor)
	inserted, execErr := boolQueryPerformer.performSingleRowQuery(ctx, inserter)

	if execErr != nil && !errors.Is(execErr, sql.ErrNoRows) {
		rollbackErr := tx.Rollback()
		if rollbackErr != nil {
			return interfaces.WriteResourceResultError, fmt.Errorf("write resource to database failed: %s and unable to roll back transaction: %s", execErr, rollbackErr)
		}
		return interfaces.WriteResourceResultError, fmt.Errorf("write resource to database failed: %s", execErr)
	}

	if k8sObj.GetKind() == "Pod" {
		delBuilder := db.deleter.UrlDeleter()
		delBuilder.Where(db.filter.UuidFilter(delBuilder.Cond, string(k8sObj.GetUID())))
		query, args := delBuilder.BuildWithFlavor(db.flavor)
		_, delErr := tx.ExecContext(ctx, query, args...)
		if delErr != nil {
			rollbackErr := tx.Rollback()
			if rollbackErr != nil {
				return interfaces.WriteResourceResultError, fmt.Errorf(
					"delete urls from database failed: %w and unable to roll back transaction: %w",
					delErr,
					rollbackErr,
				)
			}
			return interfaces.WriteResourceResultError, fmt.Errorf("delete urls from database failed: %w", delErr)
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
						"write urls to database failed: %w and unable to roll back transaction: %w",
						logQueryErr,
						rollbackErr,
					)
				}
				return interfaces.WriteResourceResultError, fmt.Errorf("write urls to database failed: %w", logQueryErr)
			}
		}
	}

	execErr = tx.Commit()
	if execErr != nil {
		rollbackErr := tx.Rollback()
		if rollbackErr != nil {
			return interfaces.WriteResourceResultError, fmt.Errorf("commit to database failed: %s and unable to roll back transaction: %s", execErr, rollbackErr)
		}
		return interfaces.WriteResourceResultError, fmt.Errorf("commit to database failed and the transactions was rolled back: %s", execErr)
	}

	// This block is "independent" to help with readability
	result := interfaces.WriteResourceResultError
	if errors.Is(execErr, sql.ErrNoRows) {
		result = interfaces.WriteResourceResultNone
	} else if inserted {
		result = interfaces.WriteResourceResultInserted
	} else if !inserted {
		result = interfaces.WriteResourceResultUpdated
	}

	return result, nil
}

func NewPostgreSQLDatabase() *postgreSQLDatabase {
	return &postgreSQLDatabase{&sqlDatabaseImpl{
		flavor:   sqlbuilder.PostgreSQL,
		selector: postgreSQLSelector{},
		filter:   postgreSQLFilter{},
		sorter:   postgreSQLSorter{},
		inserter: postgreSQLInserter{},
		deleter:  facade.DBDeleterImpl{},
		creator:  postgreSQLCreator{},
	}}
}
