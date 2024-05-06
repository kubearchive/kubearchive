// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package routers

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"net/http"
)

// FIXME This will be taken from a shared pkg with sink based on the DB schema
// Just for the first approach
type resources struct {
	Kind       string   `json:"kind"`
	APIVersion string   `json:"apiVersion"`
	Items      []string `json:"items"`
}

// GetAllResources responds with the list of resources of a specific type across all namespaces
func GetAllResources(c *gin.Context) {
	group := c.Param("group")
	version := c.Param("version")
	resource := c.Param("resourceType")
	response := resources{
		Kind:       resource,
		APIVersion: fmt.Sprintf("%s/%s", group, version),
		Items:      []string{"resource1", "resource2"},
	}
	c.JSON(http.StatusOK, response)
}
