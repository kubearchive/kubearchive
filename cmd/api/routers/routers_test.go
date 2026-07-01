// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package routers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kubearchive/kubearchive/cmd/api/pagination"
	"github.com/kubearchive/kubearchive/pkg/database/fake"
	"github.com/kubearchive/kubearchive/pkg/database/interfaces"
	"github.com/kubearchive/kubearchive/pkg/models"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

var testResources = fake.CreateTestResources()
var testLogUrls = fake.CreateTestLogUrls()
var nonCoreResources = testResources[:4] // First 4 are crontabs
var coreResources = testResources[4:5]   // Last 1 is pod

type List struct {
	Items []*unstructured.Unstructured
}

func retrieveLogURL(c *gin.Context) {
	recordVal, exists := c.Get("logRecord")
	if !exists || recordVal == nil {
		c.JSON(http.StatusNotFound, "no log record")
		return
	}
	record := recordVal.(*interfaces.LogRecord)
	c.JSON(http.StatusOK, fmt.Sprintf("%s-%s-%s-%s-%s-%s-%s", record.URL, record.Query, record.Start, record.End, record.PodName, record.PodUUID, record.ContainerName))
}

// buildRouter constructs a gin.Engine wired to ctrl with the standard test routes.
// setupRouter and setupRouterWithTimeout are thin wrappers around this function.
func buildRouter(ctrl Controller, core bool) *gin.Engine {
	router := gin.Default()
	router.Use(func(c *gin.Context) {
		if core {
			c.Set("apiResourceKind", "Pod")
		} else {
			c.Set("apiResourceKind", "Crontab")
		}
	})
	router.Use(pagination.Middleware())
	router.GET("/apis/:group/:version/:resourceType", ctrl.GetResources)
	router.GET("/apis/:group/:version/namespaces/:namespace/:resourceType", ctrl.GetResources)
	router.GET("/apis/:group/:version/namespaces/:namespace/:resourceType/:name", ctrl.GetResources)
	router.GET("/apis/:group/:version/namespaces/:namespace/:resourceType/uid/:uid", ctrl.GetResourceByUID)
	router.GET("/apis/:group/:version/namespaces/:namespace/:resourceType/:name/log",
		ctrl.GetLogURL, retrieveLogURL)
	router.GET("/apis/:group/:version/namespaces/:namespace/:resourceType/uid/:uid/log",
		ctrl.GetLogURL, retrieveLogURL)
	router.GET("/api/:version/:resourceType", ctrl.GetResources)
	router.GET("/api/:version/namespaces/:namespace/:resourceType", ctrl.GetResources)
	router.GET("/api/:version/namespaces/:namespace/:resourceType/:name", ctrl.GetResources)
	router.GET("/api/:version/namespaces/:namespace/:resourceType/uid/:uid", ctrl.GetResourceByUID)
	router.GET("/api/:version/namespaces/:namespace/:resourceType/:name/log",
		ctrl.GetLogURL, retrieveLogURL)
	router.GET("/api/:version/namespaces/:namespace/:resourceType/uid/:uid/log",
		ctrl.GetLogURL, retrieveLogURL)
	return router
}

// setupRouter sets up the router the same way that NewServer does without the middleware.
func setupRouter(db interfaces.DBReader, core bool) *gin.Engine {
	return buildRouter(Controller{Database: db}, core)
}

