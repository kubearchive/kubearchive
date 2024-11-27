// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package routers

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kubearchive/kubearchive/cmd/api/abort"
	"github.com/kubearchive/kubearchive/cmd/api/discovery"
	"github.com/kubearchive/kubearchive/cmd/api/pagination"
	"github.com/kubearchive/kubearchive/pkg/database"
	"github.com/kubearchive/kubearchive/pkg/observability"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type CacheExpirations struct {
	Authorized   time.Duration
	Unauthorized time.Duration
}

type Controller struct {
	Database           database.DBInterface
	CacheConfiguration CacheExpirations
}

// We roll our own List because we don't want to wrap resources
// into runtime.RawExtension. We will need more fields for
// pagination, but they are simple. If they are not, we can
// start using corev1.List{}. See "kubectl get" code to see
// how Lists are built.
type List struct {
	ApiVersion string                       `json:"apiVersion"`
	Items      []*unstructured.Unstructured `json:"items"`
	Kind       string                       `json:"kind"`
	Metadata   map[string]string            `json:"metadata"`
}

// GetAllResources responds with the list of resources of a specific type across all namespaces
func (c *Controller) GetAllResources(context *gin.Context) {
	group := context.Param("group")
	version := context.Param("version")
	apiVersion := fmt.Sprintf("%s/%s", group, version)
	limit, id, date := pagination.GetValuesFromContext(context)

	kind, err := discovery.GetAPIResourceKind(context)
	if err != nil {
		abort.Abort(context, err, http.StatusInternalServerError)
		return
	}

	resources, lastId, lastDate, err := c.Database.QueryResources(context.Request.Context(), kind, apiVersion, limit, id, date)
	if err != nil {
		abort.Abort(context, err, http.StatusInternalServerError)
		return
	}

	continueToken := pagination.CreateToken(lastId, lastDate)
	context.JSON(http.StatusOK, NewList(resources, continueToken))
}

func (c *Controller) GetNamespacedResources(context *gin.Context) {
	group := context.Param("group")
	version := context.Param("version")
	apiVersion := fmt.Sprintf("%s/%s", group, version)
	namespace := context.Param("namespace")
	limit, id, date := pagination.GetValuesFromContext(context)

	kind, err := discovery.GetAPIResourceKind(context)
	if err != nil {
		abort.Abort(context, err, http.StatusInternalServerError)
		return
	}

	resources, lastId, lastDate, err := c.Database.QueryNamespacedResources(context.Request.Context(), kind, apiVersion, namespace, limit, id, date)
	if err != nil {
		abort.Abort(context, err, http.StatusInternalServerError)
		return
	}

	continueToken := pagination.CreateToken(lastId, lastDate)
	context.JSON(http.StatusOK, NewList(resources, continueToken))
}

func (c *Controller) GetNamespacedResourceByName(context *gin.Context) {
	group := context.Param("group")
	version := context.Param("version")
	apiVersion := fmt.Sprintf("%s/%s", group, version)
	namespace := context.Param("namespace")
	name := context.Param("name")

	kind, err := discovery.GetAPIResourceKind(context)
	if err != nil {
		abort.Abort(context, err, http.StatusInternalServerError)
		return
	}

	resource, err := c.Database.QueryNamespacedResourceByName(context.Request.Context(), kind, apiVersion, namespace, name)
	if err != nil {
		abort.Abort(context, err, http.StatusInternalServerError)
		return
	}

	context.JSON(http.StatusOK, resource)
}

func (c *Controller) GetLogURLsByResourceName(context *gin.Context) {
	group := context.Param("group")
	version := context.Param("version")
	apiVersion := fmt.Sprintf("%s/%s", group, version)
	namespace := context.Param("namespace")
	name := context.Param("name")

	kind, err := discovery.GetAPIResourceKind(context)
	if err != nil {
		abort.Abort(context, err, http.StatusInternalServerError)
		return
	}
	logURLs, err := c.Database.QueryLogURLs(context.Request.Context(), kind, apiVersion, namespace, name)
	if errors.Is(err, database.ResourceNotFoundError) {
		abort.Abort(context, err, http.StatusNotFound)
	}
	if err != nil {
		abort.Abort(context, err, http.StatusInternalServerError)
		return
	}

	context.JSON(http.StatusOK, logURLs)
}

