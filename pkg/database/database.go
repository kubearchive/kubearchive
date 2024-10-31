// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/kubearchive/kubearchive/pkg/models"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

var ResourceNotFoundError = errors.New("resource not found")

type newDatabaseFunc func(map[string]string) DBInterface

var RegisteredDatabases = make(map[string]newDatabaseFunc)

type DBInfoInterface interface {
	GetDriverName() string
	GetConnectionString() string

	GetUUIDSQL() string

	GetResourcesLimitedSQL() string
	GetResourcesLimitedContinueSQL() string

	GetNamespacedResourcesLimitedSQL() string
	GetNamespacedResourcesLimitedContinueSQL() string

	GetNamespacedResourceByNameSQL() string

	GetLogURLsByPodNameSQL() string
	GetOwnedResourcesSQL() string
	GetLogURLsSQL() string

	GetWriteResourceSQL() string
	GetWriteUrlSQL() string
	GetDeleteUrlsSQL() string
}

// DBParamParser need to be implemented by every database driver compatible with KubeArchive
type DBParamParser interface {
	// ParseParams transform the given args to accepted parameters for parametrized queries
	ParseParams(query string, args ...any) (string, []any, error)
}

type DBInterface interface {
	QueryResources(ctx context.Context, kind, apiVersion, limit, continueId, continueDate string) ([]*unstructured.Unstructured, int64, string, error)
	QueryNamespacedResources(ctx context.Context, kind, apiVersion, namespace, limit, continueId, continueDate string) ([]*unstructured.Unstructured, int64, string, error)
	QueryNamespacedResourceByName(ctx context.Context, kind, apiVersion, namespace, name string) (*unstructured.Unstructured, error)
	QueryLogURLs(ctx context.Context, kind, apiVersion, namespace, name string) ([]string, error)

	WriteResource(ctx context.Context, k8sObj *unstructured.Unstructured, data []byte) error
	WriteUrls(ctx context.Context, k8sObj *unstructured.Unstructured, logs ...models.LogTuple) error
	Ping(ctx context.Context) error
	CloseDB() error
}

type Database struct {
	db          *sql.DB
	info        DBInfoInterface
	paramParser DBParamParser
}

func NewDatabase() (DBInterface, error) {
	env, err := newDatabaseEnvironment()
	if err != nil {
		return nil, err
	}
	var database DBInterface
	if f, ok := RegisteredDatabases[env[DbKindEnvVar]]; ok {
		database = f(env)
	} else {
		panic(fmt.Sprintf("No database registered with name %s", env[DbKindEnvVar]))
	}

	return database, nil
}

func (db *Database) Ping(ctx context.Context) error {
	return db.db.PingContext(ctx)
}

func (db *Database) QueryResources(ctx context.Context, kind, apiVersion, limit, continueId, continueDate string) ([]*unstructured.Unstructured, int64, string, error) {
	var pq *paramQuery

	if continueId != "" && continueDate != "" {
		pq = db.newParamQuery(db.info.GetResourcesLimitedContinueSQL())
		pq.addStringParams(kind, apiVersion, continueDate, continueId, limit)
	} else {
		pq = db.newParamQuery(db.info.GetResourcesLimitedSQL())
		pq.addStringParams(kind, apiVersion, limit)
	}

	return db.performResourceQuery(ctx, pq)
}

func (db *Database) QueryNamespacedResources(ctx context.Context, kind, apiVersion, namespace, limit, continueId, continueDate string) ([]*unstructured.Unstructured, int64, string, error) {
	var pq *paramQuery

	if continueId != "" && continueDate != "" {
		pq = db.newParamQuery(db.info.GetNamespacedResourcesLimitedContinueSQL())
		pq.addStringParams(kind, apiVersion, namespace, continueDate, continueId, limit)
	} else {
		pq = db.newParamQuery(db.info.GetNamespacedResourcesLimitedSQL())
		pq.addStringParams(kind, apiVersion, namespace, limit)
	}

	return db.performResourceQuery(ctx, pq)
}

func (db *Database) QueryNamespacedResourceByName(ctx context.Context, kind, apiVersion, namespace, name string) (*unstructured.Unstructured, error) {
	pq := db.newParamQuery(db.info.GetNamespacedResourceByNameSQL())
	pq.addStringParams(kind, apiVersion, namespace, name)
	resources, _, _, err := db.performResourceQuery(ctx, pq)
	if err != nil {
		return nil, err
	}

	if len(resources) == 0 {
		return nil, err
	} else if len(resources) == 1 {
		return resources[0], err
	} else {
		return nil, fmt.Errorf("more than one resource found")
	}
}

func (db *Database) QueryLogURLs(ctx context.Context, kind, apiVersion, namespace, name string) ([]string, error) {

	strQueryPerformer := newQueryPerformer[string](db.db)
	if kind == "Pod" {
		pq := db.newParamQuery(db.info.GetLogURLsByPodNameSQL())
		pq.addStringParams(apiVersion, namespace, name)
		return strQueryPerformer.performQuery(ctx, pq)
	}

	var uuid string
	err := db.db.QueryRowContext(ctx, db.info.GetUUIDSQL(), kind, apiVersion, namespace, name).Scan(&uuid)

	if errors.Is(err, sql.ErrNoRows) {
		return []string{}, ResourceNotFoundError
	}

	if err != nil {
		return nil, err
	}

	podUuids, err := db.getOwnedPodsUuids(ctx, []string{uuid}, []string{})
	if err != nil {
		return nil, err
	}

	pq := db.newParamQuery(db.info.GetLogURLsSQL())
	pq.addStringArrayParam(podUuids)
	return strQueryPerformer.performQuery(ctx, pq)
}

