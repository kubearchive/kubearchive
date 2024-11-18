// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package database

import (
	"database/sql"
	"log"
	"time"

	"github.com/XSAM/otelsql"
	"github.com/avast/retry-go/v4"
	"github.com/jmoiron/sqlx"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

func establishConnection(driver, connectionString string) *sqlx.DB {
	configs := []retry.Option{
		retry.Attempts(10),
		retry.OnRetry(func(n uint, err error) {
			log.Printf("Retry request %d, get error: %v", n+1, err)
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
		log.Printf("Unable to connect to the database, error: %v", errRetry)
		return nil
	}

	err = otelsql.RegisterDBStatsMetrics(conn, otelsql.WithAttributes(semconv.DBSystemKey.String(driver)))
	if err != nil {
		log.Printf("Unable to instrument the DB properly, error: %v", err)
		return nil
	}

	log.Println("Successfully connected to the database")
	return sqlx.NewDb(conn, driver)
}
