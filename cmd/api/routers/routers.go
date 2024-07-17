// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package routers

import (
	"fmt"

	"github.com/kubearchive/kubearchive/cmd/api/abort"

	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/kubearchive/kubearchive/pkg/database"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Controller struct {
	Database database.DBInterface
}

// GetAllResources responds with the list of resources of a specific type across all namespaces
func (c *Controller) GetAllResources(context *gin.Context) {
	group := context.Param("group")
	version := context.Param("version")
	kind := getAPIResourceKind(context)
	resources, err := c.Database.QueryResources(context, kind, group, version)

	if err != nil {
		context.JSON(http.StatusInternalServerError, err.Error())
	}

	context.JSON(http.StatusOK, resources)
}

func (c *Controller) GetNamespacedResources(context *gin.Context) {
	group := context.Param("group")
	version := context.Param("version")
	namespace := context.Param("namespace")
	kind := getAPIResourceKind(context)
	resources, err := c.Database.QueryNamespacedResources(context, kind, group, version, namespace)

	if err != nil {
		context.JSON(http.StatusInternalServerError, err.Error())
	}

	context.JSON(http.StatusOK, resources)
}

func getAPIResourceKind(context *gin.Context) string {
	resource, ok := context.Get("apiResource")
	if !ok {
		abort.Abort(context, "API resource not found", http.StatusInternalServerError)
	}
	apiResource, ok := resource.(metav1.APIResource)
	if !ok {
		abort.Abort(context, fmt.Sprintf("unexpected API resource type in context: %T", resource),
			http.StatusInternalServerError)
	}
	return apiResource.Kind
}
