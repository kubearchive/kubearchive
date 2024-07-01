// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	ceOtelObs "github.com/cloudevents/sdk-go/observability/opentelemetry/v2/client"
	cloudevents "github.com/cloudevents/sdk-go/v2"
	ceClient "github.com/cloudevents/sdk-go/v2/client"
	"github.com/cloudevents/sdk-go/v2/types"
	kaObservability "github.com/kubearchive/kubearchive/pkg/observability"
	_ "github.com/lib/pq"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

const (
	connStrTpl = "user=%s password=%s dbname=%s host=%s port=%s sslmode=disable"

	apiVersionExtension = "apiversion"
	kindExtension       = "kind"
	nameExtension       = "name"
	namespaceExtension  = "namespace"

	DbNameEnvVar     = "POSTGRES_DB"
	DbUserEnvVar     = "POSTGRES_USER"
	DbPasswordEnvVar = "POSTGRES_PASSWORD"
	DbUrlEnvVar      = "POSTGRES_URL"
	DbPortEnvVar     = "POSTGRES_PORT"

	dbConnectionErrStr = "Could not create database connection string: %s must be set"
)

var (
	logger         = log.New(os.Stderr, "", log.LstdFlags|log.Lmicroseconds|log.LUTC)
	db     *sql.DB = nil
)

// Represents the fields in a resource's metadata that are need to write a resource into the database
type ResourceMetadata struct {
	ResourceVersion string `json:"resourceVersion"`
}

// Respresents the fields in the data field of a cloudevent to write a resource into the database
type ResourceData struct {
	Created     string           `json:"firstTimestamp"`
	LastUpdated string           `json:"lastTimestamp"`
	Metadata    ResourceMetadata `json:"metadata"`
}

// reads database connection info from the following environment variables: POSTGRES_DB, POSTGRES_USER,
// POSTGRES_PASSWORD, POSTGRES_URL, and POSTGRES_PORT. Then returns an SQL database connection string. If any of these
// environment variable were not set, it returns an error.
func dbConnectionStr() (string, error) {
	dbName, exists := os.LookupEnv(DbNameEnvVar)
	if !exists {
		return "", fmt.Errorf(dbConnectionErrStr, DbNameEnvVar)
	}
	dbUser, exists := os.LookupEnv(DbUserEnvVar)
	if !exists {
		return "", fmt.Errorf(dbConnectionErrStr, DbUserEnvVar)
	}
	return fmt.Sprintf(connStrTpl, dbUser, dbPassword, dbName, dbUrl, dbPort), nil
}

// checks that the cloudevents has the appropriate extensions set and have values that are the right type. Additionally
// it checks that the cloudevent's data has the necessary fields. If all conditions are not met, it returns an error
func validateCloudevent(event cloudevents.Event) error {
	eventExtensions := event.Extensions()
	_, err := types.ToString(eventExtensions[apiVersionExtension])
	if err != nil {
		return err
	}
	_, err = types.ToString(eventExtensions[kindExtension])
	if err != nil {
		return err
	}
	_, err = types.ToString(eventExtensions[nameExtension])
	if err != nil {
		return err
	}
	_, err = types.ToString(eventExtensions[namespaceExtension])
	if err != nil {
		return err
	}
	payload := ResourceMetadata{}
	err = event.DataAs(&payload)
	if err != nil {
		return err
	}

	return nil
}

// Writes the kubernetes resource to the database. This method must be called after calling validateCloudevent and it
// does not return an error
func writeToDatabase(event cloudevents.Event) {
	var err error
	if db == nil {
		logger.Println("opening connection to the database")
		db, err = sql.Open("postgres", connStr)
		if err != nil {
			logger.Printf("failed to connect to database: %s", err)
			return
		}
	}

	eventExtensions := event.Extensions()
	// since a call to validateCloudevent should already have passed we can assume that all of these values exist as
	// strings
	apiVersion := eventExtensions[apiVersionExtension]
	kind := eventExtensions[kindExtension]
	name := eventExtensions[nameExtension]
	namespace := eventExtensions[namespaceExtension]

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		logger.Printf("could not begin transaction: %s", err)
		cancel()
		return
	}
	resourceData := ResourceData{}
	err = event.DataAs(&resourceData)
	if err != nil {
		logger.Printf("Canceling database transaction. Could not parse data from the cloudevent: %s\n", err)
		rollbackErr := tx.Rollback()
		if rollbackErr != nil {
			logger.Printf("Could not rollback transaction: %s\n", rollbackErr)
		}
		cancel()
		return
	}
	resource := string(event.Data())
	_, execErr := tx.ExecContext(ctx,
		"INSERT INTO ArchiveMeta (api_version, kind, name, namespace, resource_version, data, created_ts, updated_ts) Values ($1, $2, $3, $4, $5, $6, $7, $8)",
		apiVersion,
		kind,
		name,
		namespace,
		resourceData.Metadata.ResourceVersion,
		resource,
		resourceData.Created,
		resourceData.LastUpdated,
	)
	if execErr != nil {
		logger.Printf("Rolling back transaction. Write to database failed: %s\n", execErr)
		rollbackErr := tx.Rollback()
		if rollbackErr != nil {
			logger.Printf("unable to rollback transaction: %s", rollbackErr)
		}
		cancel()
		return
	}
	execErr = tx.Commit()
	if execErr != nil {
		logger.Printf("Database transaction failed: %s\n", execErr)
		rollbackErr := tx.Rollback()
		if rollbackErr != nil {
			logger.Printf("unable to rollback transaction: %s\n", rollbackErr)
		}
		cancel()
		return
	}
	cancel()
	logger.Println("transaction completed successfully")
}

// Processes incoming cloudevents and
func receive(event cloudevents.Event) {
	logger.Println("received CloudEvent: ", event.ID())
	err := validateCloudevent(event)
	if err != nil {
		logger.Printf("cloudevent is malformed and will not be processed: %s\n", err)
		return
	}
	writeToDatabase(event)
}

func main() {
	err := kaObservability.Start()
	if err != nil {
		logger.Printf("Could not start tracing: %s\n", err.Error())
	}
	httpClient, err := cloudevents.NewHTTP(
		cloudevents.WithRoundTripper(otelhttp.NewTransport(http.DefaultTransport)),
		cloudevents.WithMiddleware(func(next http.Handler) http.Handler {
			return otelhttp.NewHandler(next, "receive")
		}),
	)
	if err != nil {
		logger.Fatalf("failed to create HTTP client: %s\n", err.Error())
	}
	eventClient, err := cloudevents.NewClient(httpClient, ceClient.WithObservabilityService(ceOtelObs.NewOTelObservabilityService()))
	if err != nil {
		logger.Fatalf("failed to create CloudEvents HTTP client: %s\n", err.Error())
	}

	err = eventClient.StartReceiver(context.Background(), receive)
	if err != nil {
		logger.Fatalf("failed to start receiving CloudEvents: %s\n", err.Error())
	}
}
