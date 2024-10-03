// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kubearchive/kubearchive/pkg/cache"
	"github.com/stretchr/testify/assert"
	apiAuthnv1 "k8s.io/api/authentication/v1"
	apiAuthzv1 "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	group    = "stable.example.com"
	version  = "v1"
	resource = "crontabs"
)

type fakeSubjectAccessReviews struct {
	allowed bool
	sar     *apiAuthzv1.SubjectAccessReview
}

func (c *fakeSubjectAccessReviews) Create(ctx context.Context, subjectAccessReview *apiAuthzv1.SubjectAccessReview, opts metav1.CreateOptions) (*apiAuthzv1.SubjectAccessReview, error) {
	subjectAccessReview.Status.Allowed = c.allowed
	c.sar = subjectAccessReview
	return subjectAccessReview, nil
}

func TestAuthZMiddleware(t *testing.T) {
	tests := []struct {
		name         string
		authorized   bool
		resourceName string
		namespace    string
		verb         string
		expected     int
	}{
		{
			name:         "Unauthorized list core resource request",
			authorized:   false,
			resourceName: "",
			namespace:    "",
			verb:         "list",
			expected:     http.StatusUnauthorized,
		},
		{
			name:         "Authorized list core resource request",
			authorized:   true,
			resourceName: "",
			namespace:    "",
			verb:         "list",
			expected:     http.StatusOK,
		},
		{
			name:         "Unauthorized list namespaced resource request",
			authorized:   false,
			resourceName: "",
			namespace:    "ns",
			verb:         "list",
			expected:     http.StatusUnauthorized,
		},
		{
			name:         "Authorized list namespaced resource request",
			authorized:   true,
			resourceName: "",
			namespace:    "ns",
			verb:         "list",
			expected:     http.StatusOK,
		},
		{
			name:         "Unauthorized get namespaced resource request",
			authorized:   true,
			resourceName: "test-resource",
			namespace:    "ns",
			verb:         "get",
			expected:     http.StatusOK,
		},
		{
			name:         "Authorized list namespaced resource request",
			authorized:   true,
			resourceName: "test-resource",
			namespace:    "ns",
			verb:         "get",
			expected:     http.StatusOK,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cache := cache.New()
			fsar := &fakeSubjectAccessReviews{allowed: tc.authorized}
			res := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(res)
			c.Set("user", apiAuthnv1.UserInfo{Username: "fakeusername", UID: "fake", Groups: []string{"fakeGroup1"}})
			c.Params = gin.Params{
				gin.Param{Key: "group", Value: group},
				gin.Param{Key: "version", Value: version},
				gin.Param{Key: "resourceType", Value: resource},
				gin.Param{Key: "namespace", Value: tc.namespace},
				gin.Param{Key: "name", Value: tc.resourceName},
			}
			c.Request, _ = http.NewRequest(http.MethodGet, "", nil)
			RBACAuthorization(fsar, cache, cacheExpirationDuration, cacheExpirationDuration)(c)
			assert.Equal(t, tc.expected, res.Code)
			ra := fsar.sar.Spec.ResourceAttributes
			assert.Equal(t, group, ra.Group)
			assert.Equal(t, resource, ra.Resource)
			assert.Equal(t, version, ra.Version)
			assert.Equal(t, tc.verb, ra.Verb)
			assert.Equal(t, tc.namespace, ra.Namespace)
			assert.Equal(t, tc.authorized, cache.Get("fakeusername"), "Cache should be populated with the proper value.")
		})
	}
}

func TestAuthorizationCache(t *testing.T) {
	tests := []struct {
		name     string
		allowed  bool
		expected int
	}{
		{
			name:     "Unauthorized",
			allowed:  false,
			expected: http.StatusUnauthorized,
		},
		{
			name:     "Authorized",
			allowed:  true,
			expected: http.StatusOK,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cache := cache.New()
			cache.Set("fakeusername", !tc.allowed, cacheExpirationDuration)

			fsar := &fakeSubjectAccessReviews{allowed: tc.allowed}
			res := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(res)
			c.Set("user", apiAuthnv1.UserInfo{Username: "fakeusername", UID: "fake", Groups: []string{"fakeGroup1"}})
			c.Params = gin.Params{
				gin.Param{Key: "group", Value: group},
				gin.Param{Key: "version", Value: version},
				gin.Param{Key: "resourceType", Value: resource},
			}
			c.Request, _ = http.NewRequest(http.MethodGet, "", nil)

			RBACAuthorization(fsar, cache, cacheExpirationDuration, cacheExpirationDuration)(c)
			assert.Equal(t, !tc.allowed, cache.Get("fakeusername"))
		})
	}
}

func TestDifferentAuthorizationExpirations(t *testing.T) {
	tests := []struct {
		name     string
		allowed  bool
		expected int
	}{
		{
			name:     "Unauthorized",
			allowed:  false,
			expected: http.StatusUnauthorized,
		},
		{
			name:     "Authorized",
			allowed:  true,
			expected: http.StatusOK,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cache := cache.New()
			fsar := &fakeSubjectAccessReviews{allowed: tc.allowed}
			res := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(res)
			c.Set("user", apiAuthnv1.UserInfo{Username: "fakeusername", UID: "fake", Groups: []string{"fakeGroup1"}})
			c.Params = gin.Params{
				gin.Param{Key: "group", Value: group},
				gin.Param{Key: "version", Value: version},
				gin.Param{Key: "resourceType", Value: resource},
			}
			c.Request, _ = http.NewRequest(http.MethodGet, "", nil)

			RBACAuthorization(fsar, cache, 10*time.Minute, -10*time.Minute)(c)

			if tc.allowed {
				assert.Equal(t, tc.allowed, cache.Get("fakeusername"), "Cache should be populated with allowed when authenticated.")
			} else {
				assert.Equal(t, nil, cache.Get("fakeusername"), "Cache should be 'nil' because of negative expiration time.")
			}
		})
	}
}
