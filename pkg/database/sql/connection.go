// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package sql

import (
	"database/sql"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/XSAM/otelsql"
	"github.com/avast/retry-go/v5"
	"github.com/jmoiron/sqlx"
	"github.com/kubearchive/kubearchive/pkg/database/env"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

func establishConnection(driver, connectionString string, dbEnv map[string]string) (*sqlx.DB, error) {
	slog.Info("Establishing database connection")
	configs := []retry.Option{
		retry.Attempts(10),
		retry.OnRetry(func(n uint, err error) {
			slog.Warn("Retry request", "retry", n+1, "error", err.Error())
		}),
		retry.Delay(time.Second),
	}
	var conn *sql.DB
	var err error
	errRetry := retry.New(configs...).Do(
		func() error {
			conn, err = otelsql.Open(driver, connectionString, otelsql.WithAttributes(semconv.DBSystemKey.String(driver)))
			if err != nil {
				return err
			}
			return conn.Ping()
		})
	if errRetry != nil {
		slog.Error("Unable to connect to the database", "error", errRetry.Error())
		return nil, errRetry
	}

	slog.Info("Successfully connected to the database")

	configureConnectionPool(conn, dbEnv)

	_, err = otelsql.RegisterDBStatsMetrics(conn, otelsql.WithAttributes(semconv.DBSystemKey.String(driver)))
	if err != nil {
		slog.Error("Unable to instrument the DB properly", "error", err.Error())
		return nil, err
	}

	// Extract connection info for logging
	connectionInfo := extractConnectionInfo(driver, connectionString)
	slog.Info("Database connection information",
		"driver", driver,
		"host", connectionInfo.host,
		"port", connectionInfo.port,
		"database", connectionInfo.database,
		"user", connectionInfo.user,
		"ssl_mode", connectionInfo.sslMode,
	)
	return sqlx.NewDb(conn, driver), nil
}

func configureConnectionPool(db *sql.DB, dbEnv map[string]string) {
	maxOpenConns := getEnvInt(dbEnv, env.DbMaxOpenConnsEnvVar, env.DbDefaultMaxOpenConns)
	maxIdleConns := getEnvInt(dbEnv, env.DbMaxIdleConnsEnvVar, env.DbDefaultMaxIdleConns)
	connMaxLifetime := getEnvDuration(dbEnv, env.DbConnMaxLifetimeEnvVar, env.DbDefaultConnMaxLifetime)
	connMaxIdleTime := getEnvDuration(dbEnv, env.DbConnMaxIdleTimeEnvVar, env.DbDefaultConnMaxIdleTime)

	db.SetMaxOpenConns(maxOpenConns)
	db.SetMaxIdleConns(maxIdleConns)
	db.SetConnMaxLifetime(connMaxLifetime)
	db.SetConnMaxIdleTime(connMaxIdleTime)

	slog.Info("Database connection pool configured",
		"max_open_conns", maxOpenConns,
		"max_idle_conns", maxIdleConns,
		"conn_max_lifetime", connMaxLifetime,
		"conn_max_idle_time", connMaxIdleTime,
	)
}

func getEnvInt(dbEnv map[string]string, key string, defaultValue int) int {
	val, exists := dbEnv[key]
	if !exists || val == "" {
		return defaultValue
	}
	parsed, err := strconv.Atoi(val)
	if err != nil {
		slog.Warn("Invalid integer value for environment variable, using default",
			"key", key, "value", val, "default", defaultValue)
		return defaultValue
	}
	if parsed <= 0 {
		slog.Warn("Zero or negative value for environment variable is not allowed, using default",
			"key", key, "value", parsed, "default", defaultValue)
		return defaultValue
	}
	return parsed
}

func getEnvDuration(dbEnv map[string]string, key string, defaultValue time.Duration) time.Duration {
	val, exists := dbEnv[key]
	if !exists || val == "" {
		return defaultValue
	}
	parsed, err := time.ParseDuration(val)
	if err != nil {
		slog.Warn("Invalid duration value for environment variable, using default",
			"key", key, "value", val, "default", defaultValue)
		return defaultValue
	}
	if parsed <= 0 {
		slog.Warn("Zero or negative duration for environment variable is not allowed, using default",
			"key", key, "value", parsed, "default", defaultValue)
		return defaultValue
	}
	return parsed
}

// connectionInfo holds parsed connection details for logging
type connectionInfo struct {
	host     string
	port     string
	database string
	user     string
	sslMode  string
}

// extractConnectionInfo parses PostgreSQL connection string to extract non-sensitive information for logging
func extractConnectionInfo(driver, connectionString string) connectionInfo {
	info := connectionInfo{}

	// Only parse PostgreSQL connection strings
	if driver == "postgres" {
		// Parse postgres connection string: user=... password=... dbname=... host=... port=... sslmode=...
		parts := strings.Split(connectionString, " ")
		for _, part := range parts {
			if strings.Contains(part, "=") {
				keyValue := strings.SplitN(part, "=", 2)
				if len(keyValue) == 2 {
					key, value := keyValue[0], keyValue[1]
					switch key {
					case "host":
						info.host = value
					case "port":
						info.port = value
					case "dbname":
						info.database = value
					case "user":
						info.user = value
					case "sslmode":
						info.sslMode = value
					}
				}
			}
		}
	}

	// Set defaults for missing values
	if info.host == "" {
		info.host = "unknown"
	}
	if info.port == "" {
		info.port = "default"
	}
	if info.database == "" {
		info.database = "unknown"
	}
	if info.user == "" {
		info.user = "unknown"
	}
	if info.sslMode == "" {
		info.sslMode = "unknown"
	}

	return info
}
