// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package sql

import (
	"context"

	"github.com/huandu/go-sqlbuilder"
	"github.com/jmoiron/sqlx"
	"github.com/kubearchive/kubearchive/pkg/database/interfaces"
	"github.com/kubearchive/kubearchive/pkg/database/sql/facade"
)

const defaultContainerAnnotation = "kubectl.kubernetes.io/default-container"

type sqlDatabase interface {
	interfaces.Database

	getSelector() facade.DBSelector
	getFilter() facade.DBFilter
	getSorter() facade.DBSorter
	getInserter() facade.DBInserter
	getDeleter() facade.DBDeleter
	getFlavor() sqlbuilder.Flavor
	setConn(*sqlx.DB)
}

type sqlDatabaseImpl struct {
	db       *sqlx.DB
	flavor   sqlbuilder.Flavor
	selector facade.DBSelector
	filter   facade.DBFilter
	sorter   facade.DBSorter
	inserter facade.DBInserter
	deleter  facade.DBDeleter
	creator  facade.DBCreator
}

func (db *sqlDatabaseImpl) Init(env map[string]string) error {
	conn, err := establishConnection(db.creator.GetDriverName(), db.creator.GetConnectionString(env))
	if err != nil {
		return err
	}
	db.db = conn
	return nil
}

func (db *sqlDatabaseImpl) QueryDatabaseSchemaVersion(ctx context.Context) (string, error) {
	strQueryPerformer := newQueryPerformer[string](db.db, db.flavor)

	sb := db.selector.VersionSelector()
	return strQueryPerformer.performSingleRowQuery(ctx, sb)
}

func (db *sqlDatabaseImpl) Ping(ctx context.Context) error {
	return db.db.PingContext(ctx)
}

func (db *sqlDatabaseImpl) CloseDB() error {
	return db.db.Close()
}

func (db *sqlDatabaseImpl) getFilter() facade.DBFilter {
	return db.filter
}

func (db *sqlDatabaseImpl) getFlavor() sqlbuilder.Flavor {
	return db.flavor
}

func (db *sqlDatabaseImpl) setConn(conn *sqlx.DB) {
	db.db = conn
}
