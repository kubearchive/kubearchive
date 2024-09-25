// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package database

import (
	"errors"
	"fmt"
	"os"
)

const (
	dbConnectionErrStr string = "Could not create database connection string: %s must be set"

	DbKindEnvVar     string = "DATABASE_KIND"
	DbNameEnvVar     string = "DATABASE_DB"
	DbUserEnvVar     string = "DATABASE_USER"
	DbPasswordEnvVar string = "DATABASE_PASSWORD" // #nosec G101 not a password
	DbHostEnvVar     string = "DATABASE_URL"
	DbPortEnvVar     string = "DATABASE_PORT"
)

type databaseEnvironment struct {
	kind     string
	name     string
	user     string
	password string
	host     string
	port     string
}

func newDatabaseEnvironment() (*databaseEnvironment, error) {
	var err error

	kind := getEnv(DbKindEnvVar, &err)
	name := getEnv(DbNameEnvVar, &err)
	user := getEnv(DbUserEnvVar, &err)
	password := getEnv(DbPasswordEnvVar, &err)
	host := getEnv(DbHostEnvVar, &err)
	port := getEnv(DbPortEnvVar, &err)

	if err == nil {
		return &databaseEnvironment{kind, name, user, password, host, port}, nil
	} else {
		return nil, err
	}
}

func getEnv(key string, err *error) string {
	value, exists := os.LookupEnv(key)
	if !exists {
		*err = errors.Join(*err, fmt.Errorf(dbConnectionErrStr, key))
	}
	return value
}
