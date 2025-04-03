// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package sql

import (
	"database/sql"
	"log/slog"
	"time"

	"github.com/XSAM/otelsql"
	"github.com/avast/retry-go/v4"
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
