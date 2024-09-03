// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package routers

import (
	"errors"
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

	kind, err := getAPIResourceKind(context)
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

	kind, err := getAPIResourceKind(context)
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

	kind, err := getAPIResourceKind(context)
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

	kind, err := getAPIResourceKind(context)
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

func getAPIResourceKind(context *gin.Context) (string, error) {
	resource, ok := context.Get("apiResource")
	if !ok {
		return "", errors.New("API resource not found")
	}
	apiResource, ok := resource.(metav1.APIResource)
	if !ok {
		return "", fmt.Errorf("unexpected API resource type, in context: %T", resource)
	}
	return apiResource.Kind, nil
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
