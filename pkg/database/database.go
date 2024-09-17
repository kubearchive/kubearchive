// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package database

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"github.com/XSAM/otelsql"
	"github.com/avast/retry-go/v4"
	"github.com/kubearchive/kubearchive/pkg/models"
	_ "github.com/lib/pq"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const (
	resourceTableName        = "kubearchive.resource"
	resourcesQuery           = "SELECT data FROM %s WHERE kind=$1 AND api_version=$2"
	namespacedResourcesQuery = "SELECT data FROM %s WHERE kind=$1 AND api_version=$2 AND namespace=$3"
	writeResource            = `INSERT INTO %s (uuid, api_version, kind, name, namespace, resource_version, cluster_deleted_ts, data) Values ($1, $2, $3, $4, $5, $6, $7, $8) ON CONFLICT(uuid) DO UPDATE SET name=$4, namespace=$5, resource_version=$6, cluster_deleted_ts=$7, data=$8`
)

type DBInterface interface {
	QueryResources(ctx context.Context, kind, group, version string) ([]*unstructured.Unstructured, error)
	QueryCoreResources(ctx context.Context, kind, version string) ([]*unstructured.Unstructured, error)
	QueryNamespacedResources(ctx context.Context, kind, group, version, namespace string) ([]*unstructured.Unstructured, error)
	QueryNamespacedCoreResources(ctx context.Context, kind, version, namespace string) ([]*unstructured.Unstructured, error)
	WriteResource(ctx context.Context, k8sObj *unstructured.Unstructured, data []byte) error
	Ping(ctx context.Context) error
}

type Database struct {
	db                *sql.DB
	resourceTableName string
}

func NewDatabase() (*Database, error) {
	dataSource, err := ConnectionStr()
	if err != nil {
		return nil, err
	}
	var db *sql.DB
	configs := []retry.Option{
		retry.Attempts(10),
		retry.OnRetry(func(n uint, err error) {
			log.Printf("Retry request %d, get error: %v", n+1, err)
		}),
		retry.Delay(time.Second),
	}

	errRetry := retry.Do(
		func() error {
			db, err = otelsql.Open("postgres", dataSource)
			if err != nil {
				return err
			}
			return db.Ping()
		},
		configs...)
	if errRetry != nil {
		return nil, errRetry
	}
	log.Println("Successfully connected to the database")
	return &Database{db, resourceTableName}, nil
}

func (db *Database) Ping(ctx context.Context) error {
	return db.db.PingContext(ctx)
}

func (db *Database) QueryResources(ctx context.Context, kind, group, version string) ([]*unstructured.Unstructured, error) {
	query := fmt.Sprintf(resourcesQuery, db.resourceTableName) //nolint:gosec
	apiVersion := fmt.Sprintf("%s/%s", group, version)
	return db.performResourceQuery(ctx, query, kind, apiVersion)
}

func (db *Database) QueryCoreResources(ctx context.Context, kind, version string) ([]*unstructured.Unstructured, error) {
	query := fmt.Sprintf(resourcesQuery, db.resourceTableName) //nolint:gosec
	return db.performResourceQuery(ctx, query, kind, version)
}

func (db *Database) QueryNamespacedResources(ctx context.Context, kind, group, version, namespace string) ([]*unstructured.Unstructured, error) {
	query := fmt.Sprintf(namespacedResourcesQuery, db.resourceTableName) //nolint:gosec
	apiVersion := fmt.Sprintf("%s/%s", group, version)
	return db.performResourceQuery(ctx, query, kind, apiVersion, namespace)
}

func (db *Database) QueryNamespacedCoreResources(ctx context.Context, kind, version, namespace string) ([]*unstructured.Unstructured, error) {
	query := fmt.Sprintf(namespacedResourcesQuery, db.resourceTableName) //nolint:gosec
	return db.performResourceQuery(ctx, query, kind, version, namespace)
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
	query := fmt.Sprintf(writeResource, db.resourceTableName)
	tx, err := db.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("could not begin transaction for resource %s: %s", k8sObj.GetUID(), err)
	}
	_, execErr := tx.ExecContext(
		ctx,
		query,
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
