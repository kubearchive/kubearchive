// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package routers

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kubearchive/kubearchive/cmd/api/discovery"
	"github.com/kubearchive/kubearchive/cmd/api/pagination"
	"github.com/kubearchive/kubearchive/pkg/abort"
	dbErrors "github.com/kubearchive/kubearchive/pkg/database/errors"
	"github.com/kubearchive/kubearchive/pkg/database/interfaces"
	labelFilter "github.com/kubearchive/kubearchive/pkg/models"
	"github.com/kubearchive/kubearchive/pkg/observability"
	"k8s.io/apimachinery/pkg/labels"
)

type CacheExpirations struct {
	Authorized   time.Duration
	Unauthorized time.Duration
}

type Controller struct {
	Database           interfaces.DBReader
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

	if name != "" && strings.Contains(name, "*") {
		abort.Abort(context, errors.New("wildcard characters (*) are not allowed in path parameters, use query parameter ?name= instead"), http.StatusBadRequest)
		return
	}

	queryName := context.Query("name")

	if name != "" && queryName != "" {
		abort.Abort(context, errors.New("cannot specify both path name parameter and query name parameter"), http.StatusBadRequest)
		return
	}

	if queryName != "" {
		name = queryName
	}
	selector, parserErr := labels.Parse(context.Query("labelSelector"))
	if parserErr != nil {
		abort.Abort(context, parserErr, http.StatusBadRequest)
		return
	}
	reqs, _ := selector.Requirements()
	labelFilters, labelFiltersErr := labelFilter.NewLabelFilters(reqs)
	if labelFiltersErr != nil {
		abort.Abort(context, labelFiltersErr, http.StatusBadRequest)
	}

	if strings.HasPrefix(context.Request.URL.Path, "/apis/") && group == "" {
		abort.Abort(context, errors.New(http.StatusText(http.StatusNotFound)), http.StatusNotFound)
		return
	}

	// Parse timestamp filters
	creationTimestampAfter, err := parseTimestampQuery(context, "creationTimestampAfter")
	if err != nil {
		abort.Abort(context, err, http.StatusBadRequest)
		return
	}

	creationTimestampBefore, err := parseTimestampQuery(context, "creationTimestampBefore")
	if err != nil {
		abort.Abort(context, err, http.StatusBadRequest)
		return
	}

	if creationTimestampAfter != nil && creationTimestampBefore != nil {
		if creationTimestampBefore.Before(*creationTimestampAfter) || creationTimestampBefore.Equal(*creationTimestampAfter) {
			abort.Abort(context, errors.New("creationTimestampBefore must be after creationTimestampAfter"), http.StatusBadRequest)
			return
		}
	}

	apiVersion := version
	if group != "" {
		apiVersion = fmt.Sprintf("%s/%s", group, version)
	}

	// We send namespace even if it's an empty string (non-namespaced resources) the Database
	// knows what to do
	resources, lastId, lastDate, err := c.Database.QueryResources(
		context.Request.Context(), kind, apiVersion, namespace, name, id, date, labelFilters,
		creationTimestampAfter, creationTimestampBefore, limit)

	if err != nil {
		abort.Abort(context, err, http.StatusInternalServerError)
		return
	}

	if name != "" && !strings.Contains(name, "*") {
		if len(resources) == 0 {
			abort.Abort(context, errors.New("resource not found"), http.StatusNotFound)
			return
		} else if len(resources) > 1 {
			abort.Abort(context, errors.New("more than one resource found"), http.StatusInternalServerError)
			return
		}
		context.String(http.StatusOK, resources[0])
		return
	}

	continueToken := pagination.CreateToken(lastId, lastDate)
	context.String(http.StatusOK, listString, continueToken, strings.Join(resources, ","))
}

// parseTimestampQuery parses a timestamp query parameter and returns a pointer to time.Time
// Returns nil if the parameter is not provided or empty
func parseTimestampQuery(context *gin.Context, paramName string) (*time.Time, error) {
	value := context.Query(paramName)
	if value == "" {
		return nil, nil //nolint:nilnil // This is intentional - empty parameter means no filter
	}

	// Try parsing as RFC3339 format (ISO 8601)
	timestamp, err := time.Parse(time.RFC3339, value)
	if err != nil {
		// Try parsing as RFC3339Nano format
		timestamp, err = time.Parse(time.RFC3339Nano, value)
		if err != nil {
			return nil, fmt.Errorf("invalid %s format: %s. Expected RFC3339 format (e.g., 2023-01-01T12:00:00Z)", paramName, value)
		}
	}

	return &timestamp, nil
}

func (c *Controller) GetLogURL(context *gin.Context) {
	kind, err := discovery.GetAPIResourceKind(context)
	if err != nil {
		abort.Abort(context, err, http.StatusInternalServerError)
		return
	}

	group := context.Param("group")
	version := context.Param("version")
	namespace := context.Param("namespace")
	name := context.Param("name")
	containerName := context.Query("container")

	if strings.HasPrefix(context.Request.URL.Path, "/apis/") && group == "" {
		abort.Abort(context, errors.New(http.StatusText(http.StatusNotFound)), http.StatusNotFound)
		return
	}

	apiVersion := version
	if group != "" {
		apiVersion = fmt.Sprintf("%s/%s", group, version)
	}

	logURL, jsonPath, err := c.Database.QueryLogURL(
		context.Request.Context(), kind, apiVersion, namespace, name, containerName)

	if errors.Is(err, dbErrors.ErrResourceNotFound) {
		abort.Abort(context, err, http.StatusNotFound)
		return
	}
	if err != nil {
		abort.Abort(context, err, http.StatusInternalServerError)
		return
	}

	context.Set("logURL", logURL)
	context.Set("jsonPath", jsonPath)
}

// Livez returns current server configuration as we don't have a clear deadlock indicator
func (c *Controller) Livez(context *gin.Context) {
	observabilityConfig := observability.Status()

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