func TestLabelSelectorQueryParameter(t *testing.T) {
	router := setupRouter(fake.NewFakeDatabase(testResources, testLogUrls), false)
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
			expectedBody: fmt.Sprintf("\"%s-%s-%s-%s-%s-%s-%s\"", testLogUrls[0].Url, testLogUrls[0].Query, testLogUrls[0].Start, testLogUrls[0].End, "my-pod", testLogUrls[0].Uuid, ""),
		},
		{
			name:         "my-cronjob",
			api:          "/apis/batch/v1/namespaces/ns/cronjobs/my-cronjob/log",
			isCore:       false,
			expectedCode: http.StatusOK,
			expectedBody: fmt.Sprintf("\"%s-%s-%s-%s-%s-%s-%s\"", testLogUrls[0].Url, testLogUrls[0].Query, testLogUrls[0].Start, testLogUrls[0].End, "my-cronjob", testLogUrls[0].Uuid, ""),
		},
		{
			name:         "my-pod",
			api:          "/apis//v1/namespaces/ns/pods/my-pod/log",
			isCore:       true,
			expectedCode: http.StatusNotFound,
			expectedBody: "{\"message\":\"Not Found\"}",
		},
		{
			name:         "my-pod-uid",
			api:          fmt.Sprintf("/api/v1/namespaces/ns/pods/uid/%s/log", coreResources[0].GetUID()),
			isCore:       true,
			expectedCode: http.StatusOK,
			expectedBody: fmt.Sprintf("\"%s-%s-%s-%s-%s-%s-%s\"", testLogUrls[0].Url, testLogUrls[0].Query, testLogUrls[0].Start, testLogUrls[0].End, coreResources[0].GetUID(), testLogUrls[0].Uuid, ""),
		},
		{
			name:         "my-cronjob-uid",
			api:          fmt.Sprintf("/apis/batch/v1/namespaces/ns/cronjobs/uid/%s/log", nonCoreResources[0].GetUID()),
			isCore:       false,
			expectedCode: http.StatusOK,
			expectedBody: fmt.Sprintf("\"%s-%s-%s-%s-%s-%s-%s\"", testLogUrls[0].Url, testLogUrls[0].Query, testLogUrls[0].Start, testLogUrls[0].End, nonCoreResources[0].GetUID(), testLogUrls[0].Uuid, ""),
		},
		{
			name:         "not-found-uid",
			api:          "/apis//v1/namespaces/ns/pods/uid/not-found/log",
			isCore:       true,
			expectedCode: http.StatusNotFound,
			expectedBody: "{\"message\":\"Not Found\"}",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			router := setupRouter(fake.NewFakeDatabase(testResources, testLogUrls), test.isCore)
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
			router := setupRouter(fake.NewFakeDatabase(testResources, testLogUrls), test.isCore)
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

func TestGetResourceByUID(t *testing.T) {
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
			endpoint:       fmt.Sprintf("/apis/stable.example.com/v1/namespaces/test/crontabs/uid/%s", string(nonCoreResources[0].GetUID())),
			givenResources: testResources,
			expectedStatus: http.StatusOK,
			expectedBody:   string(nonCoreResourceBytes),
		},
		{
			name:           "Success namespaced core resource",
			isCore:         true,
			endpoint:       fmt.Sprintf("/api/v1/namespaces/test/pods/uid/%s", string(coreResources[0].GetUID())),
			givenResources: testResources,
			expectedStatus: http.StatusOK,
			expectedBody:   string(coreResourceBytes),
		},
		{
			name:           "Resource not found",
			isCore:         false,
			endpoint:       "/api/v1/namespaces/test/pods/uid/abcd",
			givenResources: testResources,
			expectedStatus: http.StatusNotFound,
			expectedBody:   "not found",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := setupRouter(fake.NewFakeDatabase(tt.givenResources, testLogUrls), tt.isCore)
			res := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, tt.endpoint, nil)
			router.ServeHTTP(res, req)
			assert.Equal(t, tt.expectedStatus, res.Code)
			assert.Contains(t, res.Body.String(), tt.expectedBody)
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
			router := setupRouter(fake.NewFakeDatabase(tt.givenResources, testLogUrls), tt.isCore)
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
	router := setupRouter(fake.NewFakeDatabase([]*unstructured.Unstructured{}, []fake.LogUrlRow{}), false)

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
	ctrl := Controller{Database: fake.NewFakeDatabase(testResources, testLogUrls)}
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

func TestWildcardNameFiltering(t *testing.T) {
	// Use the existing testResources which now include diverse crontab names

	tests := []struct {
		name          string
		api           string
		isCore        bool
		expectedCode  int
		expectedCount int // Number of resources expected in response
		description   string
	}{
		{
			name:          "wildcard middle match *e2e*",
			api:           "/apis/stable.example.com/v1/namespaces/test/crontabs?name=*e2e*",
			isCore:        false,
			expectedCode:  http.StatusOK,
			expectedCount: 2, // should match "test-e2e-job" and "my-e2e-service"
			description:   "Should match resources containing 'e2e' anywhere in name",
		},
		{
			name:          "wildcard prefix match test-*",
			api:           "/apis/stable.example.com/v1/namespaces/test/crontabs?name=test-*",
			isCore:        false,
			expectedCode:  http.StatusOK,
			expectedCount: 1, // should match "test-e2e-job"
			description:   "Should match resources starting with 'test-'",
		},
		{
			name:          "wildcard suffix match *-job",
			api:           "/apis/stable.example.com/v1/namespaces/test/crontabs?name=*-job",
			isCore:        false,
			expectedCode:  http.StatusOK,
			expectedCount: 1, // should match "test-e2e-job"
			description:   "Should match resources ending with '-job'",
		},
		{
			name:          "wildcard no matches *notfound*",
			api:           "/apis/stable.example.com/v1/namespaces/test/crontabs?name=*notfound*",
			isCore:        false,
			expectedCode:  http.StatusOK,
			expectedCount: 0, // should match nothing
			description:   "Should return empty list when no matches",
		},
		{
			name:          "wildcard case insensitive *E2E*",
			api:           "/apis/stable.example.com/v1/namespaces/test/crontabs?name=*E2E*",
			isCore:        false,
			expectedCode:  http.StatusOK,
			expectedCount: 2, // should match "test-e2e-job" and "my-e2e-service" (case insensitive)
			description:   "Should match case insensitively",
		},
		{
			name:          "exact name match (no wildcard)",
			api:           "/apis/stable.example.com/v1/namespaces/test/crontabs?name=test-e2e-job",
			isCore:        false,
			expectedCode:  http.StatusOK,
			expectedCount: 1, // should match exact name and return single resource, not list
			description:   "Should return single resource for exact match",
		},
		{
			name:          "wildcard with core resources *test*",
			api:           "/api/v1/namespaces/test/pods?name=*test*",
			isCore:        true,
			expectedCode:  http.StatusOK,
			expectedCount: 1, // should match "test" pod
			description:   "Should work with core resources",
		},
		{
			name:          "conflicting path and query name parameters",
			api:           "/apis/stable.example.com/v1/namespaces/test/crontabs/existing-resource?name=*e2e*",
			isCore:        false,
			expectedCode:  http.StatusBadRequest,
			expectedCount: 0,
			description:   "Should return 400 when both path name and query name are provided",
		},
		{
			name:          "wildcard in path parameter",
			api:           "/apis/stable.example.com/v1/namespaces/test/crontabs/*e2e*",
			isCore:        false,
			expectedCode:  http.StatusBadRequest,
			expectedCount: 0,
			description:   "Should return 400 when wildcard characters are used in path parameter",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := setupRouter(fake.NewFakeDatabase(testResources, testLogUrls), tt.isCore)
			res := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, tt.api, nil)
			router.ServeHTTP(res, req)

			assert.Equal(t, tt.expectedCode, res.Code, tt.description)

			if tt.expectedCode == http.StatusOK {
				if tt.name == "exact name match (no wildcard)" {
					// For exact matches, expect a single resource (not a list)
					var resource unstructured.Unstructured
					err := json.NewDecoder(res.Body).Decode(&resource)
					assert.NoError(t, err, "Should decode single resource")
					assert.Equal(t, "test-e2e-job", resource.GetName(), "Should return exact match")
				} else {
					// For wildcard matches, expect a list
					var resources List
					err := json.NewDecoder(res.Body).Decode(&resources)
					assert.NoError(t, err, "Should decode resource list")
					assert.Equal(t, tt.expectedCount, len(resources.Items), tt.description)
				}
			} else if tt.expectedCode == http.StatusBadRequest {
				// For validation errors, check that we got an appropriate error message
				responseBody := res.Body.String()
				if tt.name == "conflicting path and query name parameters" {
					assert.Contains(t, responseBody, "cannot specify both path name parameter and query name parameter", "Should contain appropriate error message")
				} else if tt.name == "wildcard in path parameter" {
					assert.Contains(t, responseBody, "wildcard characters (*) are not allowed in path parameters", "Should contain appropriate error message")
				}
			}
		})
	}
}

