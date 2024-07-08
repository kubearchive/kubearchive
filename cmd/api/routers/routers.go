// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package routers

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/kubearchive/kubearchive/pkg/database"
)

type Controller struct {
	Database database.DBInterface
}

// GetAllResources responds with the list of resources of a specific type across all namespaces
func (c *Controller) GetAllResources(context *gin.Context) {
	group := context.Param("group")
	version := context.Param("version")
	resource := getResourceKindFromPlural(context.Param("resourceType"))
	resources, err := c.Database.QueryResources(context, resource, group, version)

	if err != nil {
		context.JSON(http.StatusInternalServerError, err.Error())
	}

	context.JSON(http.StatusOK, resources)
}

func (c *Controller) GetNamespacedResources(context *gin.Context) {
	group := context.Param("group")
	version := context.Param("version")
	namespace := context.Param("namespace")
	resource := getResourceKindFromPlural(context.Param("resourceType")) + "/" + context.Param("namespace")
	resources, err := c.Database.QueryNamespacedResources(context, resource, group, version, namespace)

	if err != nil {
		context.JSON(http.StatusInternalServerError, err.Error())
	}

	context.JSON(http.StatusOK, resources)
}

func getResourceKindFromPlural(plural string) string {
	// FIXME - Title is deprecated and this conversion must be done in a different way
	capitalized := strings.Title(plural) //nolint:staticcheck
	return capitalized[:len(plural)-1]
}
