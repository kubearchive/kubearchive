// Copyright Kronicler Authors
// SPDX-License-Identifier: Apache-2.0

package database

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/huandu/go-sqlbuilder"
	"github.com/jmoiron/sqlx"
	"github.com/kronicler/kronicler/pkg/database/facade"
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
	db       *sqlx.DB
	flavor   sqlbuilder.Flavor
	selector facade.DBSelector
	filter   facade.DBFilter
	sorter   facade.DBSorter
	inserter facade.DBInserter
	deleter  facade.DBDeleter
}

var db Database
var once sync.Once

func newDatabase() (Database, error) {
	var err error

	once.Do(func() {
		env, newDBErr := newDatabaseEnvironment()
		if newDBErr != nil {
			err = newDBErr
			return
		}

		var creator facade.DBCreator
		if c, ok := RegisteredDBCreators[env[DbKindEnvVar]]; ok {
			creator = c(env)
		} else {
			err = fmt.Errorf("no database registered with name '%s'", env[DbKindEnvVar])
			return
		}

		conn, errConn := establishConnection(creator.GetDriverName(), creator.GetConnectionString())
		if errConn != nil {
			err = errConn
			return
		}

		if init, ok := RegisteredDatabases[env[DbKindEnvVar]]; ok {
			db = init(conn)
		} else {
			err = fmt.Errorf("no database registered with name '%s'", env[DbKindEnvVar])
		}
	})

	return db, err
}

func (db *DatabaseImpl) Ping(ctx context.Context) error {
	return db.db.PingContext(ctx)
}

func (db *DatabaseImpl) CloseDB() error {
	return db.db.Close()
}

func (db *DatabaseImpl) getFilter() facade.DBFilter {
	return db.filter
}

func (db *DatabaseImpl) getFlavor() sqlbuilder.Flavor {
	return db.flavor
}

func (db *DatabaseImpl) setConn(conn *sqlx.DB) {
	db.db = conn
}
