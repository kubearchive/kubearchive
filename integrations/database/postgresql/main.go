// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

func main() {
	fmt.Println("Starting migration process...")
	m, err := migrate.New(
		fmt.Sprintf("file://%s", path.Join(os.Getenv("KO_DATA_PATH"), "migrations")),
		fmt.Sprintf(
			"postgres://%s:%s@%s:%s/%s",
			os.Getenv("DATABASE_USER"),
			url.QueryEscape(os.Getenv("DATABASE_PASSWORD")),
			os.Getenv("DATABASE_URL"),
			os.Getenv("DATABASE_PORT"),
			os.Getenv("DATABASE_DB"),
		))
	if err != nil {
		panic(err)
	}

	err = m.Up()
	if err != nil && !errors.Is(err, migrate.ErrNoChange) {
		panic(err)
	}

	fmt.Println("Migration completed successfully")
}
