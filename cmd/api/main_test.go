// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"github.com/gin-gonic/gin"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/client-go/kubernetes/fake"
)

func TestNewServer(t *testing.T) {
	k8sClient := fake.NewSimpleClientset()
	server := NewServer(k8sClient)
	assert.NotNil(t, server.router)
	assert.Equal(t, server.k8sClient, k8sClient)
}

func TestOtelMiddlewareConfigured(t *testing.T) {
	// Set up server
	k8sClient := fake.NewSimpleClientset()
	server := NewServer(k8sClient)
	// Get the context for a new response recorder for inspection and set it to the router engine
	c := gin.CreateTestContextOnly(httptest.NewRecorder(), server.router)
	c.Request, _ = http.NewRequest("GET", "/", nil)
	server.router.HandleContext(c)
	// Get the handler names from the context
	names := c.HandlerNames()
	// Test that the last handlers in the chain are the expected ones
	// The full handler names may be different when running in debug mode
	expectedNames := []string{
		"otelgin.Middleware",
		"Authentication",
		"RBACAuthorization",
	}
	for idx, name := range names[len(names)-len(expectedNames):] {
		assert.Contains(t, name, expectedNames[idx])
	}

}

func TestAuthMiddlewareConfigured(t *testing.T) {
	// Set up server
	k8sClient := fake.NewSimpleClientset()
	server := NewServer(k8sClient)
	// Make a correct request with an invalid token
	res := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer token")
	server.router.ServeHTTP(res, req)
	// Assert unauthenticated request
	assert.Equal(t, http.StatusUnauthorized, res.Code)
}
