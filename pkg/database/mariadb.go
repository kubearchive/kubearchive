// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package database

import (
	"context"
	"fmt"

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

func (info MariaDBDatabaseInfo) GetResourcesSQL() string {
	return "SELECT data FROM resource WHERE kind=? AND api_version=?"
}

func (info MariaDBDatabaseInfo) GetNamespacedResourcesSQL() string {
	return "SELECT data FROM resource WHERE kind=? AND api_version=? AND namespace=?"
}

func (info MariaDBDatabaseInfo) GetNamespacedResourceByNameSQL() string {
	return "SELECT data FROM resource WHERE kind=? AND api_version=? AND namespace=? AND name=?"
}

func (info MariaDBDatabaseInfo) GetWriteResourceSQL() string {
	return "INSERT INTO resource (uuid, api_version, kind, name, namespace, resource_version, cluster_deleted_ts, data) " +
		"VALUES (?, ?, ?, ?, ?, ?, ?, ?) " +
		"ON DUPLICATE KEY UPDATE name=?, namespace=?, resource_version=?, cluster_deleted_ts=?, data=?"
}

type MariaDBDatabase struct {
	*Database
}

func NewMariaDBDatabase(env map[string]string) DBInterface {
	info := MariaDBDatabaseInfo{env: env}
	db := establishConnection(info.GetDriverName(), info.GetConnectionString())
	return MariaDBDatabase{&Database{db: db, info: info}}
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
