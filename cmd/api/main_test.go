// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"k8s.io/client-go/kubernetes"
	fakeK8s "k8s.io/client-go/kubernetes/fake"

	"github.com/kubearchive/kubearchive/cmd/api/routers"
	fakeDB "github.com/kubearchive/kubearchive/pkg/database/fake"
)

func fakeServer(k8sClient kubernetes.Interface) *Server {
	if k8sClient == nil {
		k8sClient = fakeK8s.NewSimpleClientset()
	}
	controller := routers.Controller{Database: fakeDB.NewFakeDatabase(nil)}
	return NewServer(k8sClient, controller)
}

func TestNewServer(t *testing.T) {
	k8sClient := fakeK8s.NewSimpleClientset()
	server := fakeServer(k8sClient)
	assert.NotNil(t, server.router)
	assert.Equal(t, server.k8sClient, k8sClient)
}

func TestOtelMiddlewareConfigured(t *testing.T) {
	// Set up server
	server := fakeServer(nil)
	// Get the context for a new response recorder for inspection and set it to the router engine
	c := gin.CreateTestContextOnly(httptest.NewRecorder(), server.router)
	c.Request, _ = http.NewRequest(http.MethodGet, "/", nil)
	server.router.HandleContext(c)
	// Get the handler names from the context
	names := c.HandlerNames()
	// Test that the last handlers in the chain are the expected ones
	// The full handler names may be different when running in debug mode
	expectedNames := []string{
		"otelgin.Middleware",
		"Authentication",
		"RBACAuthorization",
		"GetAPIResource",
	}
	for idx, name := range names[len(names)-len(expectedNames):] {
		assert.Contains(t, name, expectedNames[idx])
	}

}

func TestAuthMiddlewareConfigured(t *testing.T) {
	// Set up server
	server := fakeServer(nil)
	// Make a correct request with an invalid token
	res := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer token")
	server.router.ServeHTTP(res, req)
	// Assert unauthenticated request
	assert.Equal(t, http.StatusUnauthorized, res.Code)
}
