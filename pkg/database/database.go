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
	resourceTableName        = "resource"
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
	db   *sql.DB
	info DatabaseInfo
}

func NewDatabase() (DBInterface, error) {
	env, err := getDatabaseEnvironmentVars()
	if err != nil {
		return nil, err
	}
	var db *sql.DB
	dbInfo := NewDatabaseInfo(env)

	configs := []retry.Option{
		retry.Attempts(10),
		retry.OnRetry(func(n uint, err error) {
			log.Printf("Retry request %d, get error: %v", n+1, err)
		}),
		retry.Delay(time.Second),
	}

	errRetry := retry.Do(
		func() error {
			db, err = otelsql.Open(dbInfo.driver, dbInfo.connectionString)
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

	if env[DbKindEnvVar] == "mysql" {
		return MySQLDatabase{&Database{db, *dbInfo}}, nil
	} else {
		return PostgreSQLDatabase{&Database{db, *dbInfo}}, nil
	}
}

func (db *Database) Ping(ctx context.Context) error {
	return db.db.PingContext(ctx)
}

func (db *Database) QueryResources(ctx context.Context, kind, group, version string) ([]*unstructured.Unstructured, error) {
	query := fmt.Sprintf(resourcesQuery, db.info.resourceTableName) //nolint:gosec
	apiVersion := fmt.Sprintf("%s/%s", group, version)
	return db.performResourceQuery(ctx, query, kind, apiVersion)
}

func (db *Database) QueryCoreResources(ctx context.Context, kind, version string) ([]*unstructured.Unstructured, error) {
	query := fmt.Sprintf(resourcesQuery, db.info.resourceTableName) //nolint:gosec
	return db.performResourceQuery(ctx, query, kind, version)
}

func (db *Database) QueryNamespacedResources(ctx context.Context, kind, group, version, namespace string) ([]*unstructured.Unstructured, error) {
	query := fmt.Sprintf(namespacedResourcesQuery, db.info.resourceTableName) //nolint:gosec
	apiVersion := fmt.Sprintf("%s/%s", group, version)
	return db.performResourceQuery(ctx, query, kind, apiVersion, namespace)
}

func (db *Database) QueryNamespacedCoreResources(ctx context.Context, kind, version, namespace string) ([]*unstructured.Unstructured, error) {
	query := fmt.Sprintf(namespacedResourcesQuery, db.info.resourceTableName) //nolint:gosec
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
