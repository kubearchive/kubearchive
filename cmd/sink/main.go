// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	ceOtelObs "github.com/cloudevents/sdk-go/observability/opentelemetry/v2/client"
	cloudevents "github.com/cloudevents/sdk-go/v2"
	ceClient "github.com/cloudevents/sdk-go/v2/client"
	"github.com/kubearchive/kubearchive/cmd/sink/filters"
	"github.com/kubearchive/kubearchive/cmd/sink/k8s"
	"github.com/kubearchive/kubearchive/cmd/sink/logs"
	"github.com/kubearchive/kubearchive/pkg/database"
	"github.com/kubearchive/kubearchive/pkg/models"
	kaObservability "github.com/kubearchive/kubearchive/pkg/observability"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
)

var (
	version = "main"
	commit  = ""
	date    = ""
)

const (
	otelServiceName = "kubearchive.sink"
	mountPathEnvVar = "MOUNT_PATH"
)

type Sink struct {
	Db            database.DBInterface
	EventClient   ceClient.Client
	Filters       *filters.Filters
	K8sClient     *dynamic.DynamicClient
	logUrlBuilder *logs.UrlBuilder
}

func NewSink(db database.DBInterface, filters *filters.Filters, builder *logs.UrlBuilder) *Sink {
	if db == nil {
		slog.Error("Cannot start sink when db connection is nil")
		os.Exit(1)
	}

	httpClient, err := cloudevents.NewHTTP(
		cloudevents.WithRoundTripper(otelhttp.NewTransport(http.DefaultTransport)),
		cloudevents.WithMiddleware(func(next http.Handler) http.Handler {
			return otelhttp.NewHandler(next, "receive")
		}),
	)
	if err != nil {
		slog.Error("Failed to create HTTP client", "err", err.Error())
		os.Exit(1)
	}
	eventClient, err := cloudevents.NewClient(httpClient, ceClient.WithObservabilityService(ceOtelObs.NewOTelObservabilityService()))
	if err != nil {
		slog.Error("Failed to create CloudEvents HTTP client", "err", err.Error())
		os.Exit(1)
	}

	k8sClient, err := k8s.GetKubernetesClient()
	if err != nil {
		slog.Error("Could not start a kubernetes client", "err", err)
		os.Exit(1)
	}

	return &Sink{
		Db:            db,
		EventClient:   eventClient,
		Filters:       filters,
		K8sClient:     k8sClient,
		logUrlBuilder: builder,
	}
}

func (sink *Sink) writeLogs(ctx context.Context, obj *unstructured.Unstructured) {
	logCtx, cancel := context.WithTimeout(ctx, time.Second*5)
	defer cancel()
	urls, err := sink.logUrlBuilder.Urls(logCtx, obj)
	if err != nil {
		slog.Error("Could not build log urls for object", "objectID", obj.GetUID(), "err", err)
		return
	}
	err = sink.Db.WriteUrls(logCtx, obj, sink.logUrlBuilder.GetJsonPath(), urls...)
	if err != nil {
		slog.Error("Failed to write log urls for object to the database", "objectID", obj.GetUID(), "err", err)
	} else {
		slog.Info(
			"Successfully wrote log urls for object to the database",
			"objectID",
			obj.GetUID(),
			"kind",
			obj.GetKind(),
		)
	}
}

func (sink *Sink) writeResource(ctx context.Context, obj *unstructured.Unstructured, event cloudevents.Event) {
	dbCtx, cancel := context.WithTimeout(ctx, time.Second*5)
	defer cancel()
	err := sink.Db.WriteResource(dbCtx, obj, event.Data())
	if err != nil {
		slog.Error(
			"Failed to write object from cloudevent to the database",
			"objectID", obj.GetUID(),
			"id", event.ID(),
			"err", err,
		)
	} else {
		slog.Info(
			"Successfully wrote object from cloudevent to the database",
			"objectID", obj.GetUID(),
			"id", event.ID(),
		)
	}
	// only write logs for k8s resources likes pods that have them and UrlBuilder is configured
	if sink.logUrlBuilder != nil && strings.ToLower(obj.GetKind()) == "pod" {
		sink.writeLogs(ctx, obj)
	}
}

