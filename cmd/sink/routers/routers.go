// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package routers

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	ceOtelObs "github.com/cloudevents/sdk-go/observability/opentelemetry/v2/http"
	cloudevents "github.com/cloudevents/sdk-go/v2"
	ceClient "github.com/cloudevents/sdk-go/v2/client"
	"github.com/gin-gonic/gin"
	"github.com/kubearchive/kubearchive/cmd/sink/filters"
	"github.com/kubearchive/kubearchive/cmd/sink/logs"
	"github.com/kubearchive/kubearchive/pkg/database"
	"github.com/kubearchive/kubearchive/pkg/models"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
)

type Controller struct {
	ceHandler     *ceClient.EventReceiver
	ceProtocol    *cloudevents.HTTPProtocol
	Db            database.DBInterface
	Filters       filters.Interface
	K8sClient     dynamic.Interface
	LogUrlBuilder *logs.UrlBuilder
}

func NewController(
	db database.DBInterface, filter filters.Interface, k8sClient dynamic.Interface, urlBuilder *logs.UrlBuilder,
) (*Controller, error) {
	controller := &Controller{
		Db: db, Filters: filter, K8sClient: k8sClient, LogUrlBuilder: urlBuilder,
	}
	ceProtocol, err := ceOtelObs.NewObservedHTTP()
	if err != nil {
		return nil, fmt.Errorf("Could not create controller: %w", err)
	}
	controller.ceProtocol = ceProtocol
	// using context.Background() because the context passed to this function does not get used
	ceHandler, err := cloudevents.NewHTTPReceiveHandler(context.Background(), ceProtocol, controller.receiveCloudEvent)
	if err != nil {
		return nil, fmt.Errorf("Could not create controller: %w", err)
	}
	controller.ceHandler = ceHandler
	return controller, nil
}

func (c *Controller) writeLogs(ctx context.Context, obj *unstructured.Unstructured) {
	logCtx, cancel := context.WithTimeout(ctx, time.Second*5)
	defer cancel()
	urls, err := c.LogUrlBuilder.Urls(logCtx, obj)
	if err != nil {
		slog.Error(
			"Could not build log urls for resource",
			"id", obj.GetUID(),
			"kind", obj.GetKind(),
			"namespace", obj.GetNamespace(),
			"name", obj.GetName(),
			"err", err,
		)
		return
	}
	if len(urls) == 0 {
		slog.Info(
			"No log urls were generated for object",
			"id", obj.GetUID(),
			"kind", obj.GetKind(),
			"namespace", obj.GetNamespace(),
			"name", obj.GetName(),
		)
		return
	}
	err = c.Db.WriteUrls(logCtx, obj, c.LogUrlBuilder.GetJsonPath(), urls...)
	if err != nil {
		slog.Error(
			"Failed to write log urls for object to the database",
			"id", obj.GetUID(),
			"kind", obj.GetKind(),
			"namespace", obj.GetNamespace(),
			"name", obj.GetName(),
			"err", err,
		)
	} else {
		slog.Info(
			"Successfully wrote log urls for object to the database",
			"id", obj.GetUID(),
			"kind", obj.GetKind(),
			"namespace", obj.GetNamespace(),
			"name", obj.GetName(),
		)
	}
}

func (c *Controller) writeResource(ctx context.Context, obj *unstructured.Unstructured, event cloudevents.Event) {
	dbCtx, cancel := context.WithTimeout(ctx, time.Second*5)
	defer cancel()
	err := c.Db.WriteResource(dbCtx, obj, event.Data())
	if err != nil {
		slog.Error(
			"Failed to write object from cloudevent to the database",
			"event-id", event.ID(),
			"event-type", event.Type(),
			"id", obj.GetUID(),
			"kind", obj.GetKind(),
			"namespace", obj.GetNamespace(),
			"name", obj.GetName(),
			"err", err,
		)
	} else {
		slog.Info(
			"Successfully wrote object from cloudevent to the database",
			"event-id", event.ID(),
			"event-type", event.Type(),
			"id", obj.GetUID(),
			"kind", obj.GetKind(),
			"namespace", obj.GetNamespace(),
			"name", obj.GetName(),
			"id", event.ID(),
		)
	}
	// only write logs for k8s resources likes pods that have them and UrlBuilder is configured
	if c.LogUrlBuilder != nil && strings.ToLower(obj.GetKind()) == "pod" {
		c.writeLogs(ctx, obj)
	}
}

