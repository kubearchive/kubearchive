// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package database

import (
	"context"
	"fmt"

	"github.com/kubearchive/kubearchive/pkg/models"
	_ "github.com/lib/pq"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const postgreSQLConnectionString = "user=%s password=%s dbname=%s host=%s port=%s sslmode=disable"

var postgresqlStmts = &SQLStatements{
	resourceTableName:        "resource",
	resourcesQuery:           "SELECT data FROM %s WHERE kind=$1 AND api_version=$2",
	namespacedResourcesQuery: "SELECT data FROM %s WHERE kind=$1 AND api_version=$2 AND namespace=$3",
	writeResourceSQL: "INSERT INTO %s (uuid, api_version, kind, name, namespace, resource_version, cluster_deleted_ts, data) " +
		"VALUES ($1, $2, $3, $4, $5, $6, $7, $8) " +
		"ON CONFLICT(uuid) DO UPDATE SET name=$4, namespace=$5, resource_version=$6, cluster_deleted_ts=$7, data=$8",
}

type PostgreSQLDatabase struct {
	*Database
}

func NewPostgreSQLDatabase(env *databaseEnvironment) (*PostgreSQLDatabase, error) {
	conn, err := establishConnection("postgres",
		fmt.Sprintf(postgreSQLConnectionString, env.user, env.password, env.host, env.port, env.name))
	if err != nil {
		return nil, err
	}
	return &PostgreSQLDatabase{&Database{conn, *postgresqlStmts}}, nil
}

func (db PostgreSQLDatabase) WriteResource(ctx context.Context, k8sObj *unstructured.Unstructured, data []byte) error {
	query := fmt.Sprintf(db.sqlStmts.writeResourceSQL, db.sqlStmts.resourceTableName)
	tx, err := db.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("could not begin transaction for resource %s: %s", k8sObj.GetUID(), err)
	}
	_, execErr := tx.ExecContext(
		ctx,
		query,
		k8sObj.GetUID(),
		k8sObj.GetAPIVersion(),
		k8sObj.GetKind(),
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
