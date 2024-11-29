// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
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
	controller := routers.Controller{Database: fakeDB.NewFakeDatabase(nil, nil, "")}
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
		expectedMiddleware []string
	}{
		{
			name:               "Root group",
			path:               "/",
			expectedMiddleware: []string{"otelgin.Middleware"},
		},
		{
			name:               "API group",
			path:               "/api/v1/jobs",
			expectedMiddleware: []string{"otelgin.Middleware", "Authentication", "RBACAuthorization", "GetAPIResource"},
		},
		{
			name:               "APIs group",
			path:               "/apis/apps/v1/deployments",
			expectedMiddleware: []string{"otelgin.Middleware", "Authentication", "RBACAuthorization", "GetAPIResource"},
		},
		{
			name: "Logs APIs group",
			path: "/apis/apps/v1/namespaces/ns/deployments/my-deploy/log",
			expectedMiddleware: []string{"otelgin.Middleware", "Authentication", "RBACAuthorization", "GetAPIResource",
				"SetLoggingCredentials", "LogRetrieval"},
		},
		{
			name: "Logs API group",
			path: "/api/v1/namespaces/ns/pods/my-pod/log",
			expectedMiddleware: []string{"otelgin.Middleware", "Authentication", "RBACAuthorization", "GetAPIResource",
				"SetLoggingCredentials", "LogRetrieval"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			c := gin.CreateTestContextOnly(httptest.NewRecorder(), server.router)
			c.Request = httptest.NewRequest(http.MethodGet, testCase.path, nil)
			server.router.HandleContext(c)

			handlers := c.HandlerNames()

			t.Log(handlers)
			for _, expectedHandler := range testCase.expectedMiddleware {
				expectedHandlerExists := false
				for _, handler := range handlers {
					if strings.Contains(handler, expectedHandler) {
						expectedHandlerExists = true
					}
				}

				if !expectedHandlerExists {
					t.Fatalf("Handler %s is not in the list of handlers", expectedHandler)
				}
			}
		})
	}
}

func TestUnauthQuery(t *testing.T) {
	// Set up server
	memCache := cache.New()
	server := fakeServer(nil, memCache)
	// Make a correct request with an invalid token
	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs", nil)
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
