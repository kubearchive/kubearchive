// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package sql

import (
	"github.com/kubearchive/kubearchive/pkg/database/sql/facade"
)

func (db *sqlDatabaseImpl) getInserter() facade.DBInserter {
	return db.inserter
}

func (db *sqlDatabaseImpl) getDeleter() facade.DBDeleter {
	return db.deleter
}
