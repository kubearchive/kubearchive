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
	GetResourcesSQL() string
	GetNamespacedResourcesSQL() string
	GetNamespacedResourceByNameSQL() string
	GetWriteResourceSQL() string
}

type DBInterface interface {
	QueryResources(ctx context.Context, kind, group, version string) ([]*unstructured.Unstructured, error)
	QueryCoreResources(ctx context.Context, kind, version string) ([]*unstructured.Unstructured, error)
	QueryNamespacedResources(ctx context.Context, kind, group, version, namespace string) ([]*unstructured.Unstructured, error)
	QueryNamespacedResourceByName(ctx context.Context, kind, group, version, namespace, name string) (*unstructured.Unstructured, error)
	QueryNamespacedCoreResources(ctx context.Context, kind, version, namespace string) ([]*unstructured.Unstructured, error)
	QueryNamespacedCoreResourceByName(ctx context.Context, kind, version, namespace, name string) (*unstructured.Unstructured, error)
	WriteResource(ctx context.Context, k8sObj *unstructured.Unstructured, data []byte) error
	Ping(ctx context.Context) error
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

func (db *Database) QueryResources(ctx context.Context, kind, group, version string) ([]*unstructured.Unstructured, error) {
	apiVersion := fmt.Sprintf("%s/%s", group, version)
	return db.performResourceQuery(ctx, db.info.GetResourcesSQL(), kind, apiVersion)
}

func (db *Database) QueryCoreResources(ctx context.Context, kind, version string) ([]*unstructured.Unstructured, error) {
	return db.performResourceQuery(ctx, db.info.GetResourcesSQL(), kind, version)
}

func (db *Database) QueryNamespacedResources(ctx context.Context, kind, group, version, namespace string) ([]*unstructured.Unstructured, error) {
	apiVersion := fmt.Sprintf("%s/%s", group, version)
	return db.performResourceQuery(ctx, db.info.GetNamespacedResourcesSQL(), kind, apiVersion, namespace)
}

func (db *Database) QueryNamespacedResourceByName(ctx context.Context, kind, group, version, namespace, name string) (*unstructured.Unstructured, error) {
	apiVersion := fmt.Sprintf("%s/%s", group, version)
	resources, err := db.performResourceQuery(ctx, db.info.GetNamespacedResourceByNameSQL(), kind, apiVersion, namespace, name)
	if len(resources) == 0 {
		return nil, err
	} else if len(resources) == 1 {
		return resources[0], err
	} else {
		return nil, fmt.Errorf("More than one resource found")
	}
}

func (db *Database) QueryNamespacedCoreResources(ctx context.Context, kind, version, namespace string) ([]*unstructured.Unstructured, error) {
	return db.performResourceQuery(ctx, db.info.GetNamespacedResourcesSQL(), kind, version, namespace)
}

func (db *Database) QueryNamespacedCoreResourceByName(ctx context.Context, kind, version, namespace, name string) (*unstructured.Unstructured, error) {
	resources, err := db.performResourceQuery(ctx, db.info.GetNamespacedResourceByNameSQL(), kind, version, namespace, name)
	if len(resources) == 0 {
		return nil, err
	} else if len(resources) == 1 {
		return resources[0], err
	} else {
		return nil, fmt.Errorf("More than one resource found")
	}
}

func (db *Database) performResourceQuery(ctx context.Context, query string, args ...string) ([]*unstructured.Unstructured, error) {
	castedArgs := make([]interface{}, len(args))
	for i, v := range args {
		castedArgs[i] = v
	}
	rows, err := db.db.QueryContext(ctx, query, castedArgs...)
	if err != nil {
		return nil, err
	}
	defer func(rows *sql.Rows) {
		err = rows.Close()
	}(rows)
	var resources []*unstructured.Unstructured
	if err != nil {
		return resources, err
	}
	for rows.Next() {
		var b sql.RawBytes
		var r *unstructured.Unstructured
		if err := rows.Scan(&b); err != nil {
			return resources, err
		}
		if r, err = models.UnstructuredFromByteSlice([]byte(b)); err != nil {
			return resources, err
		}
		resources = append(resources, r)
	}
	return resources, err
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
