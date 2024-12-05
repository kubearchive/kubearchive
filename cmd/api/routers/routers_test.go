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
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/kubearchive/kubearchive/pkg/database"
	"github.com/kubearchive/kubearchive/pkg/database/fake"
)

var testResources = fake.CreateTestResources()
var testLogUrls = fake.CreateTestLogUrls()
var nonCoreResources = testResources[:1]
var coreResources = testResources[1:2]

type List struct {
	Items []*unstructured.Unstructured
}

func setupRouter(db database.DBInterface, core bool) *gin.Engine {
	router := gin.Default()
	ctrl := Controller{Database: db}
	router.Use(func(c *gin.Context) {
		if core {
			c.Set("apiResourceKind", "Pod")
		} else {
			c.Set("apiResourceKind", "Crontab")
		}
	})
	router.GET("/apis/:group/:version/:resourceType", ctrl.GetAllResources)
	router.GET("/apis/:group/:version/namespace/:namespace/:resourceType", ctrl.GetAllResources)
	router.GET("/apis/:group/:version/namespace/:namespace/:resourceType/:name", ctrl.GetNamespacedResourceByName)
	router.GET("/apis/:group/:version/namespace/:namespace/:resourceType/:name/log", ctrl.GetLogURLsByResourceName)
	router.GET("/api/:version/:resourceType", ctrl.GetAllCoreResources)
	router.GET("/api/:version/namespace/:namespace/:resourceType", ctrl.GetNamespacedCoreResources)
	router.GET("/api/:version/namespace/:namespace/:resourceType/:name", ctrl.GetNamespacedCoreResourceByName)
	router.GET("/api/:version/namespace/:namespace/:resourceType/:name/log", ctrl.GetLogURLsByCoreResourceName)
	return router
}

func TestGetCoreResourcesLogURLs(t *testing.T) {
	router := setupRouter(fake.NewFakeDatabase(testResources, testLogUrls), true)

	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/namespace/ns/pods/my-pod/log", nil)
	router.ServeHTTP(res, req)

	assert.Equal(t, http.StatusOK, res.Code)
	var urls []string
	if err := json.NewDecoder(res.Body).Decode(&urls); err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, []string{testLogUrls[0].Url}, urls)
}

func TestGetResourcesLogURLs(t *testing.T) {
	router := setupRouter(fake.NewFakeDatabase(testResources, testLogUrls), false)

	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/apis/batch/v1/namespace/ns/cronjobs/my-cronjob/log", nil)
	router.ServeHTTP(res, req)

	assert.Equal(t, http.StatusOK, res.Code)
	var urls []string
	if err := json.NewDecoder(res.Body).Decode(&urls); err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, len(testLogUrls), len(urls))
}

func TestGetAllResources(t *testing.T) {
	router := setupRouter(fake.NewFakeDatabase(testResources, testLogUrls), false)

	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/apis/stable.example.com/v1/crontabs", nil)
	router.ServeHTTP(res, req)

	assert.Equal(t, http.StatusOK, res.Code)
	var resources List
	if err := json.NewDecoder(res.Body).Decode(&resources); err != nil {
		t.Fail()
	}
	assert.Equal(t, nonCoreResources, resources.Items)
}

func TestGetNamespacedResources(t *testing.T) {
	router := setupRouter(fake.NewFakeDatabase(testResources, testLogUrls), false)

	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/apis/stable.example.com/v1/namespace/test/crontabs", nil)
	router.ServeHTTP(res, req)

	assert.Equal(t, http.StatusOK, res.Code)
	var resources List
	if err := json.NewDecoder(res.Body).Decode(&resources); err != nil {
		t.Fail()
	}

	assert.Equal(t, nonCoreResources, resources.Items)
}

func TestGetNamespacedResourcesByName(t *testing.T) {
	router := setupRouter(fake.NewFakeDatabase(testResources, testLogUrls), false)

	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/apis/stable.example.com/v1/namespace/test/crontabs/test", nil)
	router.ServeHTTP(res, req)

	assert.Equal(t, http.StatusOK, res.Code)
	var resource *unstructured.Unstructured
	if err := json.NewDecoder(res.Body).Decode(&resource); err != nil {
		t.Fail()
	}
	assert.Equal(t, nonCoreResources[0], resource)
}

