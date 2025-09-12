// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package sql

import (
	"database/sql"
	"log/slog"
	"strings"
	"time"

	"github.com/XSAM/otelsql"
	"github.com/avast/retry-go/v4"
	"github.com/jmoiron/sqlx"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

func establishConnection(driver, connectionString string) (*sqlx.DB, error) {
	startTime := time.Now()

	// Enhanced logging only for PostgreSQL
	if driver == "postgres" {
		// Extract connection details for logging (without sensitive info)
		connectionInfo := extractConnectionInfo(driver, connectionString)

		slog.Info("Establishing database connection",
			"driver", driver,
			"host", connectionInfo.host,
			"port", connectionInfo.port,
			"database", connectionInfo.database,
			"user", connectionInfo.user,
			"ssl_mode", connectionInfo.sslMode,
			"connection_string_length", len(connectionString),
		)

		configs := []retry.Option{
			retry.Attempts(10),
			retry.OnRetry(func(n uint, err error) {
				slog.Warn("Database retry request",
					"retry", n+1,
					"error", err.Error(),
					"driver", driver,
					"host", connectionInfo.host,
					"port", connectionInfo.port,
				)
			}),
			retry.Delay(time.Second),
		}
		var conn *sql.DB
		var err error
		errRetry := retry.Do(
			func() error {
				conn, err = otelsql.Open(driver, connectionString, otelsql.WithAttributes(semconv.DBSystemKey.String(driver)))
				if err != nil {
					return err
				}
				return conn.Ping()
			},
			configs...)
		if errRetry != nil {
			connectionDuration := time.Since(startTime)
			slog.Error("Unable to connect to the Database",
				"error", errRetry.Error(),
				"driver", driver,
				"host", connectionInfo.host,
				"port", connectionInfo.port,
				"database", connectionInfo.database,
				"connection_attempts", 10,
				"total_duration", connectionDuration,
			)
			return nil, errRetry
		}

		err = otelsql.RegisterDBStatsMetrics(conn, otelsql.WithAttributes(semconv.DBSystemKey.String(driver)))
		if err != nil {
			slog.Error("Unable to instrument Database properly",
				"error", err.Error(),
				"driver", driver,
				"host", connectionInfo.host,
			)
			return nil, err
		}

		connectionDuration := time.Since(startTime)
		slog.Info("Successfully connected to Database",
			"driver", driver,
			"host", connectionInfo.host,
			"port", connectionInfo.port,
			"database", connectionInfo.database,
			"user", connectionInfo.user,
			"ssl_mode", connectionInfo.sslMode,
			"connection_duration", connectionDuration,
			"instrumentation", "enabled",
		)
		return sqlx.NewDb(conn, driver), nil
	} else {
		// simple logging for non-PostgreSQL databases
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
		errRetry := retry.Do(
			func() error {
				conn, err = otelsql.Open(driver, connectionString, otelsql.WithAttributes(semconv.DBSystemKey.String(driver)))
				if err != nil {
					return err
				}
				return conn.Ping()
			},
			configs...)
		if errRetry != nil {
			slog.Error("Unable to connect to the database", "error", errRetry.Error())
			return nil, errRetry
		}

		err = otelsql.RegisterDBStatsMetrics(conn, otelsql.WithAttributes(semconv.DBSystemKey.String(driver)))
		if err != nil {
			slog.Error("Unable to instrument the DB properly", "error", err.Error())
			return nil, err
		}

		slog.Info("Successfully connected to the database")
		return sqlx.NewDb(conn, driver), nil
	}
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