func (db *Database) getOwnedPodsUuids(ctx context.Context, ownersUuids, podUuids []string) ([]string, error) {

	type uuidKind struct {
		Uuid string `json:"uuid"`
		Kind string `json:"kind"`
	}

	if len(ownersUuids) == 0 {
		return podUuids, nil
	} else {
		pq := db.newParamQuery(db.info.GetOwnedResourcesSQL())
		pq.addStringArrayParam(ownersUuids)
		parsedRows, err := newQueryPerformer[uuidKind](db.db).performQuery(ctx, pq)
		if err != nil {
			return nil, err
		}
		ownersUuids = make([]string, 0)
		for _, row := range parsedRows {
			if row.Kind == "Pod" {
				podUuids = append(podUuids, row.Uuid)
			} else {
				ownersUuids = append(ownersUuids, row.Uuid)
			}
		}
		return db.getOwnedPodsUuids(ctx, ownersUuids, podUuids)
	}
}

func (db *Database) performResourceQuery(ctx context.Context, pq *paramQuery) ([]*unstructured.Unstructured, int64, string, error) {
	type resourceFields struct {
		Date  string       `json:"created_at"`
		Id    int64        `json:"id"`
		Bytes sql.RawBytes `json:"data"`
	}

	var resources []*unstructured.Unstructured

	parsedRows, err := newQueryPerformer[resourceFields](db.db).performQuery(ctx, pq)

	if err != nil {
		return resources, 0, "", err
	}

	if len(parsedRows) == 0 {
		return resources, 0, "", err
	}
	lastRow := parsedRows[len(parsedRows)-1]

	for _, parsedRow := range parsedRows {
		r, err := models.UnstructuredFromByteSlice([]byte(parsedRow.Bytes))
		if err != nil {
			return resources, 0, "", err
		}
		resources = append(resources, r)
	}
	return resources, lastRow.Id, lastRow.Date, nil
}

func (db *Database) WriteResource(ctx context.Context, k8sObj *unstructured.Unstructured, data []byte) error {
	tx, err := db.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("could not begin transaction for resource %s: %s", k8sObj.GetUID(), err)
	}
	_, execErr := tx.ExecContext(
		ctx,
		db.info.GetWriteResourceSQL(),
		k8sObj.GetUID(),
		k8sObj.GetAPIVersion(),
		k8sObj.GetKind(),
		k8sObj.GetName(),
		k8sObj.GetNamespace(),
		k8sObj.GetResourceVersion(),
		models.OptionalTimestamp(k8sObj.GetDeletionTimestamp()),
		data,
	)
	if execErr != nil {
		rollbackErr := tx.Rollback()
		if rollbackErr != nil {
			return fmt.Errorf("write to database failed: %s and unable to roll back transaction: %s", execErr, rollbackErr)
		}
		return fmt.Errorf("write to database failed: %s", execErr)
	}
	execErr = tx.Commit()
	if execErr != nil {
		rollbackErr := tx.Rollback()
		if rollbackErr != nil {
			return fmt.Errorf("commit to database failed: %s and unable to roll back transaction: %s", execErr, rollbackErr)
		}
		return fmt.Errorf("commit to database failed and the transactions was rolled back: %s", execErr)
	}
	return nil
}

// WriteUrls deletes urls for k8sObj before writing urls to prevent duplicates
func (db *Database) WriteUrls(ctx context.Context, k8sObj *unstructured.Unstructured, logs ...models.LogTuple) error {
	tx, err := db.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf(
			"could not begin transaction to write urls for resource %s: %w",
			k8sObj.GetUID(),
			err,
		)
	}
	_, execErr := tx.ExecContext(
		ctx,
		db.info.GetDeleteUrlsSQL(),
		k8sObj.GetUID(),
	)
	if execErr != nil {
		rollbackErr := tx.Rollback()
		if rollbackErr != nil {
			return fmt.Errorf(
				"delete to database failed: %w and unable to roll back transaction: %w",
				execErr,
				rollbackErr,
			)
		}
		return fmt.Errorf("delete to database failed: %w", execErr)
	}

	for _, log := range logs {
		_, execErr := tx.ExecContext(
			ctx,
			db.info.GetWriteUrlSQL(),
			k8sObj.GetUID(),
			log.Url,
			log.ContainerName,
		)
		if execErr != nil {
			rollbackErr := tx.Rollback()
			if rollbackErr != nil {
				return fmt.Errorf(
					"write to database failed: %w and unable to roll back transaction: %w",
					execErr,
					rollbackErr,
				)
			}
			return fmt.Errorf("write to database failed: %w", execErr)
		}
	}
	commitErr := tx.Commit()
	if commitErr != nil {
		rollbackErr := tx.Rollback()
		if rollbackErr != nil {
			return fmt.Errorf(
				"commit to database failed: %w and unable rollback transaction: %w",
				commitErr,
				rollbackErr,
			)
		}
		return fmt.Errorf("commit to database failed and the transaction was rolled back: %w", commitErr)
	}
	return nil
}

func (db *Database) newParamQuery(query string) *paramQuery {
	return &paramQuery{query: query, dbArrayParser: db.paramParser}
}

func (db *Database) CloseDB() error {
	return db.db.Close()
}
