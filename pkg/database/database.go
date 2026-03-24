// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package database

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"strconv"
	"sync"

	"github.com/kubearchive/kubearchive/pkg/database/env"
	"github.com/kubearchive/kubearchive/pkg/database/interfaces"
	"github.com/kubearchive/kubearchive/pkg/database/sql"
)

type SchemaVersionRange struct {
	Min int
	Max int
}

var DatabaseSchemaVersions = map[string]SchemaVersionRange{
	"postgresql": {Min: 10, Max: 13},
	"mariadb":    {Min: 1, Max: 1},
}

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
		slog.Info("Initializing database connection")

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

		versionRange, hasRange := DatabaseSchemaVersions[dbType]
		if !hasRange {
			err = fmt.Errorf("no schema version range defined for database type '%s'", dbType)
			slog.Error("No schema version range defined", "type", dbType)
			return
		}

		dbVersionInt, errParse := strconv.Atoi(dbVersion)
		if errParse != nil {
			err = fmt.Errorf("invalid database schema version '%s': expected an integer: %w", dbVersion, errParse)
			slog.Error("Failed to parse database schema version",
				"version", dbVersion,
				"error", errParse.Error(),
			)
			return
		}

		slog.Info("Database schema version check",
			"min_version", versionRange.Min,
			"max_version", versionRange.Max,
			"actual_version", dbVersionInt,
		)

		if dbVersionInt < versionRange.Min || dbVersionInt > versionRange.Max {
			err = fmt.Errorf("database schema version %d is outside accepted range [%d, %d]", dbVersionInt, versionRange.Min, versionRange.Max)
			slog.Error("Database schema version out of range",
				"actual_version", dbVersionInt,
				"min_version", versionRange.Min,
				"max_version", versionRange.Max,
			)
			return
		}

		slog.Info("Database connection fully established",
			"type", dbType,
			"schema_version", dbVersionInt,
			"status", "ready",
		)
	})

	return db, err
}
