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
	"github.com/jmoiron/sqlx"
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

// resolveKeyIDs batch-resolves label key names to their integer IDs.
func resolveKeyIDs(ctx context.Context, querier sqlx.QueryerContext, keys []string) (map[string]int64, error) {
	if len(keys) == 0 {
		return nil, nil
	}

	query, args, err := sqlx.In("SELECT id, key FROM label_key WHERE key IN (?)", keys)
	if err != nil {
		return nil, fmt.Errorf("building key resolution query: %w", err)
	}
	query = sqlx.Rebind(sqlx.DOLLAR, query)

	rows, err := querier.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("resolving label key IDs: %w", err)
	}
	defer rows.Close()

	result := make(map[string]int64, len(keys))
	for rows.Next() {
		var id int64
		var key string
		if err := rows.Scan(&id, &key); err != nil {
			return nil, fmt.Errorf("scanning label key row: %w", err)
		}
		result[key] = id
	}
	return result, rows.Err()
}

// resolveLabelIDs batch-resolves key=value label pairs to their label_key_value.id.
func resolveLabelIDs(ctx context.Context, querier sqlx.QueryerContext, labels map[string]string) (map[string]int64, error) {
	if len(labels) == 0 {
		return nil, nil
	}

	keys := slices.Sorted(maps.Keys(labels))
	tupleFragments := make([]string, 0, len(keys))
	args := make([]any, 0, len(keys)*2)
	for i, key := range keys {
		tupleFragments = append(tupleFragments, fmt.Sprintf("($%d, $%d)", i*2+1, i*2+2))
		args = append(args, key, labels[key])
	}

	query := fmt.Sprintf(
		"SELECT lkv.id, lk.key, lv.value FROM label_key_value lkv "+
			"JOIN label_key lk ON lk.id = lkv.key_id "+
			"JOIN label_value lv ON lv.id = lkv.value_id "+
			"WHERE (lk.key, lv.value) IN (%s)",
		strings.Join(tupleFragments, ", "),
	)

	rows, err := querier.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("resolving label IDs: %w", err)
	}
	defer rows.Close()

	result := make(map[string]int64, len(labels))
	for rows.Next() {
		var id int64
		var key, value string
		if err := rows.Scan(&id, &key, &value); err != nil {
			return nil, fmt.Errorf("scanning label ID row: %w", err)
		}
		result[key+"="+value] = id
	}
	return result, rows.Err()
}

// resolveLabelIDsMulti batch-resolves key with multiple values to their label_key_value.id.
func resolveLabelIDsMulti(ctx context.Context, querier sqlx.QueryerContext, labels map[string][]string) (map[string]int64, error) {
	if len(labels) == 0 {
		return nil, nil
	}

	keys := slices.Sorted(maps.Keys(labels))
	tupleFragments := make([]string, 0)
	args := make([]any, 0)
	paramIdx := 1
	for _, key := range keys {
		for _, value := range labels[key] {
			tupleFragments = append(tupleFragments, fmt.Sprintf("($%d, $%d)", paramIdx, paramIdx+1))
			args = append(args, key, value)
			paramIdx += 2
		}
	}

	query := fmt.Sprintf(
		"SELECT lkv.id, lk.key, lv.value FROM label_key_value lkv "+
			"JOIN label_key lk ON lk.id = lkv.key_id "+
			"JOIN label_value lv ON lv.id = lkv.value_id "+
			"WHERE (lk.key, lv.value) IN (%s)",
		strings.Join(tupleFragments, ", "),
	)

	rows, err := querier.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("resolving label IDs (multi): %w", err)
	}
	defer rows.Close()

	result := make(map[string]int64)
	for rows.Next() {
		var id int64
		var key, value string
		if err := rows.Scan(&id, &key, &value); err != nil {
			return nil, fmt.Errorf("scanning label ID row: %w", err)
		}
		result[key+"="+value] = id
	}
	return result, rows.Err()
}

// Helper methods for building positive filter conditions using pre-resolved IDs

func (postgreSQLFilter) applyExistsFilter(sb *sqlbuilder.SelectBuilder, keyIDs map[string]int64, keys []string) {
	for _, key := range slices.Sorted(slices.Values(keys)) {
		keyID := keyIDs[key]
		existsSubquery := fmt.Sprintf(
			"EXISTS (SELECT 1 FROM resource_label rl "+
				"JOIN label_key_value lkv ON lkv.id = rl.label_id "+
				"WHERE rl.resource_id = resource.id AND lkv.key_id = %s)",
			sb.Var(keyID),
		)
		sb.Where(existsSubquery)
	}
}