func TestTimestampQueryParameters(t *testing.T) {
	router := setupRouter(fake.NewFakeDatabase(testResources, testLogUrls), false)
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
	router := setupRouter(fake.NewFakeDatabase(testResources, testLogUrls), true)

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

func TestCountResources(t *testing.T) {
	tests := []struct {
		name          string
		api           string
		isCore        bool
		expectedCode  int
		expectedCount int64
	}{
		{
			name:          "count non-core resources",
			api:           "/apis/stable.example.com/v1/namespaces/test/crontabs?count=true",
			isCore:        false,
			expectedCode:  http.StatusOK,
			expectedCount: int64(len(nonCoreResources)),
		},
		{
			name:          "count core resources",
			api:           "/api/v1/namespaces/test/pods?count=true",
			isCore:        true,
			expectedCode:  http.StatusOK,
			expectedCount: int64(len(coreResources)),
		},
		{
			name:          "count with no matching resources",
			api:           "/apis/stable.example.com/v1/namespaces/nonexistent/crontabs?count=true",
			isCore:        false,
			expectedCode:  http.StatusOK,
			expectedCount: 0,
		},
		{
			name:          "count with label selector",
			api:           "/apis/stable.example.com/v1/namespaces/test/crontabs?count=true&labelSelector=app%3Dtest",
			isCore:        false,
			expectedCode:  http.StatusOK,
			expectedCount: int64(len(nonCoreResources)),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := setupRouter(fake.NewFakeDatabase(testResources, testLogUrls), tt.isCore)
			res := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, tt.api, nil)
			router.ServeHTTP(res, req)

			assert.Equal(t, tt.expectedCode, res.Code)
			var response map[string]int64
			err := json.NewDecoder(res.Body).Decode(&response)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedCount, response["count"])
		})
	}
}

