// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package database

type DatabaseInfo struct {
	driver                   string
	connectionErrorString    string
	resourceTableName        string
	resourcesQuery           string
	namespacedResourcesQuery string
	writeResourceSQL         string
}

var PostgreSQLDatabaseInfo = &DatabaseInfo{
	driver:                   "postgres",
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
		return MySQLDatabaseInfo
	}

	// Default is postgresql
	return PostgreSQLDatabaseInfo
}
