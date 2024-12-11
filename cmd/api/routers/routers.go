// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package routers

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kubearchive/kubearchive/cmd/api/abort"
	"github.com/kubearchive/kubearchive/cmd/api/discovery"
	"github.com/kubearchive/kubearchive/cmd/api/pagination"
	"github.com/kubearchive/kubearchive/pkg/database"
	"github.com/kubearchive/kubearchive/pkg/observability"
)

type CacheExpirations struct {
	Authorized   time.Duration
	Unauthorized time.Duration
}

type Controller struct {
	Database           database.DBInterface
	CacheConfiguration CacheExpirations
}

const listString = `{"kind": "List", "apiVersion": "v1", "metadata": {"continue": "%s"}, "items": [%s]}`

func (c *Controller) GetResources(context *gin.Context) {
	limit, id, date := pagination.GetValuesFromContext(context)
	kind, err := discovery.GetAPIResourceKind(context)
	if err != nil {
		abort.Abort(context, err, http.StatusInternalServerError)
		return
	}

	group := context.Param("group")
	version := context.Param("version")
	namespace := context.Param("namespace")
	name := context.Param("name")

	apiVersion := version
	if group != "" {
		apiVersion = fmt.Sprintf("%s/%s", group, version)
	}

	// We send namespace even if it's an empty string (non-namespaced resources) the Database
	// knows what to do
	resources, lastId, lastDate, err := c.Database.QueryResources(
		context.Request.Context(), kind, apiVersion, namespace, name, id, date, limit)

	if err != nil {
		abort.Abort(context, err, http.StatusInternalServerError)
		return
	}

	if name != "" {
		if len(resources) > 1 {
			abort.Abort(context, errors.New("more than one resource found"), http.StatusInternalServerError)
			return
		}
		context.String(http.StatusOK, resources[0])
		return
	}

	continueToken := pagination.CreateToken(lastId, lastDate)
	context.String(http.StatusOK, listString, continueToken, strings.Join(resources, ","))
}

func (c *Controller) GetResourceLogs(context *gin.Context) {
	kind, err := discovery.GetAPIResourceKind(context)
	if err != nil {
		abort.Abort(context, err, http.StatusInternalServerError)
		return
	}

	group := context.Param("group")
	version := context.Param("version")
	namespace := context.Param("namespace")
	name := context.Param("name")

	apiVersion := version
	if group != "" {
		apiVersion = fmt.Sprintf("%s/%s", group, version)
	}

	logURLs, err := c.Database.QueryLogURLs(
		context.Request.Context(), kind, apiVersion, namespace, name)
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
