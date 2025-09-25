// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package routers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kubearchive/kubearchive/pkg/database/fake"
	"github.com/kubearchive/kubearchive/pkg/database/interfaces"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

var testResources = fake.CreateTestResources()
var testLogUrls = fake.CreateTestLogUrls()
var testLogJsonPath = "$."
var nonCoreResources = testResources[:1]
var coreResources = testResources[1:2]

type List struct {
	Items []*unstructured.Unstructured
}

func retrieveLogURL(c *gin.Context) {
	c.JSON(http.StatusOK, fmt.Sprintf("%s-%s", c.GetString("logURL"), c.GetString("jsonPath")))
}

// setupRouter set up the router the same way that NewServer does without the middleware
func setupRouter(db interfaces.DBReader, core bool) *gin.Engine {
	router := gin.Default()
	ctrl := Controller{Database: db}
	router.Use(func(c *gin.Context) {
		if core {
			c.Set("apiResourceKind", "Pod")
		} else {
			c.Set("apiResourceKind", "Crontab")
		}
	})
	router.GET("/apis/:group/:version/:resourceType", ctrl.GetResources)
	router.GET("/apis/:group/:version/namespaces/:namespace/:resourceType", ctrl.GetResources)
	router.GET("/apis/:group/:version/namespaces/:namespace/:resourceType/:name", ctrl.GetResources)
	router.GET("/apis/:group/:version/namespaces/:namespace/:resourceType/:name/log",
		ctrl.GetLogURL, retrieveLogURL)
	router.GET("/api/:version/:resourceType", ctrl.GetResources)
	router.GET("/api/:version/namespaces/:namespace/:resourceType", ctrl.GetResources)
	router.GET("/api/:version/namespaces/:namespace/:resourceType/:name", ctrl.GetResources)
	router.GET("/api/:version/namespaces/:namespace/:resourceType/:name/log",
		ctrl.GetLogURL, retrieveLogURL)
	return router
}

func TestLabelSelectorQueryParameter(t *testing.T) {
	router := setupRouter(fake.NewFakeDatabase(testResources, testLogUrls, testLogJsonPath), false)
	tests := []struct {
		name          string
		labelSelector string
		expected      int
	}{
		{
			name:          "empty label selector",
			labelSelector: "",
			expected:      200,
		},
		{
			name:          "invalid label selector operator",
			labelSelector: "app>kubearchive",
			expected:      400,
		},
		{
			name:          "valid exists default operator",
			labelSelector: "app",
			expected:      200,
		},
		{
			name:          "invalid exists operator",
			labelSelector: "exists app",
			expected:      400,
		},
		{
			name:          "valid not exists operator",
			labelSelector: "!app",
			expected:      200,
		},
		{
			name:          "valid equals operator",
			labelSelector: "app=kubearchive",
			expected:      200,
		},
		{
			name:          "invalid equals operator",
			labelSelector: "app==kubearchive",
			expected:      400,
		},
		{
			name:          "valid not equals operator",
			labelSelector: "app!=kubearchive",
			expected:      200,
		},
		{
			name:          "valid in operator",
			labelSelector: "app in (kubearchive, postgresql)",
			expected:      200,
		},
		{
			name:          "valid notin operator",
			labelSelector: "app notin (kubearchive, postgresql)",
			expected:      200,
		},
		{
			name:          "invalid notin operator",
			labelSelector: "app not in (kubearchive, postgresql)",
			expected:      400,
		},
		{
			name:          "all operators",
			labelSelector: "environment, !control-plane, app.kubernetes.io/part-of=kubernetes, environment!= dev, app in (kubearchive-api-server, kubearchive-sink), version notin (0.1, 0.2)",
			expected:      200,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := httptest.NewRecorder()
			req := httptest.NewRequest(
				http.MethodGet,
				fmt.Sprintf("/apis/batch/v1/namespaces/ns/cronjobs?labelSelector=%s", url.QueryEscape(tt.labelSelector)),
				nil,
			)
			router.ServeHTTP(res, req)
			assert.Equal(t, tt.expected, res.Code)
		})
	}
}