func TestCountResourcesWithError(t *testing.T) {
	router := setupRouter(fake.NewFakeDatabaseWithError(errors.New("database error")), false)
	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/apis/stable.example.com/v1/namespaces/test/crontabs?count=true", nil)
	router.ServeHTTP(res, req)
	assert.Equal(t, http.StatusInternalServerError, res.Code)
}

func TestCountResourcesWithInvalidParams(t *testing.T) {
	router := setupRouter(fake.NewFakeDatabase(testResources, testLogUrls), false)
	tests := []struct {
		name string
		api  string
	}{
		{
			name: "count with limit",
			api:  "/apis/stable.example.com/v1/namespaces/test/crontabs?count=true&limit=10",
		},
		{
			name: "count with continue",
			api:  "/apis/stable.example.com/v1/namespaces/test/crontabs?count=true&continue=MSAyMDI1LTAxLTAxVDAwOjAwOjAwWg==",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, tt.api, nil)
			router.ServeHTTP(res, req)
			assert.Equal(t, http.StatusBadRequest, res.Code)
		})
	}
}

// slowDBReader is a test double for interfaces.DBReader whose query methods block
// until the supplied context is cancelled, then return ctx.Err(). This lets us
// verify that the Controller's QueryTimeout correctly cancels in-flight DB calls.
type slowDBReader struct {
	// blockFor is the maximum time each method will block before checking the context.
	// Setting it longer than QueryTimeout guarantees the timeout fires first.
	blockFor time.Duration
}

func (s *slowDBReader) block(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(s.blockFor):
		return nil
	}
}

func (s *slowDBReader) QueryResources(ctx context.Context, _, _, _, _, _, _ string, _ *models.LabelFilters, _, _ *time.Time, _ int) ([]models.Resource, error) {
	return nil, s.block(ctx)
}

