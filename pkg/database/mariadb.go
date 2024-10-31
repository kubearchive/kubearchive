// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package database

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	_ "github.com/go-sql-driver/mysql"
	"github.com/kubearchive/kubearchive/pkg/models"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func init() {
	RegisteredDatabases["mariadb"] = NewMariaDBDatabase
}

type MariaDBDatabaseInfo struct {
	env map[string]string
}

func (info MariaDBDatabaseInfo) GetDriverName() string {
	return "mysql"
}

func (info MariaDBDatabaseInfo) GetConnectionString() string {
	return fmt.Sprintf("%s:%s@tcp(%s:%s)/%s", info.env[DbUserEnvVar],
		info.env[DbPasswordEnvVar], info.env[DbHostEnvVar], info.env[DbPortEnvVar], info.env[DbNameEnvVar])
}

func (info MariaDBDatabaseInfo) GetResourcesLimitedSQL() string {
	return "SELECT JSON_VALUE(data, '$.metadata.creationTimestamp'), uuid, data FROM resource WHERE kind=? AND api_version=? ORDER BY CONVERT(JSON_VALUE(data, '$.metadata.creationTimestamp'), datetime), uuid LIMIT ?"
}

func (info MariaDBDatabaseInfo) GetResourcesLimitedContinueSQL() string {
	return "SELECT JSON_VALUE(data, '$.metadata.creationTimestamp'), uuid, data FROM resource WHERE kind=? AND api_version=? AND (CONVERT(JSON_VALUE(data, '$.metadata.creationTimestamp'), datetime), uuid) > (?, ?) ORDER BY CONVERT(JSON_VALUE(data, '$.metadata.creationTimestamp'), datetime), uuid LIMIT ?"
}

func (info MariaDBDatabaseInfo) GetNamespacedResourcesLimitedSQL() string {
	return "SELECT JSON_VALUE(data, '$.metadata.creationTimestamp'), uuid, data FROM resource WHERE kind=? AND api_version=? AND namespace=? ORDER BY CONVERT(JSON_VALUE(data, '$.metadata.creationTimestamp'), datetime), uuid LIMIT ?"
}

func (info MariaDBDatabaseInfo) GetNamespacedResourcesLimitedContinueSQL() string {
	return "SELECT JSON_VALUE(data, '$.metadata.creationTimestamp'), uuid, data FROM resource WHERE kind=? AND api_version=? AND namespace=? AND (CONVERT(JSON_VALUE(data, '$.metadata.creationTimestamp'), datetime), uuid) > (?, ?) ORDER BY CONVERT(JSON_VALUE(data, '$.metadata.creationTimestamp'), datetime), uuid LIMIT ?"
}

func (info MariaDBDatabaseInfo) GetNamespacedResourceByNameSQL() string {
	return "SELECT JSON_VALUE(data, '$.metadata.creationTimestamp'), uuid, data FROM resource WHERE kind=? AND api_version=? AND namespace=? AND name=?"
}

func (info MariaDBDatabaseInfo) GetUUIDSQL() string {
	return "SELECT uuid FROM resource WHERE kind=? AND api_version=? AND namespace=? AND name=?"
}

func (info MariaDBDatabaseInfo) GetOwnedResourcesSQL() string {
	// TODO test
	return "SELECT uuid, kind FROM resource WHERE JSON_OVERLAPS(JSON_EXTRACT(data, '$.metadata.ownerReferences.**.uid'), JSON_ARRAY(?))"
}

func (info MariaDBDatabaseInfo) GetLogURLsByPodNameSQL() string {
	return "SELECT log.url FROM log_url log JOIN resource res ON log.uuid=res.uuid WHERE res.kind='Pod' AND res.api_version=? AND res.namespace=? AND res.name=?"
}

func (info MariaDBDatabaseInfo) GetLogURLsSQL() string {
	return "SELECT url FROM log_url WHERE uuid IN (?)"
}

func (info MariaDBDatabaseInfo) GetWriteResourceSQL() string {
	return "INSERT INTO resource (uuid, api_version, kind, name, namespace, resource_version, cluster_deleted_ts, data) " +
		"VALUES (?, ?, ?, ?, ?, ?, ?, ?) " +
		"ON DUPLICATE KEY UPDATE name=?, namespace=?, resource_version=?, cluster_deleted_ts=?, data=?"
}

func (info MariaDBDatabaseInfo) GetWriteUrlSQL() string {
	return "INSERT INTO log_url (uuid, url, container_name) VALUES (?, ?, ?)"
}

func (info MariaDBDatabaseInfo) GetDeleteUrlsSQL() string {
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

func NewMariaDBDatabase(env map[string]string) DBInterface {
	info := MariaDBDatabaseInfo{env: env}
	paramParser := MariaDBParamParser{}
	db := establishConnection(info.GetDriverName(), info.GetConnectionString())
	return MariaDBDatabase{&Database{db: db, info: info, paramParser: paramParser}}
}

func (db MariaDBDatabase) WriteResource(ctx context.Context, k8sObj *unstructured.Unstructured, data []byte) error {
	tx, err := db.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("could not begin transaction for resource %s: %s", k8sObj.GetUID(), err)
	}
	_, execErr := tx.ExecContext(
		ctx,
		db.info.GetWriteResourceSQL(),
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
