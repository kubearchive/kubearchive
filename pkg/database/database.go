// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/XSAM/otelsql"
	"github.com/avast/retry-go/v4"
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const (
	dbConnectionErrStr string = "Could not create database connection string: %s must be set"

	DbKindEnvVar     string = "DATABASE_KIND"
	DbNameEnvVar     string = "DATABASE_DB"
	DbUserEnvVar     string = "DATABASE_USER"
	DbPasswordEnvVar string = "DATABASE_PASSWORD" // #nosec G101 not a password
	DbHostEnvVar     string = "DATABASE_URL"
	DbPortEnvVar     string = "DATABASE_PORT"
)

var DbEnvVars = [...]string{DbKindEnvVar, DbNameEnvVar, DbUserEnvVar, DbPasswordEnvVar, DbHostEnvVar, DbPortEnvVar}

type DatabaseInterface interface {
	QueryResources(ctx context.Context, kind, group, version string) ([]*unstructured.Unstructured, error)
	QueryCoreResources(ctx context.Context, kind, version string) ([]*unstructured.Unstructured, error)
	QueryNamespacedResources(ctx context.Context, kind, group, version, namespace string) ([]*unstructured.Unstructured, error)
	QueryNamespacedCoreResources(ctx context.Context, kind, version, namespace string) ([]*unstructured.Unstructured, error)
	WriteResource(ctx context.Context, k8sObj *unstructured.Unstructured, data []byte) error
	Ping(ctx context.Context) error
}

func NewDatabase() (DatabaseInterface, error) {
	env, err := getDatabaseEnvironmentVars()
	if err != nil {
		return nil, err
	}

	configs := []retry.Option{
		retry.Attempts(10),
		retry.OnRetry(func(n uint, err error) {
			log.Printf("Retry request %d, get error: %v", n+1, err)
		}),
		retry.Delay(time.Second),
	}

	driverName, connectionString := getConnectionString(env)
	var db *sql.DB
	errRetry := retry.Do(
		func() error {
			db, err = otelsql.Open(driverName, connectionString)
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
		return newMySQLDatabase(db), nil
	} else {
		return newPostgreSQLDatabase(db), nil
	}
}

// Reads database connection info from the environment variables and returns a map of variable name to value.
func getDatabaseEnvironmentVars() (map[string]string, error) {
	var err error
	env := make(map[string]string)
	for _, name := range DbEnvVars {
		value, exists := os.LookupEnv(name)
		if exists {
			env[name] = value
		} else {
			err = errors.Join(err, fmt.Errorf(dbConnectionErrStr, name))
		}
	}
	if err == nil {
		return env, nil
	} else {
		return nil, err
	}

}

func getConnectionString(env map[string]string) (string, string) {
	if env[DbKindEnvVar] == "mysql" {
		return "mysql", fmt.Sprintf(
			"%s:%s@tcp(%s:%s)/%s",
			env[DbUserEnvVar],
			env[DbPasswordEnvVar],
			env[DbHostEnvVar],
			env[DbPortEnvVar],
			env[DbNameEnvVar],
		)
	}

	return "postgres", fmt.Sprintf(
		"user=%s password=%s dbname=%s host=%s port=%s sslmode=disable",
		env[DbUserEnvVar],
		env[DbPasswordEnvVar],
		env[DbNameEnvVar],
		env[DbHostEnvVar],
		env[DbPortEnvVar],
	)
}