func (s *slowDBReader) StreamResources(queryCtx, _ context.Context, _, _, _, _, _, _ string, _ *models.LabelFilters, _, _ *time.Time, _ int, _ func(models.Resource) error) error {
	return s.block(queryCtx)
}

func (s *slowDBReader) CountResources(ctx context.Context, _, _, _, _ string, _ *models.LabelFilters, _, _ *time.Time) (int64, error) {
	return 0, s.block(ctx)
}

func (s *slowDBReader) QueryResourceByUID(ctx context.Context, _, _, _, _ string) (*models.Resource, error) {
	return nil, s.block(ctx)
}

func (s *slowDBReader) QueryLogURLByName(ctx context.Context, _, _, _, _, _ string) (*interfaces.LogRecord, error) {
	return nil, s.block(ctx)
}

func (s *slowDBReader) QueryLogURLByUID(ctx context.Context, _, _, _, _, _ string) (*interfaces.LogRecord, error) {
	return nil, s.block(ctx)
}

func (s *slowDBReader) Ping(_ context.Context) error { return nil }
func (s *slowDBReader) QueryDatabaseSchemaVersion(_ context.Context) (string, error) {
	return "", nil
}
func (s *slowDBReader) CloseDB() error                 { return nil }
func (s *slowDBReader) Init(_ map[string]string) error { return nil }

// setupRouterWithTimeout creates a test router backed by db and applies queryTimeout.
func setupRouterWithTimeout(db interfaces.DBReader, core bool, queryTimeout time.Duration) *gin.Engine {
	return buildRouter(Controller{Database: db, QueryTimeout: queryTimeout}, core)
}

// TestQueryTimeout verifies that Controller.QueryTimeout causes DB-bound handlers
// to abort with 500 (context deadline exceeded) before the slow DB finishes,
// and that the response arrives well within the blockFor budget.
func TestQueryTimeout(t *testing.T) {
	const timeout = 50 * time.Millisecond
	const blockFor = 500 * time.Millisecond // much longer than the timeout

	slow := &slowDBReader{blockFor: blockFor}

	tests := []struct {
		name   string
		isCore bool
		url    string
	}{
		{
			name:   "GetResources list times out",
			isCore: false,
			url:    "/apis/stable.example.com/v1/namespaces/test/crontabs",
		},
		{
			name:   "GetResources by name times out",
			isCore: false,
			url:    "/apis/stable.example.com/v1/namespaces/test/crontabs/my-crontab",
		},
		{
			name:   "GetResources count times out",
			isCore: false,
			url:    "/apis/stable.example.com/v1/namespaces/test/crontabs?count=true",
		},
		{
			name:   "GetResourceByUID times out",
			isCore: false,
			url:    "/apis/stable.example.com/v1/namespaces/test/crontabs/uid/some-uid",
		},
		{
			name:   "GetLogURL times out",
			isCore: true,
			url:    "/api/v1/namespaces/test/pods/my-pod/log",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := setupRouterWithTimeout(slow, tt.isCore, timeout)

			start := time.Now()
			res := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, tt.url, nil)
			router.ServeHTTP(res, req)
			elapsed := time.Since(start)

			// Handler must return an error status — the DB never returned data.
			assert.Equal(t, http.StatusInternalServerError, res.Code,
				"expected 500 when DB call times out")

			// The response must arrive well before the DB's blockFor duration,
			// confirming the timeout fired rather than the DB finishing naturally.
			assert.Less(t, elapsed, blockFor,
				"response took %v but blockFor is %v — timeout should have fired first", elapsed, blockFor)
		})
	}
}

// TestQueryTimeoutDisabled verifies that when QueryTimeout is zero the handler
// does NOT impose a timeout — the fast fake DB returns normally.
func TestQueryTimeoutDisabled(t *testing.T) {
	router := setupRouterWithTimeout(fake.NewFakeDatabase(testResources, testLogUrls), false, 0)

	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/apis/stable.example.com/v1/namespaces/test/crontabs", nil)
	router.ServeHTTP(res, req)

	assert.Equal(t, http.StatusOK, res.Code, "zero QueryTimeout should not block successful requests")
}
