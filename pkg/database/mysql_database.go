// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package database

import (
	"context"
	"database/sql"
	"fmt"

	_ "github.com/go-sql-driver/mysql"
	"github.com/kubearchive/kubearchive/pkg/models"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type MySQLDatabase struct {
	db                       *sql.DB
	resourceTableName        string
	resourcesQuery           string
	namespacedResourcesQuery string
	writeResourceQuery       string
}

func (db *MySQLDatabase) QueryResources(ctx context.Context, kind, group, version string) ([]*unstructured.Unstructured, error) {
	query := fmt.Sprintf(db.resourcesQuery, db.resourceTableName)
	apiVersion := fmt.Sprintf("%s/%s", group, version)
	return performResourceQuery(db.db, ctx, query, kind, apiVersion)
}

func (db *MySQLDatabase) QueryCoreResources(ctx context.Context, kind, version string) ([]*unstructured.Unstructured, error) {
	query := fmt.Sprintf(db.resourcesQuery, db.resourceTableName) //nolint:gosec
	return performResourceQuery(db.db, ctx, query, kind, version)
}

func (db *MySQLDatabase) QueryNamespacedResources(ctx context.Context, kind, group, version, namespace string) ([]*unstructured.Unstructured, error) {
	query := fmt.Sprintf(db.namespacedResourcesQuery, db.resourceTableName) //nolint:gosec
	apiVersion := fmt.Sprintf("%s/%s", group, version)
	return performResourceQuery(db.db, ctx, query, kind, apiVersion, namespace)
}

func (db *MySQLDatabase) QueryNamespacedCoreResources(ctx context.Context, kind, version, namespace string) ([]*unstructured.Unstructured, error) {
	query := fmt.Sprintf(db.namespacedResourcesQuery, db.resourceTableName) //nolint:gosec
	return performResourceQuery(db.db, ctx, query, kind, version, namespace)
}

func (db *MySQLDatabase) Ping(ctx context.Context) error {
	return db.db.PingContext(ctx)
}

func (db *MySQLDatabase) WriteResource(ctx context.Context, k8sObj *unstructured.Unstructured, data []byte) error {
	query := fmt.Sprintf(db.writeResourceQuery, db.resourceTableName)
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

func newMySQLDatabase(db *sql.DB) *MySQLDatabase {
	return &MySQLDatabase{
		db:                       db,
		resourceTableName:        "resource",
		resourcesQuery:           "SELECT data FROM %s WHERE kind=? AND api_version=?",
		namespacedResourcesQuery: "SELECT data FROM %s WHERE kind=? AND api_version=? AND namespace=?",
		writeResourceQuery: "INSERT INTO %s (uuid, api_version, kind, name, namespace, resource_version, cluster_deleted_ts, data) " +
			"VALUES (?, ?, ?, ?, ?, ?, ?, ?) " +
			"ON DUPLICATE KEY UPDATE name=?, namespace=?, resource_version=?, cluster_deleted_ts=?, data=?",
	}
}