func (postgreSQLFilter) applyEqualsFilter(sb *sqlbuilder.SelectBuilder, labelIDs map[string]int64, labels map[string]string) {
	for _, key := range slices.Sorted(maps.Keys(labels)) {
		value := labels[key]
		labelID := labelIDs[key+"="+value]
		existsSubquery := fmt.Sprintf(
			"EXISTS (SELECT 1 FROM resource_label rl "+
				"WHERE rl.resource_id = resource.id AND rl.label_id = %s)",
			sb.Var(labelID),
		)
		sb.Where(existsSubquery)
	}
}

func (postgreSQLFilter) applyInFilter(sb *sqlbuilder.SelectBuilder, labelIDs map[string]int64, labels map[string][]string) {
	for _, key := range slices.Sorted(maps.Keys(labels)) {
		values := labels[key]
		idVars := make([]string, 0, len(values))
		for _, v := range values {
			if id, ok := labelIDs[key+"="+v]; ok {
				idVars = append(idVars, sb.Var(id))
			}
		}
		existsSubquery := fmt.Sprintf(
			"EXISTS (SELECT 1 FROM resource_label rl "+
				"WHERE rl.resource_id = resource.id AND rl.label_id IN (%s))",
			strings.Join(idVars, ", "),
		)
		sb.Where(existsSubquery)
	}
}

// Helper methods for building negative filter conditions

func (postgreSQLFilter) applyNotExistsFilter(sb *sqlbuilder.SelectBuilder, keyIDs map[string]int64, keys []string) {
	idVars := make([]string, 0, len(keys))
	for _, key := range keys {
		if id, ok := keyIDs[key]; ok {
			idVars = append(idVars, sb.Var(id))
		}
	}
	if len(idVars) == 0 {
		return
	}
	notExistsSubquery := fmt.Sprintf(
		"NOT EXISTS (SELECT 1 FROM resource_label rl "+
			"JOIN label_key_value lkv ON lkv.id = rl.label_id "+
			"WHERE rl.resource_id = resource.id AND lkv.key_id IN (%s))",
		strings.Join(idVars, ", "),
	)
	sb.Where(notExistsSubquery)
}

func (postgreSQLFilter) applyNotEqualsFilter(sb *sqlbuilder.SelectBuilder, labelIDs map[string]int64, labels map[string]string) {
	idVars := make([]string, 0, len(labels))
	for _, key := range slices.Sorted(maps.Keys(labels)) {
		value := labels[key]
		if id, ok := labelIDs[key+"="+value]; ok {
			idVars = append(idVars, sb.Var(id))
		}
	}
	if len(idVars) == 0 {
		return
	}
	notEqualsSubquery := fmt.Sprintf(
		"NOT EXISTS (SELECT 1 FROM resource_label rl "+
			"WHERE rl.resource_id = resource.id AND rl.label_id IN (%s))",
		strings.Join(idVars, ", "),
	)
	sb.Where(notEqualsSubquery)
}

func (postgreSQLFilter) applyNotInFilter(sb *sqlbuilder.SelectBuilder, keyIDs map[string]int64, labelIDs map[string]int64, labels map[string][]string) {
	keys := slices.Sorted(maps.Keys(labels))

	// Ensure all keys exist using pre-resolved key IDs
	keyIDVars := make([]string, 0, len(keys))
	for _, key := range keys {
		if id, ok := keyIDs[key]; ok {
			keyIDVars = append(keyIDVars, sb.Var(id))
		}
	}
	existsSubquery := fmt.Sprintf(
		"EXISTS (SELECT 1 FROM resource_label rl "+
			"JOIN label_key_value lkv ON lkv.id = rl.label_id "+
			"WHERE rl.resource_id = resource.id AND lkv.key_id IN (%s) "+
			"GROUP BY rl.resource_id "+
			"HAVING COUNT(DISTINCT lkv.key_id) = %d)",
		strings.Join(keyIDVars, ", "),
		len(keys),
	)
	sb.Where(existsSubquery)

	// Exclude forbidden labels using pre-resolved label IDs
	forbiddenIDVars := make([]string, 0)
	for _, key := range keys {
		for _, value := range labels[key] {
			if id, ok := labelIDs[key+"="+value]; ok {
				forbiddenIDVars = append(forbiddenIDVars, sb.Var(id))
			}
		}
	}
	if len(forbiddenIDVars) > 0 {
		notInSubquery := fmt.Sprintf(
			"NOT EXISTS (SELECT 1 FROM resource_label rl "+
				"WHERE rl.resource_id = resource.id AND rl.label_id IN (%s))",
			strings.Join(forbiddenIDVars, ", "),
		)
		sb.Where(notInSubquery)
	}
}

