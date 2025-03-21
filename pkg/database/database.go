// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package database

import (
	"context"
	"errors"
	"fmt"

	"github.com/huandu/go-sqlbuilder"
	"github.com/jmoiron/sqlx"
	"github.com/kubearchive/kubearchive/pkg/database/facade"
)

const defaultContainerAnnotation = "kubectl.kubernetes.io/default-container"

var ResourceNotFoundError = errors.New("resource not found")

type newDatabaseFunc func(*sqlx.DB) Database
type newDBCreatorFunc func(map[string]string) facade.DBCreator

var RegisteredDatabases = make(map[string]newDatabaseFunc)
var RegisteredDBCreators = make(map[string]newDBCreatorFunc)

type Database interface {
	DBReader
	DBWriter
}

type DatabaseImpl struct {
	DB       *sqlx.DB
	Flavor   sqlbuilder.Flavor
	Selector facade.DBSelector
	Filter   facade.DBFilter
	Sorter   facade.DBSorter
	Inserter facade.DBInserter
	Deleter  facade.DBDeleter
}

func newDatabase() (Database, error) {
	env, err := newDatabaseEnvironment()
	if err != nil {
		return nil, err
	}

	var creator facade.DBCreator
	if c, ok := RegisteredDBCreators[env[DbKindEnvVar]]; ok {
		creator = c(env)
	} else {
		return nil, fmt.Errorf("no database registered with name '%s'", env[DbKindEnvVar])
	}

	conn, err := establishConnection(creator.GetDriverName(), creator.GetConnectionString())
	if err != nil {
		return nil, err
	}

	var database Database
	if init, ok := RegisteredDatabases[env[DbKindEnvVar]]; ok {
		database = init(conn)
	} else {
		return nil, fmt.Errorf("no database registered with name '%s'", env[DbKindEnvVar])
	}

	return database, nil
}

func (db *DatabaseImpl) Ping(ctx context.Context) error {
	return db.DB.PingContext(ctx)
}

func (db *DatabaseImpl) CloseDB() error {
	return db.DB.Close()
}
