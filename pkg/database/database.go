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
	GetResourcesLimitedSQL() string
	GetResourcesLimitedContinueSQL() string

	GetNamespacedResourcesSQL() string
	GetNamespacedResourcesLimitedSQL() string
	GetNamespacedResourcesLimitedContinueSQL() string

	GetNamespacedResourceByNameSQL() string

	GetWriteResourceSQL() string
}

type DBInterface interface {
	QueryCoreResources(ctx context.Context, kind, version, limit, continueUUID, continueDate string) ([]*unstructured.Unstructured, string, string, error)
	QueryNamespacedCoreResources(ctx context.Context, kind, version, namespace, limit, continueUUID, continueDate string) ([]*unstructured.Unstructured, string, string, error)
	QueryNamespacedCoreResourceByName(ctx context.Context, kind, version, namespace, name string) (*unstructured.Unstructured, error)

	QueryResources(ctx context.Context, kind, group, version, limit, continueUUID, continueDate string) ([]*unstructured.Unstructured, string, string, error)
	QueryNamespacedResources(ctx context.Context, kind, group, version, namespace, limit, continueUUID, continueDate string) ([]*unstructured.Unstructured, string, string, error)
	QueryNamespacedResourceByName(ctx context.Context, kind, group, version, namespace, name string) (*unstructured.Unstructured, error)

	WriteResource(ctx context.Context, k8sObj *unstructured.Unstructured, data []byte) error
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

func (db *Database) QueryResources(ctx context.Context, kind, group, version, limit, continueUUID, continueDate string) ([]*unstructured.Unstructured, string, string, error) {
	apiVersion := fmt.Sprintf("%s/%s", group, version)

	args := []string{kind, apiVersion}
	query := db.info.GetResourcesSQL()
	if limit != "" {
		if continueUUID == "" || continueDate == "" {
			query = db.info.GetResourcesLimitedSQL()
			args = append(args, limit)
		} else {
			query = db.info.GetResourcesLimitedContinueSQL()
			args = append(args, continueDate, continueUUID, limit)
		}
	}

	return db.performResourceQuery(ctx, query, args...)
}

func (db *Database) QueryCoreResources(ctx context.Context, kind, version, limit, continueUUID, continueDate string) ([]*unstructured.Unstructured, string, string, error) {
	args := []string{kind, version}
	query := db.info.GetResourcesSQL()

	if limit != "" {
		if continueUUID == "" || continueDate == "" {
			query = db.info.GetResourcesLimitedSQL()
			args = append(args, limit)
		} else {
			query = db.info.GetResourcesLimitedContinueSQL()
			args = append(args, continueDate, continueUUID, limit)
		}
	}

	return db.performResourceQuery(ctx, query, args...)
}

func (db *Database) QueryNamespacedResources(ctx context.Context, kind, group, version, namespace, limit, continueUUID, continueDate string) ([]*unstructured.Unstructured, string, string, error) {
	apiVersion := fmt.Sprintf("%s/%s", group, version)

	// Build query and args depending on `limit` and `continueValue`
	args := []string{kind, apiVersion, namespace}
	query := db.info.GetNamespacedResourcesSQL()
	if limit != "" {
		if continueUUID == "" || continueDate == "" {
			query = db.info.GetNamespacedResourcesLimitedSQL()
			args = append(args, limit)
		} else {
			query = db.info.GetNamespacedResourcesLimitedContinueSQL()
			args = append(args, continueDate, continueUUID, limit)
		}
	}

	return db.performResourceQuery(ctx, query, args...)
}

func (db *Database) QueryNamespacedResourceByName(ctx context.Context, kind, group, version, namespace, name string) (*unstructured.Unstructured, error) {
	apiVersion := fmt.Sprintf("%s/%s", group, version)
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

func (db *Database) QueryNamespacedCoreResources(ctx context.Context, kind, version, namespace, limit, continueUUID, continueDate string) ([]*unstructured.Unstructured, string, string, error) {
	args := []string{kind, version, namespace}
	query := db.info.GetNamespacedResourcesSQL()

	if limit != "" {
		if continueUUID == "" || continueDate == "" {
			query = db.info.GetNamespacedResourcesLimitedSQL()
			args = append(args, limit)
		} else {
			query = db.info.GetNamespacedResourcesLimitedContinueSQL()
			args = append(args, continueDate, continueUUID, limit)
		}
	}

	return db.performResourceQuery(ctx, query, args...)
}

func (db *Database) QueryNamespacedCoreResourceByName(ctx context.Context, kind, version, namespace, name string) (*unstructured.Unstructured, error) {
	resources, _, _, err := db.performResourceQuery(ctx, db.info.GetNamespacedResourceByNameSQL(), kind, version, namespace, name)
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

func (db *Database) performResourceQuery(ctx context.Context, query string, args ...string) ([]*unstructured.Unstructured, string, string, error) {
	castedArgs := make([]interface{}, len(args))
	for i, v := range args {
		castedArgs[i] = v
	}

	rows, err := db.db.QueryContext(ctx, query, castedArgs...)
	if err != nil {
		return nil, "", "", err
	}
	defer func(rows *sql.Rows) {
		err = rows.Close()
	}(rows)
	var resources []*unstructured.Unstructured
	if err != nil {
		return resources, "", "", err
	}
	var date string
	var uuid string
	for rows.Next() {
		var b sql.RawBytes
		if err := rows.Scan(&date, &uuid, &b); err != nil {
			return resources, "", "", err
		}

		r, err := models.UnstructuredFromByteSlice([]byte(b))
		if err != nil {
			return resources, "", "", err
		}
		resources = append(resources, r)
	}

	return resources, uuid, date, nil
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
func (db *Database) CloseDB() error {
	return db.db.Close()
}
