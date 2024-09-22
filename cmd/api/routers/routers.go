// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package routers

import (
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kubearchive/kubearchive/cmd/api/abort"
	"github.com/kubearchive/kubearchive/cmd/api/discovery"
	"github.com/kubearchive/kubearchive/pkg/database"
	"github.com/kubearchive/kubearchive/pkg/observability"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type CacheExpirations struct {
	Authorized   time.Duration
	Unauthorized time.Duration
}

type Controller struct {
	Database           database.DatabaseInterface
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

	kind, err := discovery.GetAPIResourceKind(context)
	if err != nil {
		abort.Abort(context, err.Error(), http.StatusInternalServerError)
		return
	}

	resources, err := c.Database.QueryResources(context.Request.Context(), kind, group, version)
	if err != nil {
		abort.Abort(context, err.Error(), http.StatusInternalServerError)
		return
	}

	context.JSON(http.StatusOK, NewList(resources))
}

func (c *Controller) GetNamespacedResources(context *gin.Context) {
	group := context.Param("group")
	version := context.Param("version")
	namespace := context.Param("namespace")

	kind, err := discovery.GetAPIResourceKind(context)
	if err != nil {
		abort.Abort(context, err.Error(), http.StatusInternalServerError)
		return
	}

	resources, err := c.Database.QueryNamespacedResources(context.Request.Context(), kind, group, version, namespace)
	if err != nil {
		abort.Abort(context, err.Error(), http.StatusInternalServerError)
		return
	}

	context.JSON(http.StatusOK, NewList(resources))
}

func (c *Controller) GetAllCoreResources(context *gin.Context) {
	version := context.Param("version")

	kind, err := discovery.GetAPIResourceKind(context)
	if err != nil {
		abort.Abort(context, err.Error(), http.StatusInternalServerError)
		return
	}

	resources, err := c.Database.QueryCoreResources(context.Request.Context(), kind, version)
	if err != nil {
		abort.Abort(context, err.Error(), http.StatusInternalServerError)
		return
	}

	context.JSON(http.StatusOK, NewList(resources))
}

func (c *Controller) GetNamespacedCoreResources(context *gin.Context) {
	version := context.Param("version")
	namespace := context.Param("namespace")

	kind, err := discovery.GetAPIResourceKind(context)
	if err != nil {
		abort.Abort(context, err.Error(), http.StatusInternalServerError)
		return
	}

	resources, err := c.Database.QueryNamespacedCoreResources(context.Request.Context(), kind, version, namespace)
	if err != nil {
		abort.Abort(context, err.Error(), http.StatusInternalServerError)
		return
	}

	context.JSON(http.StatusOK, NewList(resources))
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
		abort.Abort(context, err.Error(), http.StatusServiceUnavailable)
		return
	}
	context.JSON(http.StatusOK, gin.H{"message": "ready"})
}

// NewList creates a new List struct with apiVersion "v1",
// kind "List" and resource Version "". These values
// can be deserialized into any metav1.<Kind>List safely
func NewList(resources []*unstructured.Unstructured) List {
	return List{
		ApiVersion: "v1",
		Kind:       "List",
		Items:      resources,
		Metadata:   map[string]string{"resourceVersion": ""},
	}
}
