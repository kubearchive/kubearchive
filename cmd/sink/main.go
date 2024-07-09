// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	ceOtelObs "github.com/cloudevents/sdk-go/observability/opentelemetry/v2/client"
	cloudevents "github.com/cloudevents/sdk-go/v2"
	ceClient "github.com/cloudevents/sdk-go/v2/client"
	"github.com/kubearchive/kubearchive/pkg/database"
	"github.com/kubearchive/kubearchive/pkg/models"
	kaObservability "github.com/kubearchive/kubearchive/pkg/observability"
	_ "github.com/lib/pq"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

const (
	ClusterNameEnvVar = "KUBEARCHIVE_CLUSTER_NAME"
)

type Sink struct {
	ClusterName string
	Db          database.DBInterface
	EventClient ceClient.Client
	Logger      *log.Logger
	Lock        sync.Locker
}

func NewSink(clusterName string, db database.DBInterface, logger *log.Logger) *Sink {
	if logger == nil {
		logger = log.New(os.Stderr, "", log.LstdFlags|log.Lmicroseconds|log.LUTC)
		logger.Println("sink was provided a nil logger, falling back to defualt logger")
	}
	if db == nil {
		logger.Fatalln("cannot start sink when db connection is nil")
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

	return &Sink{
		ClusterName: clusterName,
		Db:          db,
		EventClient: eventClient,
		Logger:      logger,
		Lock:        &sync.Mutex{},
	}
}

// Processes incoming cloudevents and writes them to the database
func (sink *Sink) Receive(event cloudevents.Event) {
	sink.Logger.Println("received CloudEvent: ", event.ID())
	archiveEntry, err := models.ResourceEntryFromCloudevent(event, sink.ClusterName)
	if err != nil {
		sink.Logger.Printf("cloudevent %s is malformed and will not be processed: %s\n", event.ID(), err)
		return
	}
	sink.Logger.Printf("cloudevent %s contains all required fields. Attempting to write it to the database\n", event.ID())
	sink.Lock.Lock()
	defer sink.Lock.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	sink.Logger.Printf("checking if resource from cloudevent %s has been written to the database already\n", event.ID())
	entryId, err := sink.Db.QueryResourceId(ctx, archiveEntry)
	cancel()
	ctx, cancel = context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	if err == nil {
		sink.Logger.Printf("resource from cloudevent %s was found in the database. Updating existing entry\n", event.ID())
		err = sink.Db.UpdateResource(ctx, entryId, archiveEntry)
	} else if errors.Is(err, sql.ErrNoRows) {
		sink.Logger.Printf("resource from cloudevent %s was not found in the database. Writing it as a new entry\n", event.ID())
		err = sink.Db.WriteResource(ctx, archiveEntry)
	} else {
		sink.Logger.Printf("error when looking up resource in the database for cloudevent %s: %s\n", event.ID(), err)
		return
	}
	if err != nil {
		sink.Logger.Printf("failed to write cloudevent %s to the database: %s\n", event.ID(), err)
		return
	}
	sink.Logger.Printf("successfully wrote cloudevent %s to the database\n", event.ID())
}

func getClusterName() (string, error) {
	clusterName := os.Getenv(ClusterNameEnvVar)
	if clusterName == "" {
		return clusterName,
			fmt.Errorf(
				"the cluster name must be provided using %s environment variable and cannot be set to an empty string",
				ClusterNameEnvVar,
			)
	}
	return clusterName, nil
}

func main() {
	logger := log.New(os.Stderr, "", log.LstdFlags|log.Lmicroseconds|log.LUTC)
	err := kaObservability.Start()
	if err != nil {
		logger.Printf("Could not start tracing: %s\n", err.Error())
	}
	clusterName, err := getClusterName()
	if err != nil {
		logger.Fatalf("Could not determine the kubernetes cluster name: %s\n", err)
	}
	db, err := database.NewDatabase()
	if err != nil {
		logger.Fatalf("Could not connect to the database: %s\n", err)
	}
	sink := NewSink(clusterName, db, logger)
	err = sink.EventClient.StartReceiver(context.Background(), sink.Receive)
	if err != nil {
		logger.Fatalf("failed to start receiving CloudEvents: %s\n", err.Error())
	}
}