func TestGetResourcesLogURLS(t *testing.T) {
	tests := []struct {
		name         string
		api          string
		isCore       bool
		expectedCode int
		expectedBody string
	}{
		{
			name:         "my-pod",
			api:          "/api/v1/namespaces/ns/pods/my-pod/log",
			isCore:       true,
			expectedCode: http.StatusOK,
			expectedBody: fmt.Sprintf("\"%s-%s\"", testLogUrls[0].Url, testLogJsonPath),
		},
		{
			name:         "my-cronjob",
			api:          "/apis/batch/v1/namespaces/ns/cronjobs/my-cronjob/log",
			isCore:       false,
			expectedCode: http.StatusOK,
			expectedBody: fmt.Sprintf("\"%s-%s\"", testLogUrls[0].Url, testLogJsonPath),
		},
		{
			name:         "my-pod",
			api:          "/apis//v1/namespaces/ns/pods/my-pod/log",
			isCore:       true,
			expectedCode: http.StatusNotFound,
			expectedBody: "{\"message\":\"Not Found\"}",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			router := setupRouter(fake.NewFakeDatabase(testResources, testLogUrls, testLogJsonPath), test.isCore)
			res := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, test.api, nil)
			router.ServeHTTP(res, req)

			assert.Equal(t, test.expectedCode, res.Code)
			assert.Equal(t, test.expectedBody, res.Body.String())
		})
	}
}

func TestGetResources(t *testing.T) {
	tests := []struct {
		name              string
		api               string
		isCore            bool
		expectedCode      int
		expectedResources []*unstructured.Unstructured
	}{
		{
			name:              "crontabs",
			api:               "/apis/stable.example.com/v1/crontabs",
			isCore:            false,
			expectedCode:      http.StatusOK,
			expectedResources: nonCoreResources,
		},
		{
			name:              "namespaced crontabs",
			api:               "/apis/stable.example.com/v1/namespaces/test/crontabs",
			isCore:            false,
			expectedCode:      http.StatusOK,
			expectedResources: nonCoreResources,
		},
		{
			name:              "namespaced crontabs",
			api:               "/apis/stable.example.com/v1/namespaces/test/crontabs",
			isCore:            false,
			expectedCode:      http.StatusOK,
			expectedResources: nonCoreResources,
		},
		{
			name:              "pods",
			api:               "/api/v1/pods",
			isCore:            true,
			expectedCode:      http.StatusOK,
			expectedResources: coreResources,
		},
		{
			name:              "namespaced pods",
			api:               "/api/v1/namespaces/test/pods",
			isCore:            true,
			expectedCode:      http.StatusOK,
			expectedResources: coreResources,
		},
		{
			name:              "namespaced pods",
			api:               "/apis//v1/namespaces/test/pods",
			isCore:            true,
			expectedCode:      http.StatusNotFound,
			expectedResources: nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			router := setupRouter(fake.NewFakeDatabase(testResources, testLogUrls, testLogJsonPath), test.isCore)
			res := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, test.api, nil)
			router.ServeHTTP(res, req)

			assert.Equal(t, test.expectedCode, res.Code)
			var resources List
			if err := json.NewDecoder(res.Body).Decode(&resources); err != nil {
				t.Fail()
			}
			assert.Equal(t, test.expectedResources, resources.Items)
		})
	}
}

