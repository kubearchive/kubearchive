// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package database

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"sync"

	"github.com/kubearchive/kubearchive/pkg/database/env"
	"github.com/kubearchive/kubearchive/pkg/database/interfaces"
	"github.com/kubearchive/kubearchive/pkg/database/sql"
)

var CurrentDatabaseSchemaVersion = "5"
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
		slog.Info("Initializing database connection",
			"expected_schema_version", CurrentDatabaseSchemaVersion,
		)

		e, errEnv := env.NewDatabaseEnvironment()
		if errEnv != nil {
			err = errEnv
			slog.Error("Failed to read database environment variables", "error", errEnv.Error())
			return
		}

		dbType := e[env.DbKindEnvVar]
		slog.Info("Database environment configured",
			"database_type", dbType,
			"host", e[env.DbHostEnvVar],
			"port", e[env.DbPortEnvVar],
			"database_name", e[env.DbNameEnvVar],
			"user", e[env.DbUserEnvVar],
		)

		if regDB, ok := RegisteredDatabases[dbType]; ok {
			slog.Info("Initializing registered database", "type", dbType)
			err = regDB.Init(e)
			if err != nil {
				slog.Error("Failed to initialize database",
					"type", dbType,
					"error", err.Error(),
				)
				return
			}
			db = regDB
			slog.Info("Database initialized successfully")
		} else {
			err = fmt.Errorf("no interfaces registered with name '%s'", dbType)
			slog.Error("Unsupported database type",
				"requested_type", dbType,
				"supported_types", maps.Keys(RegisteredDatabases),
			)
			return
		}

		slog.Info("Verifying database schema version")
		var dbVersion string
		dbVersion, err = db.QueryDatabaseSchemaVersion(context.TODO())
		if err != nil {
			slog.Error("Failed to query database schema version",
				"error", err.Error(),
			)
			return
		}

		slog.Info("Database schema version check",
			"expected_version", CurrentDatabaseSchemaVersion,
			"actual_version", dbVersion,
		)

		if dbVersion != CurrentDatabaseSchemaVersion {
			err = fmt.Errorf("expected database schema version '%s', found '%s'", CurrentDatabaseSchemaVersion, dbVersion)
			slog.Error("Database schema version mismatch",
				"expected_version", CurrentDatabaseSchemaVersion,
				"actual_version", dbVersion,
			)
			return
		}

		slog.Info("Database connection fully established",
			"type", dbType,
			"schema_version", dbVersion,
			"status", "ready",
		)
	})

	return db, err
}
