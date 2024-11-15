// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package database

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	_ "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
	"github.com/kubearchive/kubearchive/pkg/models"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func init() {
	RegisteredDatabases["mariadb"] = NewMariaDBDatabase
	RegisteredDBCreators["postgresql"] = NewMariaDBCreator
}

type MariaDBDatabaseCreator struct {
	env map[string]string
}

func NewMariaDBCreator(env map[string]string) DBCreator {
	return &MariaDBDatabaseCreator{env: env}
}

func (creator MariaDBDatabaseCreator) GetDriverName() string {
	return "mysql"
}

func (creator MariaDBDatabaseCreator) GetConnectionString() string {
	return fmt.Sprintf("%s:%s@tcp(%s:%s)/%s", creator.env[DbUserEnvVar],
		creator.env[DbPasswordEnvVar], creator.env[DbHostEnvVar], creator.env[DbPortEnvVar], creator.env[DbNameEnvVar])
}

type MariaDBSelector struct{}

func (MariaDBSelector) ResourceSelector() Selector {
	return "SELECT JSON_VALUE(data, '$.metadata.creationTimestamp'), uuid, data FROM resource"
}

func (MariaDBSelector) UUIDResourceSelector() Selector {
	return "SELECT uuid FROM resource"
}

func (MariaDBSelector) OwnedResourceSelector() Selector {
	return "SELECT uuid, kind FROM resource"
}

func (MariaDBSelector) UrlFromResourceSelector() Selector {
	return "SELECT log.url FROM log_url log JOIN resource res ON log.uuid = res.uuid"
}

func (MariaDBSelector) UrlSelector() Selector {
	return "SELECT url FROM log_url"
}

type MariaDBFilter struct{}

func (MariaDBFilter) PodFilter(idx int) (Filter, int) {
	return "kind='Pod'", 0
}

func (MariaDBFilter) KindFilter(idx int) (Filter, int) {
	return "kind=?", 1
}

func (MariaDBFilter) ApiVersionFilter(idx int) (Filter, int) {
	return "api_version=?", 1
}

func (MariaDBFilter) NamespaceFilter(idx int) (Filter, int) {
	return "namespace=?", 1
}

func (MariaDBFilter) NameFilter(idx int) (Filter, int) {
	return "name=?", 1
}

func (MariaDBFilter) CreationTSAndIDFilter(idx int) (Filter, int) {
	return "(CONVERT(JSON_VALUE(data, '$.metadata.creationTimestamp'), datetime), uuid) > (?, ?)", 2
}

func (MariaDBFilter) OwnerFilter(idx int) (Filter, int) {
	return "JSON_OVERLAPS(JSON_EXTRACT(data, '$.metadata.ownerReferences.**.uid'), JSON_ARRAY(?))", 1
}

func (MariaDBFilter) UuidFilter(idx int) (Filter, int) {
	return "uuid IN (?)", 1
}

type MariaDBSorter struct{}

func (MariaDBSorter) CreationTSAndIDSorter() Sorter {
	return "ORDER BY CONVERT(JSON_VALUE(data, '$.metadata.creationTimestamp'), datetime)"
}

type MariaDBLimiter struct{}

func (MariaDBLimiter) Limiter(idx int) Limiter {
	return "LIMIT ?"
}

type MariaDBInserter struct{}

func (MariaDBInserter) ResourceInserter() string {
	return "INSERT INTO resource (uuid, api_version, kind, name, namespace, resource_version, cluster_deleted_ts, data) " +
		"VALUES (?, ?, ?, ?, ?, ?, ?, ?) " +
		"ON DUPLICATE KEY UPDATE name=?, namespace=?, resource_version=?, cluster_deleted_ts=?, data=?"
}

func (MariaDBInserter) UrlInserter() string {
	return "INSERT INTO log_url (uuid, url, container_name) VALUES (?, ?, ?)"
}

type MariaDBDeleter struct{}

func (MariaDBDeleter) UrlDeleter() string {
	return "DELETE FROM log_url WHERE uuid=?"
}

type MariaDBParamParser struct{}

// ParseParams in MariaDB transform the query from one param ? to one for each element in the array and
// flattens the array of strings into strings as MariaDB doesn't support arrays as parameters for
// prepared queries
func (MariaDBParamParser) ParseParams(query string, args ...any) (string, []any, error) {
	var parsedArgs []any
	parsedQuery := query
	for i, arg := range args {
		switch reflect.TypeOf(arg).Kind() {
		case reflect.Slice:
			arraySize := len(arg.([]string))
			newParams := strings.Join(strings.Fields(strings.Repeat("? ", arraySize)), ",")
			parsedQuery = replaceNth(query, "?", newParams, i)
			for _, elem := range arg.([]string) {
				parsedArgs = append(parsedArgs, elem)
			}
		default:
			parsedArgs = append(parsedArgs, arg)
		}
	}
	return parsedQuery, parsedArgs, nil
}

type MariaDBDatabase struct {
	*Database
}

func NewMariaDBDatabase(conn *sqlx.DB) DBInterface {
	return MariaDBDatabase{&Database{
		db:          conn,
		selector:    MariaDBSelector{},
		filter:      MariaDBFilter{},
		sorter:      MariaDBSorter{},
		limiter:     MariaDBLimiter{},
		inserter:    MariaDBInserter{},
		deleter:     MariaDBDeleter{},
		paramParser: MariaDBParamParser{},
	}}
}

func (db MariaDBDatabase) WriteResource(ctx context.Context, k8sObj *unstructured.Unstructured, data []byte) error {
	tx, err := db.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("could not begin transaction for resource %s: %s", k8sObj.GetUID(), err)
	}
	_, execErr := tx.ExecContext(
		ctx,
		db.inserter.ResourceInserter(),
		k8sObj.GetUID(),
		k8sObj.GetAPIVersion(),
		k8sObj.GetKind(),
		k8sObj.GetName(),
		k8sObj.GetNamespace(),
		k8sObj.GetResourceVersion(),
		models.OptionalTimestamp(k8sObj.GetDeletionTimestamp()),
		data,
		k8sObj.GetName(),
		k8sObj.GetNamespace(),
		k8sObj.GetResourceVersion(),
		models.OptionalTimestamp(k8sObj.GetDeletionTimestamp()),
		data,
	)
	if execErr != nil {
		rollbackErr := tx.Rollback()
		if rollbackErr != nil {
			return fmt.Errorf("write to database failed: %s and unable to roll back transaction: %s", execErr, rollbackErr)
		}
		return fmt.Errorf("write to database failed: %s", execErr)
	}
	execErr = tx.Commit()
	if execErr != nil {
		rollbackErr := tx.Rollback()
		if rollbackErr != nil {
			return fmt.Errorf("commit to database failed: %s and unable to roll back transaction: %s", execErr, rollbackErr)
		}
		return fmt.Errorf("commit to database failed and the transactions was rolled back: %s", execErr)
	}
	return nil
}

// Replace the nth occurrence of old in s by replaced.
func replaceNth(s, old, replaced string, n int) string {
	i := 0
	for m := 1; m <= n; m++ {
		x := strings.Index(s[i:], old)
		if x < 0 {
			break
		}
		i += x
		if m == n {
			return s[:i] + replaced + s[i+len(old):]
		}
		i += len(old)
	}
	return s
}