func TestGetResourceByName(t *testing.T) {
	nonCoreResourceBytes, _ := json.Marshal(nonCoreResources[0])
	coreResourceBytes, _ := json.Marshal(coreResources[0])
	tests := []struct {
		name           string
		isCore         bool
		endpoint       string
		givenResources []*unstructured.Unstructured
		expectedStatus int
		expectedBody   string
	}{
		{
			name:           "Success namespaced non-core resource",
			isCore:         false,
			endpoint:       "/apis/stable.example.com/v1/namespaces/test/crontabs/test",
			givenResources: testResources,
			expectedStatus: http.StatusOK,
			expectedBody:   string(nonCoreResourceBytes),
		},
		{
			name:           "Success namespaced core resource",
			isCore:         true,
			endpoint:       "/api/v1/namespaces/test/pods/test",
			givenResources: testResources,
			expectedStatus: http.StatusOK,
			expectedBody:   string(coreResourceBytes),
		},
		{
			name:           "More than one resource found",
			isCore:         false,
			endpoint:       "/apis/stable.example.com/v1/namespaces/test/crontabs/test",
			givenResources: append(testResources, testResources[0]),
			expectedStatus: http.StatusInternalServerError,
			expectedBody:   "more than one resource found",
		},
		{
			name:           "Failed named non-core resource across all namespaces",
			isCore:         false,
			endpoint:       "/apis/stable.example.com/v1/crontabs/test",
			givenResources: testResources,
			expectedStatus: http.StatusNotFound,
			expectedBody:   "not found",
		},
		{
			name:           "Failed named core resource across all namespaces",
			isCore:         true,
			endpoint:       "/api/v1/pods/test",
			givenResources: testResources,
			expectedStatus: http.StatusNotFound,
			expectedBody:   "not found",
		},
		{
			name:           "Resource not found",
			isCore:         false,
			endpoint:       "/api/v1/namespaces/test/pods/notfound",
			givenResources: testResources,
			expectedStatus: http.StatusNotFound,
			expectedBody:   "not found",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := setupRouter(fake.NewFakeDatabase(tt.givenResources, testLogUrls, testLogJsonPath), tt.isCore)
			res := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, tt.endpoint, nil)
			router.ServeHTTP(res, req)
			assert.Equal(t, tt.expectedStatus, res.Code)
			assert.Contains(t, res.Body.String(), tt.expectedBody)
		})
	}
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

func TestGetResourcesEmpty(t *testing.T) {
	router := setupRouter(fake.NewFakeDatabase([]*unstructured.Unstructured{}, []fake.LogUrlRow{}, testLogJsonPath), false)

	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/apis/stable.example.com/v1/namespaces/test/crontabs", nil)
	router.ServeHTTP(res, req)

	assert.Equal(t, http.StatusOK, res.Code)
	var resources List
	if err := json.NewDecoder(res.Body).Decode(&resources); err != nil {
		t.Fail()
	}
	assert.Equal(t, 0, len(resources.Items))
}

func TestNoAPIResourceInContextError(t *testing.T) {
	// Setting a router without a middleware that sets the api resource
	router := gin.Default()
	ctrl := Controller{Database: fake.NewFakeDatabase(testResources, testLogUrls, testLogJsonPath)}
	router.GET("/api/:version/:resourceType", ctrl.GetResources)

	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/pods", nil)
	router.ServeHTTP(res, req)

	assert.Equal(t, http.StatusInternalServerError, res.Code)
	assert.Contains(t, res.Body.String(), "API resource not found")
	assert.NotContains(t, res.Body.String(), "Kind")
}

