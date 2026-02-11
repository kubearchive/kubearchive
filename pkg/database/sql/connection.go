// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package sql

import (
	"database/sql"
	"log/slog"
	"strings"
	"time"

	"github.com/XSAM/otelsql"
	"github.com/avast/retry-go/v5"
	"github.com/jmoiron/sqlx"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

func establishConnection(driver, connectionString string) (*sqlx.DB, error) {
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

	err = otelsql.RegisterDBStatsMetrics(conn, otelsql.WithAttributes(semconv.DBSystemKey.String(driver)))
	if err != nil {
		slog.Error("Unable to instrument the DB properly", "error", err.Error())
		return nil, err
	}
	slog.Info("Successfully connected to the database")

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
