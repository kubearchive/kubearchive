// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package env

import (
	"errors"
	"fmt"
	"os"
	"time"
)

const (
	dbConnectionErrStr string = "could not create database connection string: %s must be set"

	DbKindEnvVar     string = "DATABASE_KIND"
	DbNameEnvVar     string = "DATABASE_DB"
	DbUserEnvVar     string = "DATABASE_USER"
	DbPasswordEnvVar string = "DATABASE_PASSWORD" // nosec G101 not a password
	DbHostEnvVar     string = "DATABASE_URL"
	DbPortEnvVar     string = "DATABASE_PORT"

	DbMaxOpenConnsEnvVar    string = "DATABASE_MAX_OPEN_CONNS"
	DbMaxIdleConnsEnvVar    string = "DATABASE_MAX_IDLE_CONNS"
	DbConnMaxLifetimeEnvVar string = "DATABASE_CONN_MAX_LIFETIME"
	DbConnMaxIdleTimeEnvVar string = "DATABASE_CONN_MAX_IDLE_TIME"

	// WARNING: These defaults assume a typical deployment of 2 API pods + 1 sink pod
	// (3 pods × 10 connections = 30 total). If you override these via environment variables,
	// ensure the total across all pods does not exceed your database's max_connections setting
	// (PostgreSQL default: 100, max: 262,143).
	DbDefaultMaxOpenConns    = 10
	DbDefaultMaxIdleConns    = 5
	DbDefaultConnMaxLifetime = 5 * time.Minute
	DbDefaultConnMaxIdleTime = 2 * time.Minute
)

var DbEnvVars = [...]string{DbKindEnvVar, DbNameEnvVar, DbUserEnvVar, DbPasswordEnvVar, DbHostEnvVar, DbPortEnvVar}
var DbPoolConnEnvVars = [...]string{DbMaxOpenConnsEnvVar, DbMaxIdleConnsEnvVar, DbConnMaxLifetimeEnvVar, DbConnMaxIdleTimeEnvVar}

// Reads database connection info from the environment variables and returns a map of variable name to value.
func NewDatabaseEnvironment() (map[string]string, error) {
	var err error
	env := make(map[string]string)
	for _, name := range DbEnvVars {
		value, exists := os.LookupEnv(name)
		if exists {
			env[name] = value
		} else {
			err = errors.Join(err, fmt.Errorf(dbConnectionErrStr, name))
		}
	}
	if err != nil {
		return nil, err
	}

	for _, name := range DbPoolConnEnvVars {
		value, exists := os.LookupEnv(name)
		if exists {
			env[name] = value
		}
	}

	return env, nil
}
