// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package sql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
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
		sb.As("resource.data->'metadata'->>'creationTimestamp'", "created_at"),
		"resource.id",
		"resource.uuid",
		"resource.data",
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
		"(resource.data->'metadata'->>'creationTimestamp', resource.id) < (%s, %s)",
		cond.Var(continueDate), cond.Var(continueId),
	)
}

func (postgreSQLFilter) CreationTimestampAfterFilter(cond sqlbuilder.Cond, timestamp time.Time) string {
	return fmt.Sprintf(
		"resource.data->'metadata'->>'creationTimestamp' > %s",
		cond.Var(timestamp.Format(time.RFC3339)),
	)
}

func (postgreSQLFilter) CreationTimestampBeforeFilter(cond sqlbuilder.Cond, timestamp time.Time) string {
	return fmt.Sprintf(
		"resource.data->'metadata'->>'creationTimestamp' < %s",
		cond.Var(timestamp.Format(time.RFC3339)),
	)
}

func (postgreSQLFilter) NameWildcardFilter(cond sqlbuilder.Cond, namePattern string) string {
	return fmt.Sprintf("name ILIKE %s", cond.Var(namePattern))
}

func (postgreSQLFilter) OwnerFilter(cond sqlbuilder.Cond, owners []string) string {
	jsons := make([]string, 0, len(owners))
	for _, owner := range owners {
		jsons = append(jsons, fmt.Sprintf("[{\"uid\":\"%s\"}]", owner))
	}
	return fmt.Sprintf(
		"(resource.data->'metadata'->'ownerReferences') @> ANY(%s::jsonb[])",
		cond.Var(pq.Array(jsons)),
	)
}

// Helper methods for building positive filter conditions using direct JOINs

func (postgreSQLFilter) addExistsWhereCondition(sb *sqlbuilder.SelectBuilder, keys []string) string {
	keyVars := make([]string, len(keys))
	for i, key := range keys {
		keyVars[i] = sb.Var(key)
	}
	return fmt.Sprintf("lk.key IN (%s)", strings.Join(keyVars, ", "))
}

func (postgreSQLFilter) addEqualsWhereConditions(sb *sqlbuilder.SelectBuilder, labels map[string]string) []string {
	keys := slices.Sorted(maps.Keys(labels))
	conditions := make([]string, 0, len(keys))
	for _, key := range keys {
		value := labels[key]
		conditions = append(conditions, fmt.Sprintf(
			"(lk.key = %s AND lv.value = %s)",
			sb.Var(key),
			sb.Var(value),
		))
	}
	return conditions
}

func (postgreSQLFilter) addInWhereConditions(sb *sqlbuilder.SelectBuilder, labels map[string][]string) []string {
	keys := slices.Sorted(maps.Keys(labels))
	conditions := make([]string, 0, len(keys))
	for _, key := range keys {
		values := labels[key]
		valueVars := make([]string, len(values))
		for i, v := range values {
			valueVars[i] = sb.Var(v)
		}
		conditions = append(conditions, fmt.Sprintf(
			"(lk.key = %s AND lv.value IN (%s))",
			sb.Var(key),
			strings.Join(valueVars, ", "),
		))
	}
	return conditions
}

func (postgreSQLFilter) addExistsHavingCondition(sb *sqlbuilder.SelectBuilder, keys []string) string {
	keyVars := make([]string, len(keys))
	for i, key := range keys {
		keyVars[i] = sb.Var(key)
	}
	return fmt.Sprintf(
		"COUNT(DISTINCT CASE WHEN lk.key IN (%s) THEN lk.key END) = %d",
		strings.Join(keyVars, ", "),
		len(keys),
	)
}