// ApplyLabelFilters pre-resolves label keys and key-value labels to integer IDs,
// then applies all label filters using EXISTS/NOT EXISTS subqueries with those IDs.
// This lets the PostgreSQL planner use MCV statistics for accurate row estimates.
func (f postgreSQLFilter) ApplyLabelFilters(ctx context.Context, querier sqlx.QueryerContext, sb *sqlbuilder.SelectBuilder, labelFilters *models.LabelFilters) error {
	// Collect all keys that need resolution
	allKeys := make(map[string]struct{})
	for _, k := range labelFilters.Exists {
		allKeys[k] = struct{}{}
	}
	for _, k := range labelFilters.NotExists {
		allKeys[k] = struct{}{}
	}
	for k := range labelFilters.NotIn {
		allKeys[k] = struct{}{}
	}

	// Resolve key IDs (needed for Exists, NotExists, NotIn)
	var keyIDs map[string]int64
	if len(allKeys) > 0 {
		keySlice := slices.Sorted(maps.Keys(allKeys))
		var err error
		keyIDs, err = resolveKeyIDs(ctx, querier, keySlice)
		if err != nil {
			return err
		}
	}

	// Resolve label IDs for Equals/NotEquals (single-value)
	allEqualsLabels := make(map[string]string)
	maps.Copy(allEqualsLabels, labelFilters.Equals)
	maps.Copy(allEqualsLabels, labelFilters.NotEquals)

	var labelIDs map[string]int64
	if len(allEqualsLabels) > 0 {
		var err error
		labelIDs, err = resolveLabelIDs(ctx, querier, allEqualsLabels)
		if err != nil {
			return err
		}
	}

	// Resolve label IDs for In/NotIn (multi-value)
	allMultiLabels := make(map[string][]string)
	for k, v := range labelFilters.In {
		allMultiLabels[k] = append(allMultiLabels[k], v...)
	}
	for k, v := range labelFilters.NotIn {
		allMultiLabels[k] = append(allMultiLabels[k], v...)
	}

	var multiLabelIDs map[string]int64
	if len(allMultiLabels) > 0 {
		var err error
		multiLabelIDs, err = resolveLabelIDsMulti(ctx, querier, allMultiLabels)
		if err != nil {
			return err
		}
	}

	// Apply positive filters with short-circuit on missing keys/labels

	if len(labelFilters.Exists) > 0 {
		for _, key := range labelFilters.Exists {
			if _, ok := keyIDs[key]; !ok {
				sb.Where("1=0")
				return nil
			}
		}
		f.applyExistsFilter(sb, keyIDs, labelFilters.Exists)
	}

	if len(labelFilters.Equals) > 0 {
		for _, key := range slices.Sorted(maps.Keys(labelFilters.Equals)) {
			value := labelFilters.Equals[key]
			if _, ok := labelIDs[key+"="+value]; !ok {
				sb.Where("1=0")
				return nil
			}
		}
		f.applyEqualsFilter(sb, labelIDs, labelFilters.Equals)
	}

	if len(labelFilters.In) > 0 {
		for _, key := range slices.Sorted(maps.Keys(labelFilters.In)) {
			hasAny := false
			for _, v := range labelFilters.In[key] {
				if _, ok := multiLabelIDs[key+"="+v]; ok {
					hasAny = true
					break
				}
			}
			if !hasAny {
				sb.Where("1=0")
				return nil
			}
		}
		f.applyInFilter(sb, multiLabelIDs, labelFilters.In)
	}

	// Apply negative filters — missing keys/labels are no-ops (trivially true)

	if len(labelFilters.NotExists) > 0 {
		f.applyNotExistsFilter(sb, keyIDs, labelFilters.NotExists)
	}

	if len(labelFilters.NotEquals) > 0 {
		f.applyNotEqualsFilter(sb, labelIDs, labelFilters.NotEquals)
	}

	if len(labelFilters.NotIn) > 0 {
		allKeysFound := true
		for _, key := range slices.Sorted(maps.Keys(labelFilters.NotIn)) {
			if _, ok := keyIDs[key]; !ok {
				allKeysFound = false
				break
			}
		}
		if allKeysFound {
			f.applyNotInFilter(sb, keyIDs, multiLabelIDs, labelFilters.NotIn)
		} else {
			sb.Where("1=0")
		}
	}

	return nil
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
