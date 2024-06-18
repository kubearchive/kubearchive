// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package database

import (
	"fmt"
	"os"
)

type EnvVar string

const (
	connStrTpl         = "user=%s password=%s dbname=%s host=%s port=%s sslmode=disable"
	dbConnectionErrStr = "Could not create database connection string: %s must be set"

	DbNameEnvVar     EnvVar = "POSTGRES_DB"
	DbUserEnvVar     EnvVar = "POSTGRES_USER"
	DbPasswordEnvVar EnvVar = "POSTGRES_PASSWORD" // #nosec G101 not a password
	DbUrlEnvVar      EnvVar = "POSTGRES_URL"
	DbPortEnvVar     EnvVar = "POSTGRES_PORT"
)

// Returns the value of the environment variable dbEnvVar. If dbEnvVar is not set, readDbEnvVar returns an error
func readDbEnvVar(dbEnvVar EnvVar) (string, error) {
	envVarVal, exists := os.LookupEnv(string(dbEnvVar))
	if !exists {
		return "", fmt.Errorf(dbConnectionErrStr, string(dbEnvVar))
	}
	return envVarVal, nil
}

// reads database connection info from the following environment variables: POSTGRES_DB, POSTGRES_USER,
// POSTGRES_PASSWORD, POSTGRES_URL, and POSTGRES_PORT. Then returns an SQL database connection string. If any of these
// environment variable were not set, it returns an error.
func ConnectionStr() (string, error) {
	dbName, err := readDbEnvVar(DbNameEnvVar)
	if err != nil {
		return "", err
	}
	dbUser, err := readDbEnvVar(DbUserEnvVar)
	if err != nil {
		return "", err
	}
	dbPassword, err := readDbEnvVar(DbPasswordEnvVar)
	if err != nil {
		return "", err
	}
	dbUrl, err := readDbEnvVar(DbUrlEnvVar)
	if err != nil {
		return "", err
	}
	dbPort, err := readDbEnvVar(DbPortEnvVar)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(connStrTpl, dbUser, dbPassword, dbName, dbUrl, dbPort), nil
}