func (postgreSQLFilter) addEqualsHavingConditions(sb *sqlbuilder.SelectBuilder, labels map[string]string) []string {
	keys := slices.Sorted(maps.Keys(labels))
	conditions := make([]string, 0, len(keys))
	for _, key := range keys {
		value := labels[key]
		conditions = append(conditions, fmt.Sprintf(
			"COUNT(CASE WHEN lk.key = %s AND lv.value = %s THEN 1 END) >= 1",
			sb.Var(key),
			sb.Var(value),
		))
	}
	return conditions
}

func (postgreSQLFilter) addInHavingConditions(sb *sqlbuilder.SelectBuilder, labels map[string][]string) []string {
	keys := slices.Sorted(maps.Keys(labels))
	conditions := make([]string, 0, len(keys))
	for _, key := range keys {
		values := labels[key]
		valueVars := make([]string, len(values))
		for i, v := range values {
			valueVars[i] = sb.Var(v)
		}
		conditions = append(conditions, fmt.Sprintf(
			"COUNT(CASE WHEN lk.key = %s AND lv.value IN (%s) THEN 1 END) >= 1",
			sb.Var(key),
			strings.Join(valueVars, ", "),
		))
	}
	return conditions
}

func (f postgreSQLFilter) applyPositiveFiltersWithJoins(sb *sqlbuilder.SelectBuilder, labelFilters *models.LabelFilters) {
	// Add JOINs to normalized label tables
	sb.Join("resource_label rl", "rl.resource_id = resource.id")
	sb.Join("label_pair lp", "lp.id = rl.pair_id")
	sb.Join("label_key lk", "lk.id = lp.key_id")
	sb.Join("label_value lv", "lv.id = lp.value_id")

	// Build WHERE conditions for label matching
	var whereConditions []string
	if len(labelFilters.Exists) > 0 {
		whereConditions = append(whereConditions, f.addExistsWhereCondition(sb, labelFilters.Exists))
	}
	if len(labelFilters.Equals) > 0 {
		whereConditions = append(whereConditions, f.addEqualsWhereConditions(sb, labelFilters.Equals)...)
	}
	if len(labelFilters.In) > 0 {
		whereConditions = append(whereConditions, f.addInWhereConditions(sb, labelFilters.In)...)
	}
	sb.Where(sb.Or(whereConditions...))

	// GROUP BY resource primary key (PostgreSQL allows this without listing all columns)
	sb.GroupBy("resource.id")

	// Build HAVING conditions to ensure ALL required labels are present
	var havingConditions []string
	if len(labelFilters.Exists) > 0 {
		havingConditions = append(havingConditions, f.addExistsHavingCondition(sb, labelFilters.Exists))
	}
	if len(labelFilters.Equals) > 0 {
		havingConditions = append(havingConditions, f.addEqualsHavingConditions(sb, labelFilters.Equals)...)
	}
	if len(labelFilters.In) > 0 {
		havingConditions = append(havingConditions, f.addInHavingConditions(sb, labelFilters.In)...)
	}
	if len(havingConditions) > 0 {
		sb.Having(sb.And(havingConditions...))
	}
}

// Helper methods for building negative filter conditions

func (postgreSQLFilter) applyNotExistsFilter(sb *sqlbuilder.SelectBuilder, keys []string) {
	keyVars := make([]string, len(keys))
	for i, key := range keys {
		keyVars[i] = sb.Var(key)
	}
	notExistsSubquery := fmt.Sprintf(
		"NOT EXISTS (SELECT 1 FROM resource_label rl "+
			"JOIN label_pair lp ON lp.id = rl.pair_id "+
			"JOIN label_key lk ON lk.id = lp.key_id "+
			"WHERE rl.resource_id = resource.id AND lk.key IN (%s))",
		strings.Join(keyVars, ", "),
	)
	sb.Where(notExistsSubquery)
}

