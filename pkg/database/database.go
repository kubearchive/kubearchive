// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package database

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/kubearchive/kubearchive/pkg/models"
	_ "github.com/lib/pq"
)

const (
	resourceTableName   = "resource"
	resourcesQuery      = "SELECT data FROM %s WHERE kind=$1 AND api_version=$2"
	resourceIdQuery     = `SELECT id FROM %s WHERE kind=$1 AND api_version=$2 AND cluster=$3 AND cluster_uid=$4 AND name=$5 AND namespace=$6`
	updateResourceQuery = `UPDATE %s SET resource_version=$1, updated_ts=$2, cluster_deleted_ts=$3, data=$4 WHERE id=$5`
	writeResource       = `INSERT INTO %s (api_version, cluster, cluster_uid, kind, name, namespace, resource_version, created_ts, updated_ts, cluster_deleted_ts, data) Values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`
)

type DBInterface interface {
	QueryResources(ctx context.Context, kind, group, version string) ([]models.Resource, error)
	QueryResourceId(ctx context.Context, entry *models.ResourceEntry) (int64, error)
	UpdateResource(ctx context.Context, id int64, entry *models.ResourceEntry) error
	WriteResource(ctx context.Context, entry *models.ResourceEntry) error
}

type Database struct {
	db                *sql.DB
	resourceTableName string
}

func NewDatabase() (*Database, error) {
	dataSource, err := ConnectionStr()
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("postgres", dataSource)
	if err != nil {
		return nil, err
	}
	if err = db.Ping(); err != nil {
		return nil, err
	}
	return &Database{db, resourceTableName}, nil
}

func (db *Database) QueryResources(ctx context.Context, kind, group, version string) ([]models.Resource, error) {
	query := fmt.Sprintf(resourcesQuery, db.resourceTableName) //nolint:gosec
	apiVersion := fmt.Sprintf("%s/%s", group, version)
	rows, err := db.db.QueryContext(ctx, query, kind, apiVersion)
	defer func(rows *sql.Rows) {
		err = rows.Close()
	}(rows)
	if err != nil {
		return nil, err
	}
	var resources []models.Resource
	for rows.Next() {
		var r models.Resource
		if err := rows.Scan(&r); err != nil {
			return resources, err
		}
		resources = append(resources, r)
	}
	return resources, err
}

// Returns the first id from the database for a resource that has the specified kind, apiVersion, clusterUid. Returns an
// error if there are no resources that match the query.
func (db *Database) QueryResourceId(ctx context.Context, entry *models.ResourceEntry) (int64, error) {
	query := fmt.Sprintf(resourceIdQuery, db.resourceTableName)
	row := db.db.QueryRowContext(ctx, query, entry.Kind, entry.ApiVersion, entry.Cluster, entry.ClusterUid, entry.Name, entry.Namespace)
	var id int64
	err := row.Scan(&id)
	return id, err
}

func (db *Database) UpdateResource(ctx context.Context, id int64, entry *models.ResourceEntry) error {
	query := fmt.Sprintf(updateResourceQuery, db.resourceTableName)
	tx, err := db.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("could not begin transaction: %s", err)
	}
	_, execErr := tx.ExecContext(
		ctx,
		query,
		entry.ResourceVersion,
		entry.LastUpdated,
		entry.Deleted,
		entry.Data,
		id,
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

func (db *Database) WriteResource(ctx context.Context, entry *models.ResourceEntry) error {
	query := fmt.Sprintf(writeResource, db.resourceTableName)
	tx, err := db.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("could not begin transaction: %s", err)
	}
	_, execErr := tx.ExecContext(
		ctx,
		query,
		entry.ApiVersion,
		entry.Cluster,
		entry.ClusterUid,
		entry.Kind,
		entry.Name,
		entry.Namespace,
		entry.ResourceVersion,
		entry.Created,
		entry.LastUpdated,
		entry.Deleted,
		entry.Data,
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
