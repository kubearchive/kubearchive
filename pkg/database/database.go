// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package database

import (
	"fmt"
	"sync"

	"github.com/kubearchive/kubearchive/pkg/database/env"
	"github.com/kubearchive/kubearchive/pkg/database/interfaces"
	"github.com/kubearchive/kubearchive/pkg/database/sql"
)

var CurrentDatabaseSchemaVersion = "1"
var RegisteredDatabases = map[string]interfaces.Database{
	"postgresql": sql.NewPostgreSQLDatabase(),
	"mariadb":    sql.NewMariaDBDatabase(),
}

var db interfaces.Database
var once sync.Once

func NewReader() (interfaces.DBReader, error) {
	return newDatabase()
}

func NewWriter() (interfaces.DBWriter, error) {
	return newDatabase()
}

func newDatabase() (interfaces.Database, error) {
	var err error

	once.Do(func() {
		e, errEnv := env.NewDatabaseEnvironment()
		if errEnv != nil {
			err = errEnv
			return
		}

		if regDB, ok := RegisteredDatabases[e[env.DbKindEnvVar]]; ok {
			err = regDB.Init(e)
			if err != nil {
				return
			}
			db = regDB
		} else {
			err = fmt.Errorf("no interfaces registered with name '%s'", e[env.DbKindEnvVar])
		}
	})

	return db, err
}
