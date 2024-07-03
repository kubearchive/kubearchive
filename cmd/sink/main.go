// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"os"
	"time"

	ceOtelObs "github.com/cloudevents/sdk-go/observability/opentelemetry/v2/client"
	cloudevents "github.com/cloudevents/sdk-go/v2"
	ceClient "github.com/cloudevents/sdk-go/v2/client"
	kaDatabase "github.com/kubearchive/kubearchive/pkg/database"
	kaObservability "github.com/kubearchive/kubearchive/pkg/observability"
	_ "github.com/lib/pq"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

type Sink struct {
	Db          *sql.DB
	EventClient ceClient.Client
	Logger      *log.Logger
}

func NewSink(db *sql.DB, logger *log.Logger) *Sink {
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
		Db:          db,
		EventClient: eventClient,
		Logger:      logger,
	}
}

// Processes incoming cloudevents and writes them to the database
func (sink Sink) Receive(event cloudevents.Event) {
	sink.Logger.Println("received CloudEvent: ", event.ID())
	archiveEntry, err := NewArchiveEntry(event)
	if err != nil {
		sink.Logger.Printf("cloudevent %s is malformed and will not be processed: %s\n", event.ID(), err)
		return
	}
	sink.Logger.Printf("cloudevent %s contains all required fields. Attempting to write it to the database\n", event.ID())
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	err = archiveEntry.WriteToDatabase(ctx, sink.Db)
	if err != nil {
		sink.Logger.Printf("failed to write cloudevent %s to the database: %s\n", event.ID(), err)
		return
	}
	sink.Logger.Printf("successfully wrote cloudevent %s to the database\n", event.ID())
}

func main() {
	logger := log.New(os.Stderr, "", log.LstdFlags|log.Lmicroseconds|log.LUTC)
	err := kaObservability.Start()
	if err != nil {
		logger.Printf("Could not start tracing: %s\n", err.Error())
	}
	connStr, err := kaDatabase.ConnectionStr()
	if err != nil {
		logger.Fatalf("Could not determine a database to connect to: %s\n", err)
	}
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		logger.Fatalf("Could not connect to the database: %s\n", err)
	}
	sink := NewSink(db, logger)
	err = sink.EventClient.StartReceiver(context.Background(), sink.Receive)
	if err != nil {
		logger.Fatalf("failed to start receiving CloudEvents: %s\n", err.Error())
	}
}