func (postgreSQLFilter) applyNotEqualsFilter(sb *sqlbuilder.SelectBuilder, labels map[string]string) {
	keys := slices.Sorted(maps.Keys(labels))
	pairConditions := make([]string, 0, len(keys))
	for _, key := range keys {
		value := labels[key]
		pairConditions = append(pairConditions, fmt.Sprintf(
			"(lk.key = %s AND lv.value = %s)",
			sb.Var(key),
			sb.Var(value),
		))
	}
	notEqualsSubquery := fmt.Sprintf(
		"NOT EXISTS (SELECT 1 FROM resource_label rl "+
			"JOIN label_pair lp ON lp.id = rl.pair_id "+
			"JOIN label_key lk ON lk.id = lp.key_id "+
			"JOIN label_value lv ON lv.id = lp.value_id "+
			"WHERE rl.resource_id = resource.id AND (%s))",
		sb.Or(pairConditions...),
	)
	sb.Where(notEqualsSubquery)
}

func (postgreSQLFilter) applyNotInFilter(sb *sqlbuilder.SelectBuilder, labels map[string][]string) {
	keys := slices.Sorted(maps.Keys(labels))

	// First ensure all keys exist
	keyVars := make([]string, len(keys))
	for i, key := range keys {
		keyVars[i] = sb.Var(key)
	}
	existsSubquery := fmt.Sprintf(
		"EXISTS (SELECT 1 FROM resource_label rl "+
			"JOIN label_pair lp ON lp.id = rl.pair_id "+
			"JOIN label_key lk ON lk.id = lp.key_id "+
			"WHERE rl.resource_id = resource.id AND lk.key IN (%s) "+
			"GROUP BY rl.resource_id "+
			"HAVING COUNT(DISTINCT lk.key) = %d)",
		strings.Join(keyVars, ", "),
		len(keys),
	)
	sb.Where(existsSubquery)

	// Then ensure none of the forbidden values exist
	keyValueConditions := make([]string, 0, len(labels))
	for _, key := range keys {
		values := labels[key]
		valueVars := make([]string, len(values))
		for i, v := range values {
			valueVars[i] = sb.Var(v)
		}
		keyValueConditions = append(keyValueConditions, fmt.Sprintf(
			"(lk.key = %s AND lv.value IN (%s))",
			sb.Var(key),
			strings.Join(valueVars, ", "),
		))
	}
	notInSubquery := fmt.Sprintf(
		"NOT EXISTS (SELECT 1 FROM resource_label rl "+
			"JOIN label_pair lp ON lp.id = rl.pair_id "+
			"JOIN label_key lk ON lk.id = lp.key_id "+
			"JOIN label_value lv ON lv.id = lp.value_id "+
			"WHERE rl.resource_id = resource.id AND (%s))",
		sb.Or(keyValueConditions...),
	)
	sb.Where(notInSubquery)
}

// ApplyLabelFilters applies all label filters using normalized tables with optimal performance
func (f postgreSQLFilter) ApplyLabelFilters(sb *sqlbuilder.SelectBuilder, labelFilters *models.LabelFilters) {
	// Apply positive filters (Exists, Equals, In) using direct JOINs with GROUP BY/HAVING
	hasPositiveFilters := len(labelFilters.Exists) > 0 || len(labelFilters.Equals) > 0 || len(labelFilters.In) > 0
	if hasPositiveFilters {
		f.applyPositiveFiltersWithJoins(sb, labelFilters)
	}

	// Apply negative filters using NOT EXISTS subqueries
	if len(labelFilters.NotExists) > 0 {
		f.applyNotExistsFilter(sb, labelFilters.NotExists)
	}
	if len(labelFilters.NotEquals) > 0 {
		f.applyNotEqualsFilter(sb, labelFilters.NotEquals)
	}
	if len(labelFilters.NotIn) > 0 {
		f.applyNotInFilter(sb, labelFilters.NotIn)
	}
}

type postgreSQLSorter struct{}

func (postgreSQLSorter) CreationTSAndIDSorter(sb *sqlbuilder.SelectBuilder) *sqlbuilder.SelectBuilder {
	return sb.OrderByDesc("resource.data->'metadata'->>'creationTimestamp'").OrderByDesc("resource.id")
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
