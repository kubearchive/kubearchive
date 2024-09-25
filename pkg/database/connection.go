// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package database

import (
	"database/sql"
	"log"
	"time"

	"github.com/XSAM/otelsql"
	"github.com/avast/retry-go/v4"
)

func establishConnection(driver, connectionString string) (*sql.DB, error) {
	configs := []retry.Option{
		retry.Attempts(10),
		retry.OnRetry(func(n uint, err error) {
			log.Printf("Retry request %d, get error: %v", n+1, err)
		}),
		retry.Delay(time.Second),
	}
	var conn *sql.DB
	errRetry := retry.Do(
		func() error {
			conn, err := otelsql.Open(driver, connectionString)
			if err != nil {
				return err
			}
			return conn.Ping()
		},
		configs...)
	if errRetry != nil {
		return nil, errRetry
	}
	log.Println("Successfully connected to the database")
	return conn, nil
}
