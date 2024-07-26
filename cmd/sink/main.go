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
	jsonpath "github.com/kubearchive/kubearchive/cmd/sink/jsonPath"
	"github.com/kubearchive/kubearchive/pkg/database"
	"github.com/kubearchive/kubearchive/pkg/models"
	kaObservability "github.com/kubearchive/kubearchive/pkg/observability"
	_ "github.com/lib/pq"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
)

const (
	DeleteWhenEnvVar = "DELETE_WHEN"
)

type Sink struct {
	Db          database.DBInterface
	DeleteWhen  string
	EventClient ceClient.Client
	K8sClient   *dynamic.DynamicClient
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

	deleteWhen := os.Getenv(DeleteWhenEnvVar)
	deleteWhen, err = jsonpath.RelaxedJSONPathExpression(deleteWhen)
	if err != nil {
		logger.Fatalf("Provided JSON Path %s could not be parsed: %s\n", deleteWhen, err)
	}

	k8sClient, err := GetKubernetesClient()
	if err != nil {
		logger.Fatalln("Could not start a kubernetes client:", err)
	}

	return &Sink{
		Db:          db,
		DeleteWhen:  deleteWhen,
		EventClient: eventClient,
		K8sClient:   k8sClient,
		Logger:      logger,
	}
}

// Processes incoming cloudevents and writes them to the database
func (sink *Sink) Receive(ctx context.Context, event cloudevents.Event) {
	sink.Logger.Println("received CloudEvent: ", event.ID())
	k8sObj, err := models.UnstructuredFromByteSlice(event.Data())
	if err != nil {
		sink.Logger.Printf("cloudevent %s is malformed and will not be processed: %s\n", event.ID(), err)
		return
	}
	sink.Logger.Printf("cloudevent %s contains all required fields. Attempting to write it to the database\n", event.ID())
	ctx, cancel := context.WithTimeout(ctx, time.Second*5)
	sink.Logger.Printf("writing resource from cloudevent %s into the database\n", event.ID())
	err = sink.Db.WriteResource(ctx, k8sObj, event.Data())
	defer cancel()
	if err != nil {
		sink.Logger.Printf("failed to write cloudevent %s to the database: %s\n", event.ID(), err)
		return
	}
	sink.Logger.Printf("successfully wrote cloudevent %s to the database\n", event.ID())
	sink.Logger.Printf("checking if resource from cloudevent %s needs to be deleted\n", event.ID())
	mustDelete, err := jsonpath.PathExists(sink.DeleteWhen, k8sObj.UnstructuredContent())
	if err != nil {
		sink.Logger.Printf(
			"encountered error while evaluating JSON Path %s on cloudevent %s: %s\n",
			sink.DeleteWhen,
			event.ID(),
			err,
		)
	}
	if mustDelete {
		sink.Logger.Println("requesting to delete kubernetes object:", k8sObj.GetUID())
		kind := k8sObj.GetObjectKind().GroupVersionKind()
		resource, _ := meta.UnsafeGuessKindToResource(kind) // we only need the plural resource
		k8sCtx, k8sCancel := context.WithTimeout(context.Background(), time.Second*5)
		defer k8sCancel()
		err = sink.K8sClient.Resource(resource).Namespace(k8sObj.GetNamespace()).Delete(k8sCtx, k8sObj.GetName(), metav1.DeleteOptions{})
		if err != nil {
			sink.Logger.Printf("failed to request resource %s be deleted: %s\n", k8sObj.GetUID(), err)
			return
		}
		sink.Logger.Printf("successfully requested kubernetes object %s be deleted\n", k8sObj.GetUID())
		deleteTs := metav1.Now()
		k8sObj.SetDeletionTimestamp(&deleteTs)
		sink.Logger.Println("updating cluster_deleted_ts for kubernetes object:", k8sObj.GetUID())
		updateCtx, updateCancel := context.WithTimeout(context.Background(), time.Second*5)
		err = sink.Db.WriteResource(updateCtx, k8sObj, event.Data())
		defer updateCancel()
		if err != nil {
			sink.Logger.Println("failed to update cluster_deleted_ts for kubernetes object:", k8sObj.GetUID())
			return
		}
		sink.Logger.Println("updated cluster_deleted_ts for kubernetes object:", k8sObj.GetUID())
	}
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
