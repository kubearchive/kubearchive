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
)

type DBInterface interface {
	QueryResources(ctx context.Context, kind, group, version string) ([]models.Resource, error)
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
