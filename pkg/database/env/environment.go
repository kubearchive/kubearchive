// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package env

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
	DbPasswordEnvVar string = "DATABASE_PASSWORD" // nosec G101 not a password
	DbHostEnvVar     string = "DATABASE_URL"
	DbPortEnvVar     string = "DATABASE_PORT"
)

var DbEnvVars = [...]string{DbKindEnvVar, DbNameEnvVar, DbUserEnvVar, DbPasswordEnvVar, DbHostEnvVar, DbPortEnvVar}

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
	if err == nil {
		return env, nil
	} else {
		return nil, err
	}
}
