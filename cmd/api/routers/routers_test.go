// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package routers

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/kubearchive/kubearchive/pkg/database"
	"github.com/kubearchive/kubearchive/pkg/database/fake"
)

var testResources = fake.CreateTestResources()
var nonCoreResources = testResources[:1]
var coreResources = testResources[1:2]
var testCoreAPIResource = metav1.APIResource{
	Kind:         "Pod",
	Name:         "pods",
	Version:      "v1",
	SingularName: "pod",
	Namespaced:   true,
}
var testNonCoreAPIResource = metav1.APIResource{
	Kind:         "Crontab",
	Name:         "crontabs",
	Group:        "stable.example.com",
	Version:      "v1",
	SingularName: "crontab",
	Namespaced:   true}

func setupRouter(db database.DBInterface, core bool) *gin.Engine {
	router := gin.Default()
	ctrl := Controller{Database: db}
	router.Use(func(c *gin.Context) {
		if core {
			c.Set("apiResource", testCoreAPIResource)
		} else {
			c.Set("apiResource", testNonCoreAPIResource)
		}
	})
	router.GET("/apis/:group/:version/:resourceType", ctrl.GetAllResources)
	router.GET("/apis/:group/:version/namespace/:namespace/:resourceType", ctrl.GetAllResources)
	router.GET("/api/:version/:resourceType", ctrl.GetAllCoreResources)
	router.GET("/api/:version/namespace/:namespace/:resourceType", ctrl.GetNamespacedCoreResources)
	return router
}

func TestGetAllResources(t *testing.T) {
	router := setupRouter(fake.NewFakeDatabase(testResources), false)

	res := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/apis/stable.example.com/v1/crontabs", nil)
	router.ServeHTTP(res, req)

	assert.Equal(t, http.StatusOK, res.Code)
	var resources List
	if err := json.NewDecoder(res.Body).Decode(&resources); err != nil {
		t.Fail()
	}
	assert.Equal(t, NewList(nonCoreResources), resources)
}

func TestGetNamespacedResources(t *testing.T) {
	router := setupRouter(fake.NewFakeDatabase(testResources), false)

	res := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/apis/stable.example.com/v1/namespace/test/crontabs", nil)
	router.ServeHTTP(res, req)

	assert.Equal(t, http.StatusOK, res.Code)
	var resources List
	if err := json.NewDecoder(res.Body).Decode(&resources); err != nil {
		t.Fail()
	}
	assert.Equal(t, NewList(nonCoreResources), resources)
}

func TestGetResourcesEmpty(t *testing.T) {
	router := setupRouter(fake.NewFakeDatabase([]*unstructured.Unstructured{}), false)

	res := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/apis/stable.example.com/v1/namespace/test/crontabs", nil)
	router.ServeHTTP(res, req)

	assert.Equal(t, http.StatusOK, res.Code)
	var resources List
	if err := json.NewDecoder(res.Body).Decode(&resources); err != nil {
		t.Fail()
	}
	assert.Equal(t, NewList(nil), resources)
}

func TestGetAllCoreResources(t *testing.T) {
	router := setupRouter(fake.NewFakeDatabase(testResources), true)

	res := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/pods", nil)
	router.ServeHTTP(res, req)

	assert.Equal(t, http.StatusOK, res.Code)
	var resources List
	if err := json.NewDecoder(res.Body).Decode(&resources); err != nil {
		t.Fail()
	}
	assert.Equal(t, NewList(coreResources), resources)
}

func TestGetCoreNamespacedResources(t *testing.T) {
	router := setupRouter(fake.NewFakeDatabase(testResources), true)

	res := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/namespace/test/pods", nil)
	router.ServeHTTP(res, req)

	assert.Equal(t, http.StatusOK, res.Code)
	var resources List
	if err := json.NewDecoder(res.Body).Decode(&resources); err != nil {
		t.Fail()
	}
	assert.Equal(t, NewList(coreResources), resources)
}

func TestDBError(t *testing.T) {
	router := setupRouter(fake.NewFakeDatabaseWithError(errors.New("test error")), true)

	res := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/pods", nil)
	router.ServeHTTP(res, req)

	assert.Equal(t, http.StatusInternalServerError, res.Code)
	assert.Contains(t, res.Body.String(), "test error")
	assert.NotContains(t, res.Body.String(), "Kind")
}

func TestNoAPIResourceInContextError(t *testing.T) {

	// Setting a router without a middleware that sets the api resource
	router := gin.Default()
	ctrl := Controller{Database: fake.NewFakeDatabase(testResources)}
	router.GET("/api/:version/:resourceType", ctrl.GetAllCoreResources)

	res := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/pods", nil)
	router.ServeHTTP(res, req)

	assert.Equal(t, http.StatusInternalServerError, res.Code)
	assert.Contains(t, res.Body.String(), "API resource not found")
	assert.NotContains(t, res.Body.String(), "Kind")
}

func TestLivez(t *testing.T) {

	router := gin.Default()
	ctrl := Controller{Database: fake.NewFakeDatabase(testResources),
		CacheConfiguration: CacheExpirations{Authorized: 60, Unauthorized: 5}}
	router.GET("/livez", ctrl.Livez)
	res := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/livez", nil)
	router.ServeHTTP(res, req)

	expected, _ := json.Marshal(gin.H{
		"code":           http.StatusOK,
		"ginMode":        "debug",
		"authCacheTTL":   60,
		"unAuthCacheTTL": 5,
		"openTelemetry":  "disabled",
		"message":        "healthy",
	})

	assert.Equal(t, res.Body.Bytes(), expected)
}

func TestReadyz(t *testing.T) {

	testCases := []struct {
		name        string
		dbConnReady bool
		expected    int
	}{
		{
			name:        "Database Ready",
			dbConnReady: true,
			expected:    http.StatusOK,
		},
		{
			name:        "Database Not Ready",
			dbConnReady: false,
			expected:    http.StatusServiceUnavailable,
		},
	}
	for _, testCase := range testCases {
		router := gin.Default()
		var ctrl Controller
		if testCase.dbConnReady {
			ctrl = Controller{Database: fake.NewFakeDatabase(testResources)}
		} else {
			ctrl = Controller{Database: fake.NewFakeDatabaseWithError(errors.New("test error"))}
		}
		router.GET("/readyz", ctrl.Readyz)
		res := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/readyz", nil)
		router.ServeHTTP(res, req)

		assert.Equal(t, testCase.expected, res.Code)
	}
}
