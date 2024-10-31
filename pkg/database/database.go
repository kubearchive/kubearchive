// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package database

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/kubearchive/kubearchive/pkg/models"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type newDatabaseFunc func(map[string]string) DBInterface

var RegisteredDatabases = make(map[string]newDatabaseFunc)

type DBInfoInterface interface {
	GetDriverName() string
	GetConnectionString() string

	GetResourcesLimitedSQL() string
	GetResourcesLimitedContinueSQL() string

	GetNamespacedResourcesLimitedSQL() string
	GetNamespacedResourcesLimitedContinueSQL() string

	GetNamespacedResourceByNameSQL() string

	GetWriteResourceSQL() string
	GetWriteUrlSQL() string
	GetDeleteUrlsSQL() string
}

type DBInterface interface {
	QueryResources(ctx context.Context, kind, apiVersion, limit, continueId, continueDate string) ([]*unstructured.Unstructured, int64, string, error)
	QueryNamespacedResources(ctx context.Context, kind, apiVersion, namespace, limit, continueId, continueDate string) ([]*unstructured.Unstructured, int64, string, error)
	QueryNamespacedResourceByName(ctx context.Context, kind, apiVersion, namespace, name string) (*unstructured.Unstructured, error)

	WriteResource(ctx context.Context, k8sObj *unstructured.Unstructured, data []byte) error
	WriteUrls(ctx context.Context, k8sObj *unstructured.Unstructured, logs ...models.LogTuple) error
	Ping(ctx context.Context) error
	CloseDB() error
}

type Database struct {
	db   *sql.DB
	info DBInfoInterface
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
	args := []string{kind, apiVersion, limit}
	query := db.info.GetResourcesLimitedSQL()

	if continueId != "" && continueDate != "" {
		query = db.info.GetResourcesLimitedContinueSQL()
		args = []string{kind, apiVersion, continueDate, continueId, limit}
	}

	return db.performResourceQuery(ctx, query, args...)
}

func (db *Database) QueryNamespacedResources(ctx context.Context, kind, apiVersion, namespace, limit, continueId, continueDate string) ([]*unstructured.Unstructured, int64, string, error) {
	args := []string{kind, apiVersion, namespace, limit}
	query := db.info.GetNamespacedResourcesLimitedSQL()

	if continueId != "" && continueDate != "" {
		query = db.info.GetNamespacedResourcesLimitedContinueSQL()
		args = []string{kind, apiVersion, namespace, continueDate, continueId, limit}
	}

	return db.performResourceQuery(ctx, query, args...)
}

func (db *Database) QueryNamespacedResourceByName(ctx context.Context, kind, apiVersion, namespace, name string) (*unstructured.Unstructured, error) {
	resources, _, _, err := db.performResourceQuery(ctx, db.info.GetNamespacedResourceByNameSQL(), kind, apiVersion, namespace, name)
	if err != nil {
		return nil, err
	}

	if len(resources) == 0 {
		return nil, err
	} else if len(resources) == 1 {
		return resources[0], err
	} else {
		return nil, fmt.Errorf("More than one resource found")
	}
}

func (db *Database) performResourceQuery(ctx context.Context, query string, args ...string) ([]*unstructured.Unstructured, int64, string, error) {
	castedArgs := make([]interface{}, len(args))
	for i, v := range args {
		castedArgs[i] = v
	}

	rows, err := db.db.QueryContext(ctx, query, castedArgs...)
	if err != nil {
		return nil, 0, "", err
	}
	defer func(rows *sql.Rows) {
		err = rows.Close()
	}(rows)
	var resources []*unstructured.Unstructured
	if err != nil {
		return resources, 0, "", err
	}
	var date string
	var id int64
	for rows.Next() {
		var b sql.RawBytes
		if err := rows.Scan(&date, &id, &b); err != nil {
			return resources, 0, "", err
		}

		r, err := models.UnstructuredFromByteSlice([]byte(b))
		if err != nil {
			return resources, 0, "", err
		}
		resources = append(resources, r)
	}

	return resources, id, date, nil
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

func (db *Database) CloseDB() error {
	return db.db.Close()
}
