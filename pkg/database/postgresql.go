// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package database

import (
	"fmt"
	"reflect"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	_ "github.com/lib/pq"
)

func init() {
	RegisteredDatabases["postgresql"] = NewPostgreSQLDatabase
	RegisteredDBCreators["postgresql"] = NewPostgreSQLCreator
}

type PostgreSQLCreator struct {
	env map[string]string
}

func NewPostgreSQLCreator(env map[string]string) DBCreator {
	return PostgreSQLCreator{env: env}
}

func (creator PostgreSQLCreator) GetDriverName() string {
	return "postgres"
}

func (creator PostgreSQLCreator) GetConnectionString() string {
	return fmt.Sprintf("user=%s password=%s dbname=%s host=%s port=%s sslmode=disable", creator.env[DbUserEnvVar],
		creator.env[DbPasswordEnvVar], creator.env[DbNameEnvVar], creator.env[DbHostEnvVar], creator.env[DbPortEnvVar])
}

type PostgreSQLSelector struct{}

func (PostgreSQLSelector) ResourceSelector() Selector {
	return "SELECT data->'metadata'->>'creationTimestamp' as created_at, id, data FROM resource"
}

func (PostgreSQLSelector) UUIDResourceSelector() Selector {
	return "SELECT uuid FROM resource"
}

func (PostgreSQLSelector) OwnedResourceSelector() Selector {
	return "SELECT uuid, kind FROM resource"
}

func (PostgreSQLSelector) UrlFromResourceSelector() Selector {
	return "SELECT log.url FROM log_url log JOIN resource res ON log.uuid = res.uuid"
}

func (PostgreSQLSelector) UrlSelector() Selector {
	return "SELECT url FROM log_url"
}

type PostgreSQLFilter struct{}

func (PostgreSQLFilter) PodFilter(idx int) (Filter, int) {
	return "kind='Pod'", 0
}

func (PostgreSQLFilter) KindFilter(idx int) (Filter, int) {
	return Filter(fmt.Sprintf("kind=$%d", idx)), 1
}

func (PostgreSQLFilter) ApiVersionFilter(idx int) (Filter, int) {
	return Filter(fmt.Sprintf("api_version=$%d", idx)), 1
}

func (PostgreSQLFilter) NamespaceFilter(idx int) (Filter, int) {
	return Filter(fmt.Sprintf("namespace=$%d", idx)), 1
}

func (PostgreSQLFilter) NameFilter(idx int) (Filter, int) {
	return Filter(fmt.Sprintf("name=$%d", idx)), 1
}

func (PostgreSQLFilter) CreationTSAndIDFilter(idx int) (Filter, int) {
	return Filter(fmt.Sprintf("(data->'metadata'->>'creationTimestamp', id) < ($%d, $%d)", idx, idx+1)), 2
}

func (PostgreSQLFilter) OwnerFilter(idx int) (Filter, int) {
	return Filter(
		fmt.Sprintf("jsonb_path_query_array(data->'metadata'->'ownerReferences', '$[*].uid') ?| $%d", idx)), 1
}

func (PostgreSQLFilter) UuidFilter(idx int) (Filter, int) {
	return Filter(fmt.Sprintf("uuid=any($%d)", idx)), 1
}

type PostgreSQLSorter struct{}

func (PostgreSQLSorter) CreationTSAndIDSorter() Sorter {
	return "ORDER BY data->'metadata'->>'creationTimestamp' DESC, id DESC"
}

type PostgreSQLLimiter struct{}

func (PostgreSQLLimiter) Limiter(idx int) Limiter {
	return Limiter(fmt.Sprintf("LIMIT $%d", idx))
}

type PostgreSQLInserter struct{}

func (PostgreSQLInserter) ResourceInserter() string {
	return "INSERT INTO resource (uuid, api_version, kind, name, namespace, resource_version, cluster_deleted_ts, data) " +
		"VALUES ($1, $2, $3, $4, $5, $6, $7, $8) " +
		"ON CONFLICT(uuid) DO UPDATE SET name=$4, namespace=$5, resource_version=$6, cluster_deleted_ts=$7, data=$8"
}

func (PostgreSQLInserter) UrlInserter() string {
	return "INSERT INTO log_url (uuid, url, container_name) VALUES ($1, $2, $3)"
}

type PostgreSQLDeleter struct{}

func (PostgreSQLDeleter) UrlDeleter() string {
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

func NewPostgreSQLDatabase(conn *sqlx.DB) DBInterface {
	return PostgreSQLDatabase{&Database{
		db:          conn,
		selector:    PostgreSQLSelector{},
		filter:      PostgreSQLFilter{},
		sorter:      PostgreSQLSorter{},
		limiter:     PostgreSQLLimiter{},
		inserter:    PostgreSQLInserter{},
		deleter:     PostgreSQLDeleter{},
		paramParser: PostgreSQLParamParser{},
	}}
}