func (c *Controller) receiveCloudEvent(ctx context.Context, event cloudevents.Event) {
	k8sObj, err := models.UnstructuredFromByteSlice(event.Data())
	if err != nil {
		slog.Error("Received malformed CloudEvent", "event-id", event.ID(), "err", err)
		return
	}

	if strings.HasSuffix(event.Type(), ".delete") {
		if c.Filters.MustArchiveOnDelete(ctx, k8sObj) {
			c.writeResource(ctx, k8sObj, event)
			return
		}

		slog.Info(
			"Resource was deleted, no action is needed",
			"event-id", event.ID(),
			"event-type", event.Type(),
			"id", k8sObj.GetUID(),
			"kind", k8sObj.GetKind(),
			"namespace", k8sObj.GetNamespace(),
			"name", k8sObj.GetName(),
		)
		return
	}

	if !c.Filters.MustArchive(ctx, k8sObj) {
		slog.Info(
			"Resource was updated, no action is needed",
			"event-id", event.ID(),
			"event-type", event.Type(),
			"id", k8sObj.GetUID(),
			"kind", k8sObj.GetKind(),
			"namespace", k8sObj.GetNamespace(),
			"name", k8sObj.GetName(),
		)
		return
	}

	c.writeResource(ctx, k8sObj, event)

	if !c.Filters.MustDelete(ctx, k8sObj) {
		slog.Info(
			"Resource was updated and archived",
			"event-id", event.ID(),
			"event-type", event.Type(),
			"id", k8sObj.GetUID(),
			"kind", k8sObj.GetKind(),
			"namespace", k8sObj.GetNamespace(),
			"name", k8sObj.GetName(),
		)
		return
	}

	kind := k8sObj.GetObjectKind().GroupVersionKind()
	resource, _ := meta.UnsafeGuessKindToResource(kind)     // we only need the plural resource
	propagationPolicy := metav1.DeletePropagationBackground // can't get address of a const
	k8sCtx, k8sCancel := context.WithTimeout(ctx, time.Second*5)
	defer k8sCancel()

	err = c.K8sClient.Resource(resource).Namespace(k8sObj.GetNamespace()).Delete(
		k8sCtx,
		k8sObj.GetName(),
		metav1.DeleteOptions{PropagationPolicy: &propagationPolicy},
	)
	if err != nil {
		slog.Error(
			"Error deleting a resource",
			"event-id", event.ID(),
			"event-type", event.Type(),
			"id", k8sObj.GetUID(),
			"kind", k8sObj.GetKind(),
			"namespace", k8sObj.GetNamespace(),
			"name", k8sObj.GetName(),
			"err", err,
		)
		return
	}

	deleteTs := metav1.Now()
	k8sObj.SetDeletionTimestamp(&deleteTs)
	c.writeResource(ctx, k8sObj, event)

	slog.Info(
		"Resource was updated, archived and deleted from the cluster",
		"event-id", event.ID(),
		"event-type", event.Type(),
		"id", k8sObj.GetUID(),
		"kind", k8sObj.GetKind(),
		"namespace", k8sObj.GetNamespace(),
		"name", k8sObj.GetName(),
	)
}

func (c *Controller) CloudEventsHandler(ctx *gin.Context) {
	c.ceHandler.ServeHTTP(ctx.Writer, ctx.Request)
}
