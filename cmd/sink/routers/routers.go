// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package routers

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	ceOtelObs "github.com/cloudevents/sdk-go/observability/opentelemetry/v2/http"
	cloudevents "github.com/cloudevents/sdk-go/v2"
	ceClient "github.com/cloudevents/sdk-go/v2/client"
	"github.com/cloudevents/sdk-go/v2/protocol"
	"github.com/gin-gonic/gin"
	kubearchiveapi "github.com/kubearchive/kubearchive/cmd/operator/api/v1"
	"github.com/kubearchive/kubearchive/cmd/sink/filters"
	"github.com/kubearchive/kubearchive/cmd/sink/k8s"
	"github.com/kubearchive/kubearchive/cmd/sink/logs"
	"github.com/kubearchive/kubearchive/pkg/abort"
	"github.com/kubearchive/kubearchive/pkg/constants"
	"github.com/kubearchive/kubearchive/pkg/database/interfaces"
	"github.com/kubearchive/kubearchive/pkg/models"
	"github.com/kubearchive/kubearchive/pkg/observability"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	errs "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
)

type Controller struct {
	ceHandler     *ceClient.EventReceiver
	ceProtocol    *cloudevents.HTTPProtocol
	Db            interfaces.DBWriter
	Filters       filters.Interface
	K8sClient     dynamic.Interface
	LogUrlBuilder *logs.UrlBuilder
}

func NewController(
	db interfaces.DBWriter, filter filters.Interface, k8sClient dynamic.Interface, urlBuilder *logs.UrlBuilder,
) (*Controller, error) {
	controller := &Controller{
		Db: db, Filters: filter, K8sClient: k8sClient, LogUrlBuilder: urlBuilder,
	}
	ceProtocol, err := ceOtelObs.NewObservedHTTP()
	if err != nil {
		return nil, fmt.Errorf("could not create controller: %w", err)
	}
	controller.ceProtocol = ceProtocol
	// using context.Background() because the context passed to this function does not get used
	ceHandler, err := cloudevents.NewHTTPReceiveHandler(context.Background(), ceProtocol, controller.receiveCloudEvent)
	if err != nil {
		return nil, fmt.Errorf("could not create controller: %w", err)
	}
	controller.ceHandler = ceHandler
	return controller, nil
}

func (c *Controller) writeResource(ctx context.Context, obj *unstructured.Unstructured, event cloudevents.Event) (interfaces.WriteResourceResult, error) {
	lastUpdateTs := k8s.GetLastUpdateTs(obj)
	dbCtx, cancel := context.WithTimeout(ctx, time.Second*5)
	defer cancel()

	var urls []models.LogTuple
	var jsonPath string
	if c.LogUrlBuilder != nil {
		jsonPath = c.LogUrlBuilder.GetJsonPath()

		var err error
		urls, err = c.LogUrlBuilder.Urls(ctx, obj)
		if err != nil {
			slog.ErrorContext(
				ctx,
				"Could not build log urls for resource",
				"id", string(obj.GetUID()),
				"kind", obj.GetKind(),
				"namespace", obj.GetNamespace(),
				"name", obj.GetName(),
				"err", err,
			)
			return interfaces.WriteResourceResultError, err
		}
		if len(urls) == 0 {
			slog.InfoContext(
				ctx,
				"No log urls were generated for object",
				"id", string(obj.GetUID()),
				"kind", obj.GetKind(),
				"namespace", obj.GetNamespace(),
				"name", obj.GetName(),
			)
		}
	}

	result, writeResourceErr := c.Db.WriteResource(dbCtx, obj, event.Data(), lastUpdateTs, jsonPath, urls...)
	if writeResourceErr != nil {
		slog.ErrorContext(
			ctx,
			"Failed to write object from cloudevent to the database",
			"event-id", event.ID(),
			"event-type", event.Type(),
			"id", string(obj.GetUID()),
			"kind", obj.GetKind(),
			"namespace", obj.GetNamespace(),
			"name", obj.GetName(),
			"err", writeResourceErr,
		)
		return result, writeResourceErr
	}

	slog.InfoContext(
		ctx,
		"Successfully wrote object from cloudevent to the database",
		"event-id", event.ID(),
		"event-type", event.Type(),
		"id", string(obj.GetUID()),
		"kind", obj.GetKind(),
		"namespace", obj.GetNamespace(),
		"name", obj.GetName(),
	)

	return result, nil
}

