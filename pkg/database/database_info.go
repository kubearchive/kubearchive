// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package database

import (
	"errors"
	"fmt"
	"os"
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

type DatabaseInfo struct {
	driver                   string
	connectionString         string
	connectionErrorString    string
	resourceTableName        string
	resourcesQuery           string
	namespacedResourcesQuery string
	writeResourceSQL         string
}

var PostgreSQLDatabaseInfo = &DatabaseInfo{
	driver:                   "postgres",
	connectionString:         "user=%s password=%s dbname=%s host=%s port=%s sslmode=disable",
	connectionErrorString:    dbConnectionErrStr,
	resourceTableName:        "resource",
	resourcesQuery:           "SELECT data FROM %s WHERE kind=$1 AND api_version=$2",
	namespacedResourcesQuery: "SELECT data FROM %s WHERE kind=$1 AND api_version=$2 AND namespace=$3",
	writeResourceSQL: "INSERT INTO %s (uuid, api_version, kind, name, namespace, resource_version, cluster_deleted_ts, data) " +
		"VALUES ($1, $2, $3, $4, $5, $6, $7, $8) " +
		"ON CONFLICT(uuid) DO UPDATE SET name=$4, namespace=$5, resource_version=$6, cluster_deleted_ts=$7, data=$8",
}

var MySQLDatabaseInfo = &DatabaseInfo{
	driver:                   "mysql",
	connectionString:         "%s:%s@tcp(%s:%s)/%s",
	connectionErrorString:    dbConnectionErrStr,
	resourceTableName:        "resource",
	resourcesQuery:           "SELECT data FROM %s WHERE kind=? AND api_version=?",
	namespacedResourcesQuery: "SELECT data FROM %s WHERE kind=? AND api_version=? AND namespace=?",
	writeResourceSQL: "INSERT INTO %s (uuid, api_version, kind, name, namespace, resource_version, cluster_deleted_ts, data) " +
		"VALUES (?, ?, ?, ?, ?, ?, ?, ?) " +
		"ON DUPLICATE KEY UPDATE name=?, namespace=?, resource_version=?, cluster_deleted_ts=?, data=?",
}

func NewDatabaseInfo(env map[string]string) *DatabaseInfo {
	if env[DbKindEnvVar] == "mysql" {
		MySQLDatabaseInfo.connectionString = fmt.Sprintf(MySQLDatabaseInfo.connectionString, env[DbUserEnvVar],
			env[DbPasswordEnvVar], env[DbHostEnvVar], env[DbPortEnvVar], env[DbNameEnvVar])
		return MySQLDatabaseInfo
	}

	// Default is postgresql
	PostgreSQLDatabaseInfo.connectionString = fmt.Sprintf(PostgreSQLDatabaseInfo.connectionString, env[DbUserEnvVar],
		env[DbPasswordEnvVar], env[DbNameEnvVar], env[DbHostEnvVar], env[DbPortEnvVar])
	return PostgreSQLDatabaseInfo
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
