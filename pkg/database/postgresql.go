// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package database

import (
	"fmt"

	_ "github.com/lib/pq"
)

func init() {
	RegisteredDatabases["postgresql"] = NewPostgreSQLDatabase
}

type PostgreSQLDatabaseInfo struct {
	env map[string]string
}

func (info PostgreSQLDatabaseInfo) GetDriverName() string {
	return "postgres"
}

func (info PostgreSQLDatabaseInfo) GetConnectionString() string {
	return fmt.Sprintf("user=%s password=%s dbname=%s host=%s port=%s sslmode=disable", info.env[DbUserEnvVar],
		info.env[DbPasswordEnvVar], info.env[DbNameEnvVar], info.env[DbHostEnvVar], info.env[DbPortEnvVar])
}

func (info PostgreSQLDatabaseInfo) GetResourcesLimitedSQL() string {
	return "SELECT data->'metadata'->>'creationTimestamp', uuid, data FROM resource WHERE kind=$1 AND api_version=$2 ORDER BY data->'metadata'->>'creationTimestamp', uuid LIMIT $3"
}

func (info PostgreSQLDatabaseInfo) GetResourcesLimitedContinueSQL() string {
	return "SELECT data->'metadata'->>'creationTimestamp', uuid, data FROM resource WHERE kind=$1 AND api_version=$2 AND (data->'metadata'->>'creationTimestamp', uuid) > ($3, $4) ORDER BY data->'metadata'->>'creationTimestamp', uuid LIMIT $5"
}

func (info PostgreSQLDatabaseInfo) GetNamespacedResourcesLimitedSQL() string {
	return "SELECT data->'metadata'->>'creationTimestamp', uuid, data FROM resource WHERE kind=$1 AND api_version=$2 AND namespace=$3 ORDER BY data->'metadata'->>'creationTimestamp', uuid LIMIT $4"
}

func (info PostgreSQLDatabaseInfo) GetNamespacedResourcesLimitedContinueSQL() string {
	return "SELECT data->'metadata'->>'creationTimestamp', uuid, data FROM resource WHERE kind=$1 AND api_version=$2 AND namespace=$3 AND (data->'metadata'->>'creationTimestamp', uuid) > ($4, $5) ORDER BY data->'metadata'->>'creationTimestamp', uuid LIMIT $6"
}

func (info PostgreSQLDatabaseInfo) GetNamespacedResourceByNameSQL() string {
	return "SELECT data->'metadata'->>'creationTimestamp', uuid, data FROM resource WHERE kind=$1 AND api_version=$2 AND namespace=$3 AND name=$4"
}

func (info PostgreSQLDatabaseInfo) GetWriteResourceSQL() string {
	return "INSERT INTO resource (uuid, api_version, kind, name, namespace, resource_version, cluster_deleted_ts, data) " +
		"VALUES ($1, $2, $3, $4, $5, $6, $7, $8) " +
		"ON CONFLICT(uuid) DO UPDATE SET name=$4, namespace=$5, resource_version=$6, cluster_deleted_ts=$7, data=$8"
}

type PostgreSQLDatabase struct {
	*Database
}

func NewPostgreSQLDatabase(env map[string]string) DBInterface {
	info := PostgreSQLDatabaseInfo{env: env}
	db := establishConnection(info.GetDriverName(), info.GetConnectionString())
	return PostgreSQLDatabase{&Database{db: db, info: info}}
}
