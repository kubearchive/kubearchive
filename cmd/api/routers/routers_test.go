// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package routers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/kubearchive/kubearchive/pkg/database/fake"
)

var testResources = fake.CreateTestResources()
var testAPIResource = metav1.APIResource{
	Kind:         "Crontab",
	Name:         "crontabs",
	Group:        "stable.example.com",
	Version:      "v1",
	SingularName: "crontab",
	Namespaced:   true}

func setupRouter() *gin.Engine {
	router := gin.Default()
	ctrl := Controller{Database: fake.NewFakeDatabase(testResources)}
	router.Use(func(c *gin.Context) {
		c.Set("apiResource", testAPIResource)
		c.Next()
	})
	router.GET("/apis/:group/:version/:resourceType", ctrl.GetAllResources)
	router.GET("/apis/:group/:version/namespace/:namespace/:resourceType", ctrl.GetAllResources)
	return router
}

func TestGetAllResources(t *testing.T) {
	router := setupRouter()

	res := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/apis/stable.example.com/v1/crontabs", nil)
	router.ServeHTTP(res, req)

	assert.Equal(t, http.StatusOK, res.Code)
	var resources []*unstructured.Unstructured
	if err := json.NewDecoder(res.Body).Decode(&resources); err != nil {
		t.Fail()
	}
	assert.Equal(t, resources, testResources)
}

func TestGetNamespacedResources(t *testing.T) {
	router := setupRouter()

	res := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/apis/stable.example.com/v1/namespace/ns/crontabs", nil)
	router.ServeHTTP(res, req)

	assert.Equal(t, http.StatusOK, res.Code)
	var resources []*unstructured.Unstructured
	if err := json.NewDecoder(res.Body).Decode(&resources); err != nil {
		t.Fail()
	}
	assert.Equal(t, resources, testResources)
}