func (c *Controller) GetAllCoreResources(context *gin.Context) {
	version := context.Param("version")
	limit, id, date := pagination.GetValuesFromContext(context)

	kind, err := discovery.GetAPIResourceKind(context)
	if err != nil {
		abort.Abort(context, err, http.StatusInternalServerError)
		return
	}

	resources, lastId, lastDate, err := c.Database.QueryResources(context.Request.Context(), kind, version, limit, id, date)
	if err != nil {
		abort.Abort(context, err, http.StatusInternalServerError)
		return
	}

	continueToken := pagination.CreateToken(lastId, lastDate)
	context.JSON(http.StatusOK, NewList(resources, continueToken))
}

func (c *Controller) GetNamespacedCoreResources(context *gin.Context) {
	version := context.Param("version")
	namespace := context.Param("namespace")
	limit, id, date := pagination.GetValuesFromContext(context)

	kind, err := discovery.GetAPIResourceKind(context)
	if err != nil {
		abort.Abort(context, err, http.StatusInternalServerError)
		return
	}

	resources, lastId, lastDate, err := c.Database.QueryNamespacedResources(context.Request.Context(), kind, version, namespace, limit, id, date)
	if err != nil {
		abort.Abort(context, err, http.StatusInternalServerError)
		return
	}

	continueToken := pagination.CreateToken(lastId, lastDate)
	context.JSON(http.StatusOK, NewList(resources, continueToken))
}

func (c *Controller) GetNamespacedCoreResourceByName(context *gin.Context) {
	version := context.Param("version")
	namespace := context.Param("namespace")
	name := context.Param("name")

	kind, err := discovery.GetAPIResourceKind(context)
	if err != nil {
		abort.Abort(context, err, http.StatusInternalServerError)
		return
	}

	resource, err := c.Database.QueryNamespacedResourceByName(context.Request.Context(), kind, version, namespace, name)
	if err != nil {
		abort.Abort(context, err, http.StatusInternalServerError)
		return
	}

	context.JSON(http.StatusOK, resource)
}

func (c *Controller) GetLogURLsByCoreResourceName(context *gin.Context) {
	version := context.Param("version")
	namespace := context.Param("namespace")
	name := context.Param("name")

	kind, err := discovery.GetAPIResourceKind(context)
	if err != nil {
		abort.Abort(context, err, http.StatusInternalServerError)
		return
	}

	logURLs, err := c.Database.QueryLogURLs(context.Request.Context(), kind, version, namespace, name)
	if errors.Is(err, database.ResourceNotFoundError) {
		abort.Abort(context, err, http.StatusNotFound)
	}
	if err != nil {
		abort.Abort(context, err, http.StatusInternalServerError)
		return
	}

	context.JSON(http.StatusOK, logURLs)

}

// Livez returns current server configuration as we don't have a clear deadlock indicator
func (c *Controller) Livez(context *gin.Context) {
	observabilityConfig := os.Getenv(observability.OtelStartEnvVar)
	if observabilityConfig == "" {
		observabilityConfig = "disabled"
	}

	context.JSON(http.StatusOK, gin.H{
		"code":           http.StatusOK,
		"ginMode":        gin.Mode(),
		"authCacheTTL":   c.CacheConfiguration.Authorized,
		"unAuthCacheTTL": c.CacheConfiguration.Unauthorized,
		"openTelemetry":  observabilityConfig,
		"message":        "healthy",
	})
}

// Readyz checks Database connection
func (c *Controller) Readyz(context *gin.Context) {
	err := c.Database.Ping(context.Request.Context())
	if err != nil {
		abort.Abort(context, err, http.StatusServiceUnavailable)
		return
	}
	context.JSON(http.StatusOK, gin.H{"message": "ready"})
}

// NewList creates a new List struct with apiVersion "v1",
// kind "List" and resource Version "". These values
// can be deserialized into any metav1.<Kind>List safely
func NewList(resources []*unstructured.Unstructured, continueValue string) List {
	metadata := map[string]string{"resourceVersion": "", "continue": continueValue}
	return List{
		ApiVersion: "v1",
		Kind:       "List",
		Items:      resources,
		Metadata:   metadata,
	}
}