func TestLivez(t *testing.T) {
	router := gin.Default()
	ctrl := Controller{Database: fake.NewFakeDatabase(testResources, testLogUrls, testLogJsonPath),
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
			ctrl = Controller{Database: fake.NewFakeDatabase(testResources, testLogUrls, testLogJsonPath)}
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

func TestTimestampQueryParameters(t *testing.T) {
	router := setupRouter(fake.NewFakeDatabase(testResources, testLogUrls, testLogJsonPath), false)
	tests := []struct {
		name                    string
		creationTimestampAfter  string
		creationTimestampBefore string
		expected                int
	}{
		{
			name:                    "empty timestamp filters",
			creationTimestampAfter:  "",
			creationTimestampBefore: "",
			expected:                200,
		},
		{
			name:                    "valid creationTimestampAfter",
			creationTimestampAfter:  "2023-01-01T00:00:00Z",
			creationTimestampBefore: "",
			expected:                200,
		},
		{
			name:                    "valid creationTimestampBefore",
			creationTimestampAfter:  "",
			creationTimestampBefore: "2023-12-31T23:59:59Z",
			expected:                200,
		},
		{
			name:                    "both timestamp filters",
			creationTimestampAfter:  "2023-01-01T00:00:00Z",
			creationTimestampBefore: "2023-12-31T23:59:59Z",
			expected:                200,
		},
		{
			name:                    "invalid creationTimestampAfter format",
			creationTimestampAfter:  "2023-01-01",
			creationTimestampBefore: "",
			expected:                400,
		},
		{
			name:                    "invalid creationTimestampBefore format",
			creationTimestampAfter:  "",
			creationTimestampBefore: "2023-12-31",
			expected:                400,
		},
		{
			name:                    "invalid both timestamp formats",
			creationTimestampAfter:  "2023-01-01",
			creationTimestampBefore: "2023-12-31",
			expected:                400,
		},
		{
			name:                    "invalid timestamp order - before is earlier than after",
			creationTimestampAfter:  "2023-12-31T23:59:59Z",
			creationTimestampBefore: "2023-01-01T00:00:00Z",
			expected:                400,
		},
		{
			name:                    "invalid timestamp order - same timestamps",
			creationTimestampAfter:  "2023-06-15T12:00:00Z",
			creationTimestampBefore: "2023-06-15T12:00:00Z",
			expected:                400,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requestURL := "/api/v1/pods"
			if tt.creationTimestampAfter != "" {
				requestURL += "?creationTimestampAfter=" + url.QueryEscape(tt.creationTimestampAfter)
			}
			if tt.creationTimestampBefore != "" {
				separator := "?"
				if strings.Contains(requestURL, "?") {
					separator = "&"
				}
				requestURL += separator + "creationTimestampBefore=" + url.QueryEscape(tt.creationTimestampBefore)
			}

			w := httptest.NewRecorder()
			req, _ := http.NewRequest(http.MethodGet, requestURL, nil)
			router.ServeHTTP(w, req)

			if w.Code != tt.expected {
				t.Errorf("expected status code %d, got %d", tt.expected, w.Code)
			}
		})
	}
}

func TestTimestampValidationErrorMessages(t *testing.T) {
	router := setupRouter(fake.NewFakeDatabase(testResources, testLogUrls, testLogJsonPath), true)

	tests := []struct {
		name                    string
		creationTimestampAfter  string
		creationTimestampBefore string
		expectedStatus          int
		expectedErrorMessage    string
	}{
		{
			name:                    "timestamp order validation - before earlier than after",
			creationTimestampAfter:  "2023-12-31T23:59:59Z",
			creationTimestampBefore: "2023-01-01T00:00:00Z",
			expectedStatus:          400,
			expectedErrorMessage:    "creationTimestampBefore must be after creationTimestampAfter",
		},
		{
			name:                    "timestamp order validation - equal timestamps",
			creationTimestampAfter:  "2023-06-15T12:00:00Z",
			creationTimestampBefore: "2023-06-15T12:00:00Z",
			expectedStatus:          400,
			expectedErrorMessage:    "creationTimestampBefore must be after creationTimestampAfter",
		},
		{
			name:                    "invalid timestamp format - after",
			creationTimestampAfter:  "invalid-timestamp",
			creationTimestampBefore: "",
			expectedStatus:          400,
			expectedErrorMessage:    "invalid creationTimestampAfter format",
		},
		{
			name:                    "invalid timestamp format - before",
			creationTimestampAfter:  "",
			creationTimestampBefore: "invalid-timestamp",
			expectedStatus:          400,
			expectedErrorMessage:    "invalid creationTimestampBefore format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requestURL := "/api/v1/pods"
			if tt.creationTimestampAfter != "" {
				requestURL += "?creationTimestampAfter=" + url.QueryEscape(tt.creationTimestampAfter)
			}
			if tt.creationTimestampBefore != "" {
				separator := "?"
				if strings.Contains(requestURL, "?") {
					separator = "&"
				}
				requestURL += separator + "creationTimestampBefore=" + url.QueryEscape(tt.creationTimestampBefore)
			}

			w := httptest.NewRecorder()
			req, _ := http.NewRequest(http.MethodGet, requestURL, nil)
			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
			assert.Contains(t, w.Body.String(), tt.expectedErrorMessage)
		})
	}
}
