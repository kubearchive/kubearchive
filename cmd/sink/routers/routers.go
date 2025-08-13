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

	cloudevents "github.com/cloudevents/sdk-go/v2"
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
	Db            interfaces.DBWriter
	Filters       filters.Interface
	K8sClient     dynamic.Interface
	LogUrlBuilder *logs.UrlBuilder
}

func NewController(
	db interfaces.DBWriter, filter filters.Interface, k8sClient dynamic.Interface, urlBuilder *logs.UrlBuilder,
) *Controller {
	return &Controller{
		Db: db, Filters: filter, K8sClient: k8sClient, LogUrlBuilder: urlBuilder,
	}
}

func (c *Controller) writeResource(ctx context.Context, obj *unstructured.Unstructured, event *cloudevents.Event) (interfaces.WriteResourceResult, error) {
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

// receiveCloudEvent returns an HTTP 400 if the request body is not a CloudEvent or HTTP 422 if event.Data is not a
// kubernetes object. All other failures should return HTTP 500 instead.
func (c *Controller) ReceiveCloudEvent(ctx *gin.Context) {
	event, eventErr := cloudevents.NewEventFromHTTPRequest(ctx.Request)
	if eventErr != nil || event == nil {
		slog.Error("Could not parse a CloudEvent from http request", "err", eventErr)
		ctx.Status(http.StatusBadRequest)
		return
	}
	validationErr := event.Validate()
	if validationErr != nil {
		slog.Error("Received invalid CloudEvent from http request", "err", validationErr)
		ctx.Status(http.StatusBadRequest)
		return
	}

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
		ctx.Status(http.StatusUnprocessableEntity)
		return
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
		ctx.Status(http.StatusAccepted)
		return
	}

	// If message is of type delete we end early
	if strings.HasSuffix(event.Type(), ".delete") {
		logMsg := "Resource deletion received, resource archived"
		if c.Filters.MustArchiveOnDelete(ctx, k8sObj) {
			result, writeErr := c.writeResource(ctx, k8sObj, event)
			if writeErr != nil {
				ctx.Status(http.StatusInternalServerError)
				return
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
		ctx.Status(http.StatusAccepted)
		return
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
		ctx.Status(http.StatusAccepted)
		return
	}

	result, err := c.writeResource(ctx, k8sObj, event)
	if err != nil {
		ctx.Status(http.StatusInternalServerError)
		return
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
		ctx.Status(http.StatusAccepted)
		return
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
		ctx.Status(http.StatusAccepted)
		return
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
		ctx.Status(http.StatusInternalServerError)
		return
	}

	// After deleting the resource we persist it with deletionTimestamp
	deleteTs := metav1.Now()
	k8sObj.SetDeletionTimestamp(&deleteTs)
	result, err = c.writeResource(ctx, k8sObj, event)
	if err != nil {
		ctx.Status(http.StatusInternalServerError)
		return
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

	ctx.Status(http.StatusAccepted)
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
