// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package database

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/huandu/go-sqlbuilder"
	"github.com/jmoiron/sqlx"
	"github.com/kubearchive/kubearchive/pkg/database/facade"
)

const defaultContainerAnnotation = "kubectl.kubernetes.io/default-container"

var ResourceNotFoundError = errors.New("resource not found")

type newDBCreatorFunc func(map[string]string) facade.DBCreator

var RegisteredDatabases = make(map[string]Database)
var RegisteredDBCreators = make(map[string]newDBCreatorFunc)

type Database interface {
	DBReader
	DBWriter
}

type DatabaseImpl struct {
	conn     *sqlx.DB
	flavor   sqlbuilder.Flavor
	selector facade.DBSelector
	filter   facade.DBFilter
	sorter   facade.DBSorter
	deleter  facade.DBDeleter
	inserter facade.DBInserter
}

func (db *DatabaseImpl) getFilter() facade.DBFilter {
	return db.filter
}

func (db *DatabaseImpl) setConn(conn *sqlx.DB) {
	db.conn = conn
}

func (db *DatabaseImpl) getFlavor() sqlbuilder.Flavor {
	return db.flavor
}

func (db *DatabaseImpl) Ping(ctx context.Context) error {
	return db.conn.PingContext(ctx)
}

func (db *DatabaseImpl) CloseDB() error {
	return db.conn.Close()
}

var once sync.Once
var singleDatabase Database

// getSingleDatabase implements singleton pattern to prevent executing getDatabaseByEnv more than once
func getSingleDatabase() (Database, error) {
	var err error
	if singleDatabase == nil {
		once.Do(func() {
			slog.Debug("Retrieving single instance of the Database and configuring it...")
			db, dbErr := getDatabaseByEnv()
			if dbErr != nil {
				err = dbErr
			}
			singleDatabase = db
		})
	}
	return singleDatabase, err
}

func getDatabaseByEnv() (Database, error) {
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
	if db, ok := RegisteredDatabases[env[DbKindEnvVar]]; ok {
		db.setConn(conn)
		database = db
	} else {
		return nil, fmt.Errorf("no database registered with name '%s'", env[DbKindEnvVar])
	}

	return database, nil
}
