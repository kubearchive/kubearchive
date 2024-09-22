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

type DBInterface interface {
	DBPing(ctx context.Context) error
	DBQueryResources(ctx context.Context, kind, group, version string) ([]*unstructured.Unstructured, error)
	DBQueryCoreResources(ctx context.Context, kind, version string) ([]*unstructured.Unstructured, error)
	DBQueryNamespacedResources(ctx context.Context, kind, group, version, namespace string) ([]*unstructured.Unstructured, error)
	DBQueryNamespacedCoreResources(ctx context.Context, kind, version, namespace string) ([]*unstructured.Unstructured, error)
	DBWriteResource(ctx context.Context, k8sObj *unstructured.Unstructured, data []byte) error
}

type BaseDatabase struct {
	db   *sql.DB
	info *DatabaseInfo
}

func (db *BaseDatabase) DBPing(ctx context.Context) error {
	return db.db.PingContext(ctx)
}

func (db *BaseDatabase) DBQueryResources(ctx context.Context, kind, group, version string) ([]*unstructured.Unstructured, error) {
	query := fmt.Sprintf(db.info.resourcesQuery, db.info.resourceTableName) //nolint:gosec
	apiVersion := fmt.Sprintf("%s/%s", group, version)
	return db.performResourceQuery(ctx, query, kind, apiVersion)
}

func (db *BaseDatabase) DBQueryCoreResources(ctx context.Context, kind, version string) ([]*unstructured.Unstructured, error) {
	query := fmt.Sprintf(db.info.resourcesQuery, db.info.resourceTableName) //nolint:gosec
	return db.performResourceQuery(ctx, query, kind, version)
}

func (db *BaseDatabase) DBQueryNamespacedResources(ctx context.Context, kind, group, version, namespace string) ([]*unstructured.Unstructured, error) {
	query := fmt.Sprintf(db.info.namespacedResourcesQuery, db.info.resourceTableName) //nolint:gosec
	apiVersion := fmt.Sprintf("%s/%s", group, version)
	return db.performResourceQuery(ctx, query, kind, apiVersion, namespace)
}

func (db *BaseDatabase) DBQueryNamespacedCoreResources(ctx context.Context, kind, version, namespace string) ([]*unstructured.Unstructured, error) {
	query := fmt.Sprintf(db.info.namespacedResourcesQuery, db.info.resourceTableName) //nolint:gosec
	return db.performResourceQuery(ctx, query, kind, version, namespace)
}

func (db *BaseDatabase) performResourceQuery(ctx context.Context, query string, args ...string) ([]*unstructured.Unstructured, error) {
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
