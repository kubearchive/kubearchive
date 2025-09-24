// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package routers

import (
	"errors"
	"fmt"
	"log/slog"
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
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
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

	// Parse label selector
	selector, parserErr := labels.Parse(context.Query("labelSelector"))
	if parserErr != nil {
		abort.Abort(context, parserErr, http.StatusBadRequest)
		return
	}
	reqs, _ := selector.Requirements()
	labelFilters, labelFiltersErr := labelFilter.NewLabelFilters(reqs)
	if labelFiltersErr != nil {
		abort.Abort(context, labelFiltersErr, http.StatusBadRequest)
		return
	}

	// Parse field selector
	fieldSelectorString := context.Query("fieldSelector")
	slog.Debug("Router: Raw field selector string", slog.String("fieldSelectorString", fieldSelectorString))

	fieldSelector, fieldParserErr := fields.ParseSelector(fieldSelectorString)
	if fieldParserErr != nil {
		slog.Error("Router: Field selector parsing failed",
			slog.String("fieldSelectorString", fieldSelectorString),
			slog.Any("error", fieldParserErr))
		abort.Abort(context, fieldParserErr, http.StatusBadRequest)
		return
	}
	fieldReqs := fieldSelector.Requirements()

	var filteredFieldReqs []fields.Requirement
	for _, req := range fieldReqs {
		if req.Field == "metadata.name" && req.Operator == selection.Equals {
			if name != "" {
				slog.Error("Router: Error - name already provided in URL path")
				abort.Abort(context,
					fmt.Errorf("field selector metadata.name not allowed when providing the name"),
					http.StatusBadRequest)
			}
			name = req.Value
		} else {
			filteredFieldReqs = append(filteredFieldReqs, req)
		}
	}

	if strings.HasPrefix(context.Request.URL.Path, "/apis/") && group == "" {
		abort.Abort(context, errors.New(http.StatusText(http.StatusNotFound)), http.StatusNotFound)
		return
	}

	apiVersion := version
	if group != "" {
		apiVersion = fmt.Sprintf("%s/%s", group, version)
	}

	slog.Debug("Router: Calling Database.QueryResources",
		slog.String("kind", kind),
		slog.String("apiVersion", apiVersion),
		slog.String("namespace", namespace),
		slog.String("name", name),
		slog.Int("limit", limit),
		slog.Int("fieldReqs", len(filteredFieldReqs)))

	// We send namespace even if it's an empty string (non-namespaced resources) the Database
	// knows what to do
	resources, lastId, lastDate, err := c.Database.QueryResources(
		context.Request.Context(), kind, apiVersion, namespace, name, id, date, labelFilters, filteredFieldReqs, limit)

	if err != nil {
		slog.Error("Router: Database.QueryResources failed", slog.Any("error", err))
		abort.Abort(context, err, http.StatusInternalServerError)
		return
	}

	slog.Debug("Router: Database.QueryResources succeeded", slog.Int("count", len(resources)))

	if name != "" {
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
