// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package database

import (
	"context"
	"database/sql"
	"log"
	"time"

	"github.com/XSAM/otelsql"
	"github.com/avast/retry-go/v4"
	_ "github.com/go-sql-driver/mysql"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type DatabaseInterface interface {
	QueryResources(ctx context.Context, kind, group, version string) ([]*unstructured.Unstructured, error)
	QueryCoreResources(ctx context.Context, kind, version string) ([]*unstructured.Unstructured, error)
	QueryNamespacedResources(ctx context.Context, kind, group, version, namespace string) ([]*unstructured.Unstructured, error)
	QueryNamespacedCoreResources(ctx context.Context, kind, version, namespace string) ([]*unstructured.Unstructured, error)
	WriteResource(ctx context.Context, k8sObj *unstructured.Unstructured, data []byte) error
	Ping(ctx context.Context) error
}

type Database struct {
	db         *sql.DB
	info       *DatabaseInfo
	postgresql *PostgreSQLDatabase
	mysql      *MySQLDatabase
}

func NewDatabase() (*Database, error) {
	env, err := getDatabaseEnvironmentVars()
	if err != nil {
		return nil, err
	}

	database := &Database{info: NewDatabaseInfo(env)}
	if env[DbKindEnvVar] == "mysql" {
		database.mysql = &MySQLDatabase{BaseDatabase: &BaseDatabase{info: database.info}}
	} else {
		database.postgresql = &PostgreSQLDatabase{BaseDatabase: &BaseDatabase{info: database.info}}
	}

	configs := []retry.Option{
		retry.Attempts(10),
		retry.OnRetry(func(n uint, err error) {
			log.Printf("Retry request %d, get error: %v", n+1, err)
		}),
		retry.Delay(time.Second),
	}

	errRetry := retry.Do(
		func() error {
			database.db, err = otelsql.Open(database.info.driver, database.info.connectionString)
			if err != nil {
				return err
			}
			return database.db.Ping()
		},
		configs...)
	if errRetry != nil {
		return nil, errRetry
	}
	log.Println("Successfully connected to the database")

	if env[DbKindEnvVar] == "mysql" {
		database.mysql.db = database.db
	} else {
		database.postgresql.db = database.db
	}
	return database, nil
}

func (db *Database) Ping(ctx context.Context) error {
	if db.mysql != nil {
		return db.mysql.DBPing(ctx)
	}
	return db.postgresql.DBPing(ctx)
}

func (db *Database) QueryResources(ctx context.Context, kind, group, version string) ([]*unstructured.Unstructured, error) {
	if db.mysql != nil {
		return db.mysql.DBQueryResources(ctx, kind, group, version)
	}
	return db.postgresql.DBQueryResources(ctx, kind, group, version)
}

func (db *Database) QueryCoreResources(ctx context.Context, kind, version string) ([]*unstructured.Unstructured, error) {
	if db.mysql != nil {
		return db.mysql.DBQueryCoreResources(ctx, kind, version)
	}
	return db.postgresql.DBQueryCoreResources(ctx, kind, version)
}

func (db *Database) QueryNamespacedResources(ctx context.Context, kind, group, version, namespace string) ([]*unstructured.Unstructured, error) {
	if db.mysql != nil {
		return db.mysql.DBQueryNamespacedResources(ctx, kind, group, version, namespace)
	}
	return db.postgresql.DBQueryNamespacedResources(ctx, kind, group, version, namespace)
}

func (db *Database) QueryNamespacedCoreResources(ctx context.Context, kind, version, namespace string) ([]*unstructured.Unstructured, error) {
	if db.mysql != nil {
		return db.mysql.DBQueryNamespacedCoreResources(ctx, kind, version, namespace)
	}
	return db.postgresql.DBQueryNamespacedCoreResources(ctx, kind, version, namespace)
}

func (db *Database) WriteResource(ctx context.Context, k8sObj *unstructured.Unstructured, data []byte) error {
	if db.mysql != nil {
		return db.mysql.DBWriteResource(ctx, k8sObj, data)
	}
	return db.postgresql.DBWriteResource(ctx, k8sObj, data)
}
