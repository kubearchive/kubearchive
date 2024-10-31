// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package database

import (
	"fmt"
	"reflect"

	"github.com/lib/pq"
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
	return "SELECT data->'metadata'->>'creationTimestamp' as created_at, id, data FROM resource WHERE kind=$1 AND api_version=$2 ORDER BY data->'metadata'->>'creationTimestamp' DESC, id DESC LIMIT $3"
}

func (info PostgreSQLDatabaseInfo) GetResourcesLimitedContinueSQL() string {
	return "SELECT data->'metadata'->>'creationTimestamp' as created_at, id, data FROM resource WHERE kind=$1 AND api_version=$2 AND (data->'metadata'->>'creationTimestamp', id) < ($3, $4) ORDER BY data->'metadata'->>'creationTimestamp' DESC, id DESC LIMIT $5"
}

func (info PostgreSQLDatabaseInfo) GetNamespacedResourcesLimitedSQL() string {
	return "SELECT data->'metadata'->>'creationTimestamp' as created_at, id, data FROM resource WHERE kind=$1 AND api_version=$2 AND namespace=$3 ORDER BY data->'metadata'->>'creationTimestamp' DESC, id DESC LIMIT $4"
}

func (info PostgreSQLDatabaseInfo) GetNamespacedResourcesLimitedContinueSQL() string {
	return "SELECT data->'metadata'->>'creationTimestamp' as created_at, id, data FROM resource WHERE kind=$1 AND api_version=$2 AND namespace=$3 AND (data->'metadata'->>'creationTimestamp', id) < ($4, $5) ORDER BY data->'metadata'->>'creationTimestamp' DESC, id DESC LIMIT $6"
}

func (info PostgreSQLDatabaseInfo) GetNamespacedResourceByNameSQL() string {
	return "SELECT data->'metadata'->>'creationTimestamp' as created_at, id, data FROM resource WHERE kind=$1 AND api_version=$2 AND namespace=$3 AND name=$4"
}

func (info PostgreSQLDatabaseInfo) GetUUIDSQL() string {
	return "SELECT uuid FROM resource WHERE kind=$1 AND api_version=$2 AND namespace=$3 AND name=$4"
}

func (info PostgreSQLDatabaseInfo) GetOwnedResourcesSQL() string {
	return "SELECT uuid, kind FROM resource WHERE jsonb_path_query_array(data->'metadata'->'ownerReferences', '$[*].uid') ?| $1"
}

func (info PostgreSQLDatabaseInfo) GetLogURLsByPodNameSQL() string {
	return "SELECT log.url FROM log_url log JOIN resource res ON log.uuid = res.uuid WHERE res.kind='Pod' AND res.api_version=$1 AND res.namespace=$2 AND res.name = $3"
}

func (info PostgreSQLDatabaseInfo) GetLogURLsSQL() string {
	return "SELECT url FROM log_url WHERE uuid = any($1)"
}

func (info PostgreSQLDatabaseInfo) GetWriteResourceSQL() string {
	return "INSERT INTO resource (uuid, api_version, kind, name, namespace, resource_version, cluster_deleted_ts, data) " +
		"VALUES ($1, $2, $3, $4, $5, $6, $7, $8) " +
		"ON CONFLICT(uuid) DO UPDATE SET name=$4, namespace=$5, resource_version=$6, cluster_deleted_ts=$7, data=$8"
}

func (info PostgreSQLDatabaseInfo) GetWriteUrlSQL() string {
	return "INSERT INTO log_url (uuid, url, container_name) VALUES ($1, $2, $3)"
}

func (info PostgreSQLDatabaseInfo) GetDeleteUrlsSQL() string {
	return "DELETE FROM log_url WHERE uuid=$1"
}

type PostgreSQLParamParser struct{}

// ParseParams in PostgreSQL transform the given arrays as pq.Array because the driver
// accepts array parameters for prepared statements
func (PostgreSQLParamParser) ParseParams(query string, args ...any) (string, []any, error) {
	var parsedArgs []any
	for _, arg := range args {
		switch reflect.TypeOf(arg).Kind() {
		case reflect.Slice:
			parsedArgs = append(parsedArgs, pq.Array(arg))
		default:
			parsedArgs = append(parsedArgs, arg)
		}
	}
	return query, parsedArgs, nil
}

type PostgreSQLDatabase struct {
	*Database
}

func NewPostgreSQLDatabase(env map[string]string) DBInterface {
	info := PostgreSQLDatabaseInfo{env: env}
	paramParser := PostgreSQLParamParser{}
	db := establishConnection(info.GetDriverName(), info.GetConnectionString())
	return PostgreSQLDatabase{&Database{db: db, info: info, paramParser: paramParser}}
}