// Processes incoming cloudevents and writes them to the database
func (sink *Sink) Receive(ctx context.Context, event cloudevents.Event) {
	slog.Info("Received cloudevent", "id", event.ID())
	k8sObj, err := models.UnstructuredFromByteSlice(event.Data())
	if err != nil {
		slog.Error("Cloudevent is malformed and will not be processed", "id", event.ID(), "err", err)
		return
	}
	slog.Info(
		"Cloudevent contains all required fields. Checking if its object needs to be archived",
		"id", event.ID(),
		"objectID", k8sObj.GetUID(),
	)

	if strings.HasSuffix(event.Type(), ".delete") {
		slog.Info(
			"The type of cloudevent is Delete. Checking if object needs to be archived",
			"id", event.ID(),
			"objectID", k8sObj.GetUID(),
		)
		if sink.Filters.MustArchiveOnDelete(ctx, k8sObj) {
			sink.writeResource(ctx, k8sObj, event)
			return
		}
		slog.Info(
			"Object from cloudevent does not need to be archived after deletion",
			"objectID", k8sObj.GetUID(),
			"id", event.ID(),
		)
		return
	}
	if !sink.Filters.MustArchive(ctx, k8sObj) {
		slog.Info(
			"Object from cloudevent does not need to be archived",
			"objectID", k8sObj.GetUID(),
			"id", event.ID(),
		)
		return
	}
	slog.Info(
		"Writing object from cloudevent into the database",
		"objectID", k8sObj.GetUID(),
		"id", event.ID(),
	)
	sink.writeResource(ctx, k8sObj, event)
	slog.Info(
		"Checking if object from cloudevent needs to be deleted",
		"objectID", k8sObj.GetUID(),
		"id", event.ID(),
	)
	if sink.Filters.MustDelete(ctx, k8sObj) {
		slog.Info("Attempting to delete kubernetes object", "objectID", k8sObj.GetUID())
		kind := k8sObj.GetObjectKind().GroupVersionKind()
		resource, _ := meta.UnsafeGuessKindToResource(kind)     // we only need the plural resource
		propagationPolicy := metav1.DeletePropagationBackground // can't get address of a const
		k8sCtx, k8sCancel := context.WithTimeout(ctx, time.Second*5)
		defer k8sCancel()
		err = sink.K8sClient.Resource(resource).Namespace(k8sObj.GetNamespace()).Delete(
			k8sCtx,
			k8sObj.GetName(),
			metav1.DeleteOptions{PropagationPolicy: &propagationPolicy},
		)
		if err != nil {
			slog.Error(
				"Could not delete object",
				"objectID", k8sObj.GetUID(),
				"err", err,
			)
			return
		}
		slog.Info("Successfully requested kubernetes object be deleted", "objectID", k8sObj.GetUID())
		deleteTs := metav1.Now()
		k8sObj.SetDeletionTimestamp(&deleteTs)
		slog.Info("Updating cluster_deleted_ts for kubernetes object", "objectID", k8sObj.GetUID())
		sink.writeResource(ctx, k8sObj, event)
	} else {
		slog.Info(
			"Object from cloudevent does not need to be deleted",
			"objectID", k8sObj.GetUID(),
			"id", event.ID(),
		)
	}
}

func main() {
	slog.Info("Starting KubeArchive Sink", "version", version, "commit", commit, "built", date)

	err := kaObservability.Start(otelServiceName)
	if err != nil {
		slog.Error("Could not start tracing", "err", err.Error())
		os.Exit(1)
	}
	db, err := database.NewDatabase()
	if err != nil {
		slog.Error("Could not connect to the database", "err", err)
		os.Exit(1)
	}
	defer func(db database.DBInterface) {
		err = db.CloseDB()
		if err != nil {
			slog.Error("Could not close the database connection", "error", err.Error())
		} else {
			slog.Info("Connection closed successfully")
		}
	}(db)

	clientset, err := k8s.GetKubernetesClientset()
	if err != nil {
		slog.Error("Could not get a kubernetes client", "error", err)
		os.Exit(1)
	}

	filters := filters.NewFilters(clientset)
	stopUpdating, err := filters.Update()
	if err != nil {
		slog.Error("Could not listen for updates to filters:", "error", err)
		os.Exit(1)
	}
	defer stopUpdating()
	builder, err := logs.NewUrlBuilder()
	if err != nil {
		slog.Error("Could not enable log url creation", "error", err)
	}
	sink := NewSink(db, filters, builder)
	err = sink.EventClient.StartReceiver(context.Background(), sink.Receive)
	if err != nil {
		slog.Error("Failed to start receiving CloudEvents", "err", err.Error())
		os.Exit(1)
	}
}
