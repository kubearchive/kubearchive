// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"k8s.io/client-go/kubernetes"
	fakeK8s "k8s.io/client-go/kubernetes/fake"

	"github.com/kubearchive/kubearchive/cmd/api/routers"
	"github.com/kubearchive/kubearchive/pkg/cache"
	fakeDB "github.com/kubearchive/kubearchive/pkg/database/fake"
)

func fakeServer(k8sClient kubernetes.Interface, cache *cache.Cache) *Server {
	if k8sClient == nil {
		// Replacing deprecated NewSimpleClientset for NewClientset fails because of TokenReview
		// It may be related to https://github.com/kubernetes/kubernetes/issues/126850 so keeping deprecated.
		k8sClient = fakeK8s.NewSimpleClientset()
	}
	controller := routers.Controller{Database: fakeDB.NewFakeDatabase(nil)}
	expirations := &routers.CacheExpirations{Authorized: 1 * time.Second, Unauthorized: 1 * time.Second}
	return NewServer(k8sClient, controller, cache, expirations)
}

func TestNewServer(t *testing.T) {
	k8sClient := fakeK8s.NewClientset()
	memCache := cache.New()

	server := fakeServer(k8sClient, memCache)
	assert.NotNil(t, server.router)
	assert.Equal(t, server.k8sClient, k8sClient)
}

func TestMiddlewareConfigured(t *testing.T) {
	// Set up server
	server := fakeServer(nil, nil)

	testCases := []struct {
		name               string
		path               string
		hasControllerRoute bool
		expectedMiddleware []string
	}{
		{
			name:               "Root group",
			path:               "/",
			hasControllerRoute: false,
			expectedMiddleware: []string{"otelgin.Middleware"},
		},
		{
			name:               "API group",
			path:               "/api/v1/jobs",
			hasControllerRoute: true,
			expectedMiddleware: []string{"otelgin.Middleware", "Authentication", "RBACAuthorization", "GetAPIResource"},
		},
		{
			name:               "APIs group",
			path:               "/apis/apps/v1/deployments",
			hasControllerRoute: true,
			expectedMiddleware: []string{"otelgin.Middleware", "Authentication", "RBACAuthorization", "GetAPIResource"},
		},
	}

	for _, testCase := range testCases {
		// Get the context for a new response recorder for inspection and set it to the router engine
		c := gin.CreateTestContextOnly(httptest.NewRecorder(), server.router)
		c.Request, _ = http.NewRequest(http.MethodGet, testCase.path, nil)
		server.router.HandleContext(c)
		// Get the handler names from the context
		handlers := c.HandlerNames()
		// Test that the last handlers in the chain are the expected ones
		// The full handler names may be different when running in debug mode
		// When the path actually matches a route the last handler is the route method
		var middlewareHandlers []string
		if testCase.hasControllerRoute {
			middlewareHandlers = handlers[len(handlers)-len(testCase.expectedMiddleware)-1 : len(handlers)-1]
		} else {
			middlewareHandlers = handlers[len(handlers)-len(testCase.expectedMiddleware):]
		}
		for idx, middlewareHandler := range middlewareHandlers {
			assert.Contains(t, middlewareHandler, testCase.expectedMiddleware[idx])
		}
	}
}

func TestUnauthQuery(t *testing.T) {
	// Set up server
	memCache := cache.New()
	server := fakeServer(nil, memCache)
	// Make a correct request with an invalid token
	res := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/jobs", nil)
	req.Header.Set("Authorization", "Bearer token")
	server.router.ServeHTTP(res, req)
	// Assert unauthenticated request
	assert.Equal(t, http.StatusUnauthorized, res.Code)
}

func TestGetCacheExpirationsErrorPaths(t *testing.T) {
	testCases := []struct {
		name                   string
		authExpirationString   string
		unauthExpirationString string
		errorContains          string
	}{
		{
			name:                   "AUTH not set",
			authExpirationString:   "",
			unauthExpirationString: "",
			errorContains:          "CACHE_EXPIRATION_AUTHORIZED' should be set.",
		},
		{
			name:                   "AUTH wrong",
			authExpirationString:   "10",
			unauthExpirationString: "",
			errorContains:          "'10' could not be parsed into a duration",
		},
		{
			name:                   "UNAUTH not set",
			authExpirationString:   "10s",
			unauthExpirationString: "",
			errorContains:          "CACHE_EXPIRATION_UNAUTHORIZED' should be set.",
		},
		{
			name:                   "UNAUTH wrong",
			authExpirationString:   "10s",
			unauthExpirationString: "10",
			errorContains:          "'10' could not be parsed into a duration",
		},
	}

	for _, tc := range testCases {
		t.Setenv(cacheExpirationAuthorizedEnvVar, tc.authExpirationString)
		t.Setenv(cacheExpirationUnauthorizedEnvVar, tc.unauthExpirationString)

		_, err := getCacheExpirations()

		assert.ErrorContains(t, err, tc.errorContains)
	}
}

func TestGetCacheExpirationsWorkingProperly(t *testing.T) {
	t.Setenv(cacheExpirationAuthorizedEnvVar, "10s")
	t.Setenv(cacheExpirationUnauthorizedEnvVar, "10s")

	expirations, err := getCacheExpirations()

	assert.NoError(t, err)
	assert.Equal(t, 10*time.Second, expirations.Authorized)
	assert.Equal(t, 10*time.Second, expirations.Unauthorized)
}
