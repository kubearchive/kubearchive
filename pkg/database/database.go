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
	resourceTableName = "resource"
	resourcesQuery    = "SELECT data FROM %s WHERE kind=$1 AND api_version=$2"
	writeResource     = `INSERT INTO %s (uuid, api_version, cluster, cluster_uid, kind, name, namespace, resource_version, created_ts, updated_ts, cluster_deleted_ts, data) Values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12) ON CONFLICT(uuid) DO UPDATE SET name=$6, namespace=$7, resource_version=$8, updated_ts=$10, cluster_deleted_ts=$11, data=$12`
)

type DBInterface interface {
	QueryResources(ctx context.Context, kind, group, version string) ([]models.Resource, error)
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

func (db *Database) WriteResource(ctx context.Context, entry *models.ResourceEntry) error {
	query := fmt.Sprintf(writeResource, db.resourceTableName)
	tx, err := db.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("could not begin transaction: %s", err)
	}
	_, execErr := tx.ExecContext(
		ctx,
		query,
		entry.Uuid,
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
