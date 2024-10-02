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

func (info PostgreSQLDatabaseInfo) GetResourcesSQL() string {
	return "SELECT data FROM resource WHERE kind=$1 AND api_version=$2"
}

func (info PostgreSQLDatabaseInfo) GetNamespacedResourcesSQL() string {
	return "SELECT data FROM resource WHERE kind=$1 AND api_version=$2 AND namespace=$3"
}

func (info PostgreSQLDatabaseInfo) GetWriteResourceSQL() string {
	return "INSERT INTO resource (uuid, api_version, kind, name, namespace, resource_version, cluster_deleted_ts, data) " +
		"VALUES ($1, $2, $3, $4, $5, $6, $7, $8) " +
		"ON CONFLICT(uuid) DO UPDATE SET name=$4, namespace=$5, resource_version=$6, cluster_deleted_ts=$7, data=$8"
}

func (info PostgreSQLDatabaseInfo) GetNamespacedResourceByNameSQL() string {
	return "SELECT data FROM resource WHERE kind=$1 AND api_version=$2 AND namespace=$3 AND name=$4"
}

type PostgreSQLDatabase struct {
	*Database
}

func NewPostgreSQLDatabase(env map[string]string) DBInterface {
	info := PostgreSQLDatabaseInfo{env: env}
	db := establishConnection(info.GetDriverName(), info.GetConnectionString())
	return PostgreSQLDatabase{&Database{db: db, info: info}}
}