// receiveCloudEvent returns an HTTP 422 if event.Data is not a kubernetes object. All other failures should return HTTP
// 500 instead.
func (c *Controller) receiveCloudEvent(ctx context.Context, event cloudevents.Event) protocol.Result {
	CEMetricAttrs := map[string]string{
		"event_type": event.Type(),
		// We default to error because error is the most duplicated result code path
		"result": string(observability.CEResultError),
	}

	// We schedule the function call with `defer`, meanwhile `CEMetricAttrs` can be modified
	defer func() {
		attrs := []attribute.KeyValue{}
		for k, v := range CEMetricAttrs {
			attrs = append(attrs, attribute.String(k, v))
		}
		observability.CloudEvents.Add(ctx, 1, metric.WithAttributes(attrs...))
	}()

	ex := event.Extensions()
	slog.InfoContext(ctx, "Received CloudEvent", "event-id", event.ID(), "event-type", event.Type(), "kind", ex["kind"], "name", ex["name"], "namespace", ex["namespace"])
	k8sObj, err := models.UnstructuredFromByteSlice(event.Data())
	if err != nil {
		slog.ErrorContext(ctx, "Received malformed CloudEvent", "event-id", event.ID(), "err", err)
		return NewCEResult(http.StatusUnprocessableEntity)
	}

	CEMetricAttrs["resource_type"] = fmt.Sprintf("%s/%s", k8sObj.GetAPIVersion(), k8sObj.GetKind())

	if !c.Filters.IsConfigured(ctx, k8sObj) {
		CEMetricAttrs["result"] = string(observability.CEResultNoConfiguration)
		slog.WarnContext(
			ctx,
			"Resource update received, resource is not configured",
			"event-id", event.ID(),
			"event-type", event.Type(),
			"id", string(k8sObj.GetUID()),
			"kind", k8sObj.GetKind(),
			"apiVersion", k8sObj.GetAPIVersion(),
			"namespace", k8sObj.GetNamespace(),
			"name", k8sObj.GetName())
		return NewCEResult(http.StatusAccepted)
	}

	// If message is of type delete we end early
	if strings.HasSuffix(event.Type(), ".delete") {
		logMsg := "Resource deletion received, resource archived"
		if c.Filters.MustArchiveOnDelete(ctx, k8sObj) {
			result, writeErr := c.writeResource(ctx, k8sObj, event)
			if writeErr != nil {
				return NewCEResult(http.StatusInternalServerError)
			}

			CEMetricAttrs["result"] = string(observability.NewCEResultFromWriteResourceResult(result))
		} else {
			CEMetricAttrs["result"] = string(observability.CEResultNoMatch)
			logMsg = "Resource deletion received, resource did not match for archive on deletion"
		}

		slog.InfoContext(
			ctx,
			logMsg,
			"event-id", event.ID(),
			"event-type", event.Type(),
			"id", string(k8sObj.GetUID()),
			"kind", k8sObj.GetKind(),
			"namespace", k8sObj.GetNamespace(),
			"name", k8sObj.GetName(),
		)
		return NewCEResult(http.StatusAccepted)
	}

	// If resource does not match archival, we exit early
	if !c.Filters.MustArchive(ctx, k8sObj) {
		slog.InfoContext(
			ctx,
			"Resource update received, no archive needed",
			"event-id", event.ID(),
			"event-type", event.Type(),
			"id", string(k8sObj.GetUID()),
			"kind", k8sObj.GetKind(),
			"namespace", k8sObj.GetNamespace(),
			"name", k8sObj.GetName(),
		)
		CEMetricAttrs["result"] = string(observability.CEResultNoMatch)
		return NewCEResult(http.StatusAccepted)
	}

	result, err := c.writeResource(ctx, k8sObj, event)
	if err != nil {
		return NewCEResult(http.StatusInternalServerError)
	}

	CEMetricAttrs["result"] = string(observability.NewCEResultFromWriteResourceResult(result))
	// If after archiving the resource does not need to be deleted we exit early
	if !c.Filters.MustDelete(ctx, k8sObj) {
		slog.InfoContext(
			ctx,
			"Resource was updated and archived",
			"event-id", event.ID(),
			"event-type", event.Type(),
			"id", string(k8sObj.GetUID()),
			"kind", k8sObj.GetKind(),
			"namespace", k8sObj.GetNamespace(),
			"name", k8sObj.GetName(),
		)
		return NewCEResult(http.StatusAccepted)
	}

	// We first schedule the deletion from the cluster
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
	if errs.IsNotFound(err) {
		slog.InfoContext(
			ctx,
			"Resource is already deleted",
			"event-id", event.ID(),
			"event-type", event.Type(),
			"id", string(k8sObj.GetUID()),
			"kind", k8sObj.GetKind(),
			"namespace", k8sObj.GetNamespace(),
			"name", k8sObj.GetName(),
		)
		return NewCEResult(http.StatusAccepted)
	}
	if err != nil {
		slog.ErrorContext(
			ctx,
			"Error deleting a resource",
			"event-id", event.ID(),
			"event-type", event.Type(),
			"id", string(k8sObj.GetUID()),
			"kind", k8sObj.GetKind(),
			"namespace", k8sObj.GetNamespace(),
			"name", k8sObj.GetName(),
			"err", err,
		)
		return NewCEResult(http.StatusInternalServerError)
	}

	// After deleting the resource we persist it with deletionTimestamp
	deleteTs := metav1.Now()
	k8sObj.SetDeletionTimestamp(&deleteTs)
	result, err = c.writeResource(ctx, k8sObj, event)
	if err != nil {
		return NewCEResult(http.StatusInternalServerError)
	}

	CEMetricAttrs["result"] = string(observability.NewCEResultFromWriteResourceResult(result))
	slog.InfoContext(
		ctx,
		"Resource was updated, archived and deleted from the cluster",
		"event-id", event.ID(),
		"event-type", event.Type(),
		"id", string(k8sObj.GetUID()),
		"kind", k8sObj.GetKind(),
		"namespace", k8sObj.GetNamespace(),
		"name", k8sObj.GetName(),
	)

	return NewCEResult(http.StatusAccepted)
}

func (c *Controller) CloudEventsHandler(ctx *gin.Context) {
	c.ceHandler.ServeHTTP(ctx.Writer, ctx.Request)
}

func (c *Controller) Livez(ctx *gin.Context) {
	observabilityConfig := observability.Status()
	ctx.JSON(http.StatusOK, gin.H{
		"Code":          http.StatusOK,
		"ginMode":       gin.Mode(),
		"openTelemetry": observabilityConfig,
		"message":       "healthy",
	})
}

// Readyz checks connections to the Database and to the Kubernetes API
func (c *Controller) Readyz(ctx *gin.Context) {
	err := c.Db.Ping(ctx.Request.Context())
	if err != nil {
		abort.Abort(ctx, err, http.StatusServiceUnavailable)
		return
	}

	_, err = c.K8sClient.Resource(kubearchiveapi.SinkFilterGVR).Namespace(constants.KubeArchiveNamespace).Get(ctx.Request.Context(), constants.SinkFilterResourceName, metav1.GetOptions{})
	if err != nil && !errs.IsNotFound(err) {
		abort.Abort(ctx, err, http.StatusServiceUnavailable)
	}
	ctx.JSON(http.StatusOK, gin.H{"message": "ready"})
}
