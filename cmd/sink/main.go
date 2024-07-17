// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"log"
	"net/http"
	"os"
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
	ClusterUid  string
	Db          database.DBInterface
	EventClient ceClient.Client
	Logger      *log.Logger
}

func NewSink(db database.DBInterface, logger *log.Logger) *Sink {
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
		// TODO: cluster name should be set by the user for multicluster support
		ClusterName: "",
		// TODO: clusterUid should be set by the user for multicluster support
		ClusterUid:  "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
		Db:          db,
		EventClient: eventClient,
		Logger:      logger,
	}
}

// Processes incoming cloudevents and writes them to the database
func (sink *Sink) Receive(event cloudevents.Event) {
	sink.Logger.Println("received CloudEvent: ", event.ID())
	k8sObj, err := models.UnstructuredFromByteSlice(event.Data())
	if err != nil {
		sink.Logger.Printf("cloudevent %s is malformed and will not be processed: %s\n", event.ID(), err)
		return
	}
	sink.Logger.Printf("cloudevent %s contains all required fields. Attempting to write it to the database\n", event.ID())
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	sink.Logger.Printf("writing resource from cloudevent %s into the database\n", event.ID())
	err = sink.Db.WriteResource(ctx, k8sObj, event.Data(), sink.ClusterName, sink.ClusterUid)
	defer cancel()
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
	db, err := database.NewDatabase()
	if err != nil {
		logger.Fatalf("Could not connect to the database: %s\n", err)
	}
	sink := NewSink(db, logger)
	err = sink.EventClient.StartReceiver(context.Background(), sink.Receive)
	if err != nil {
		logger.Fatalf("failed to start receiving CloudEvents: %s\n", err.Error())
	}
}