func TestGetResourcesEmpty(t *testing.T) {
	router := setupRouter(fake.NewFakeDatabase([]*unstructured.Unstructured{}, []fake.LogUrlRow{}), false)

	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/apis/stable.example.com/v1/namespace/test/crontabs", nil)
	router.ServeHTTP(res, req)

	assert.Equal(t, http.StatusOK, res.Code)
	var resources List
	if err := json.NewDecoder(res.Body).Decode(&resources); err != nil {
		t.Fail()
	}
	assert.Equal(t, 0, len(resources.Items))
}

func TestGetAllCoreResources(t *testing.T) {
	router := setupRouter(fake.NewFakeDatabase(testResources, testLogUrls), true)

	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/pods", nil)
	router.ServeHTTP(res, req)

	assert.Equal(t, http.StatusOK, res.Code)
	var resources List
	if err := json.NewDecoder(res.Body).Decode(&resources); err != nil {
		t.Fail()
	}
	assert.Equal(t, coreResources, resources.Items)
}

func TestGetCoreNamespacedResources(t *testing.T) {
	router := setupRouter(fake.NewFakeDatabase(testResources, testLogUrls), true)

	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/namespace/test/pods", nil)
	router.ServeHTTP(res, req)

	assert.Equal(t, http.StatusOK, res.Code)
	var resources List
	if err := json.NewDecoder(res.Body).Decode(&resources); err != nil {
		t.Fail()
	}
	assert.Equal(t, coreResources, resources.Items)
}

func TestGetCoreNamespacedResourcesByName(t *testing.T) {
	router := setupRouter(fake.NewFakeDatabase(testResources, testLogUrls), true)

	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/namespace/test/pods/test", nil)
	router.ServeHTTP(res, req)

	assert.Equal(t, http.StatusOK, res.Code)
	var resource *unstructured.Unstructured
	if err := json.NewDecoder(res.Body).Decode(&resource); err != nil {
		t.Fail()
	}
	assert.Equal(t, coreResources[0], resource)
}

func TestDBError(t *testing.T) {
	router := setupRouter(fake.NewFakeDatabaseWithError(errors.New("test error")), true)

	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/pods", nil)
	router.ServeHTTP(res, req)

	assert.Equal(t, http.StatusInternalServerError, res.Code)
	assert.Contains(t, res.Body.String(), "test error")
	assert.NotContains(t, res.Body.String(), "Kind")
}

func TestNoAPIResourceInContextError(t *testing.T) {

	// Setting a router without a middleware that sets the api resource
	router := gin.Default()
	ctrl := Controller{Database: fake.NewFakeDatabase(testResources, testLogUrls)}
	router.GET("/api/:version/:resourceType", ctrl.GetAllCoreResources)

	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/pods", nil)
	router.ServeHTTP(res, req)

	assert.Equal(t, http.StatusInternalServerError, res.Code)
	assert.Contains(t, res.Body.String(), "API resource not found")
	assert.NotContains(t, res.Body.String(), "Kind")
}

func TestLivez(t *testing.T) {

	router := gin.Default()
	ctrl := Controller{Database: fake.NewFakeDatabase(testResources, testLogUrls),
		CacheConfiguration: CacheExpirations{Authorized: 60, Unauthorized: 5}}
	router.GET("/livez", ctrl.Livez)
	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/livez", nil)
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
			ctrl = Controller{Database: fake.NewFakeDatabase(testResources, testLogUrls)}
		} else {
			ctrl = Controller{Database: fake.NewFakeDatabaseWithError(errors.New("test error"))}
		}
		router.GET("/readyz", ctrl.Readyz)
		res := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
		router.ServeHTTP(res, req)

		assert.Equal(t, testCase.expected, res.Code)
	}
}
