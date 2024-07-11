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
	// FIXME - Title is deprecated and this conversion must be done in a different way
	resource := strings.Title(context.Param("resourceType")) //nolint:staticcheck
	resources, err := c.Database.QueryResources(context, resource[:len(resource)-1], group, version)

	if err != nil {
		context.JSON(http.StatusInternalServerError, err.Error())
	}

	context.JSON(http.StatusOK, resources)
}
