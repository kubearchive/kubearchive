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
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type Controller struct {
	Database database.DBInterface
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
	kind := getAPIResourceKind(context)
	resources, err := c.Database.QueryResources(context.Request.Context(), kind, group, version)

	if err != nil {
		context.JSON(http.StatusInternalServerError, err.Error())
	}

	context.JSON(http.StatusOK, NewList(resources))
}

func (c *Controller) GetNamespacedResources(context *gin.Context) {
	group := context.Param("group")
	version := context.Param("version")
	namespace := context.Param("namespace")
	kind := getAPIResourceKind(context)
	resources, err := c.Database.QueryNamespacedResources(context.Request.Context(), kind, group, version, namespace)

	if err != nil {
		context.JSON(http.StatusInternalServerError, err.Error())
	}

	context.JSON(http.StatusOK, NewList(resources))
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
