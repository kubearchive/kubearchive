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
	group     = "stable.example.com"
	version   = "v1"
	resource  = "crontabs"
	username  = "fakeusername"
	usergroup = "fakeGroup1"
)

type fakeSubjectAccessReviews struct {
	allowed []bool
	index   int
	sar     []*apiAuthzv1.SubjectAccessReview
}

func (c *fakeSubjectAccessReviews) Create(ctx context.Context, subjectAccessReview *apiAuthzv1.SubjectAccessReview, opts metav1.CreateOptions) (*apiAuthzv1.SubjectAccessReview, error) {
	subjectAccessReview.Status.Allowed = c.allowed[c.index]
	if c.sar == nil {
		c.sar = []*apiAuthzv1.SubjectAccessReview{subjectAccessReview}
	} else {
		c.sar = append(c.sar, subjectAccessReview)
	}
	c.index++
	return subjectAccessReview, nil
}

var sarSpec = apiAuthzv1.SubjectAccessReviewSpec{
	User:   username,
	Groups: []string{usergroup},
	ResourceAttributes: &apiAuthzv1.ResourceAttributes{
		Group:    group,
		Version:  version,
		Resource: resource,
		Verb:     "list",
	},
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
			fsar := &fakeSubjectAccessReviews{allowed: []bool{tc.authorized}}
			res := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(res)
			c.Set("user", apiAuthnv1.UserInfo{Username: username, UID: "fake", Groups: []string{usergroup}})
			c.Params = gin.Params{
				gin.Param{Key: "group", Value: group},
				gin.Param{Key: "version", Value: version},
				gin.Param{Key: "resourceType", Value: resource},
				gin.Param{Key: "namespace", Value: tc.namespace},
				gin.Param{Key: "name", Value: tc.resourceName},
			}
			c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
			RBACAuthorization(fsar, cache, cacheExpirationDuration, cacheExpirationDuration)(c)
			assert.Equal(t, tc.expected, res.Code)
			ra := fsar.sar[0].Spec.ResourceAttributes
			assert.Equal(t, group, ra.Group)
			assert.Equal(t, resource, ra.Resource)
			assert.Equal(t, version, ra.Version)
			assert.Equal(t, tc.verb, ra.Verb)
			assert.Equal(t, tc.namespace, ra.Namespace)
			assert.Equal(t, tc.authorized, cache.Get(fsar.sar[0].Spec.String()), "Cache should be populated with the proper value.")
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
			name:     "Cache authorizes although the action is unauthorized",
			allowed:  false,
			expected: http.StatusOK,
		},
		{
			name:     "Cache unauthorizes although the action is authorized",
			allowed:  true,
			expected: http.StatusUnauthorized,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cache := cache.New()
			fsar := &fakeSubjectAccessReviews{allowed: []bool{tc.allowed}}
			cache.Set(sarSpec.String(), !tc.allowed, cacheExpirationDuration)

			res := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(res)
			c.Set("user", apiAuthnv1.UserInfo{Username: username, UID: "fake", Groups: []string{usergroup}})
			c.Params = gin.Params{
				gin.Param{Key: "group", Value: group},
				gin.Param{Key: "version", Value: version},
				gin.Param{Key: "resourceType", Value: resource},
			}
			c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

			RBACAuthorization(fsar, cache, cacheExpirationDuration, cacheExpirationDuration)(c)
			assert.Equal(t, !tc.allowed, cache.Get(sarSpec.String()))
			assert.Equal(t, tc.expected, res.Code)
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
			name:     "Unauthorized requests aren't cached",
			allowed:  false,
			expected: http.StatusUnauthorized,
		},
		{
			name:     "Authorized requests are cached",
			allowed:  true,
			expected: http.StatusOK,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cache := cache.New()
			fsar := &fakeSubjectAccessReviews{allowed: []bool{tc.allowed}}
			res := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(res)
			c.Set("user", apiAuthnv1.UserInfo{Username: username, UID: "fake", Groups: []string{usergroup}})
			c.Params = gin.Params{
				gin.Param{Key: "group", Value: group},
				gin.Param{Key: "version", Value: version},
				gin.Param{Key: "resourceType", Value: resource},
			}
			c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

			RBACAuthorization(fsar, cache, 10*time.Minute, -10*time.Minute)(c)

			if tc.allowed {
				assert.Equal(t, tc.allowed, cache.Get(sarSpec.String()), "Cache should be populated with allowed when authorized.")
			} else {
				assert.Equal(t, nil, cache.Get(sarSpec.String()), "Cache should be 'nil' because of negative expiration time.")
			}
			assert.Equal(t, tc.expected, res.Code)
		})
	}
}

func TestTwoSARForLogRequest(t *testing.T) {
	tests := []struct {
		name            string
		logsAllowed     bool
		resourceAllowed bool
		sarRequests     int
		expected        int
	}{
		{
			name:            "Logs and resource get allowed",
			logsAllowed:     true,
			resourceAllowed: true,
			sarRequests:     2,
			expected:        http.StatusOK,
		},
		{
			name:            "Logs allowed but resource get not",
			logsAllowed:     true,
			resourceAllowed: false,
			sarRequests:     1,
			expected:        http.StatusUnauthorized,
		},
		{
			name:            "Resource allowed but logs get not",
			logsAllowed:     false,
			resourceAllowed: true,
			sarRequests:     2,
			expected:        http.StatusUnauthorized,
		},
		{
			name:            "Nothing is allowed",
			logsAllowed:     false,
			resourceAllowed: false,
			sarRequests:     1,
			expected:        http.StatusUnauthorized,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cache := cache.New()
			fsar := &fakeSubjectAccessReviews{allowed: []bool{tc.resourceAllowed, tc.logsAllowed}}
			res := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(res)
			c.Set("user", apiAuthnv1.UserInfo{Username: username, UID: "fake", Groups: []string{usergroup}})
			c.Params = gin.Params{
				gin.Param{Key: "group", Value: group},
				gin.Param{Key: "version", Value: version},
				gin.Param{Key: "resourceType", Value: resource},
				gin.Param{Key: "namespace", Value: "ns"},
				gin.Param{Key: "name", Value: "resource"},
			}
			c.Request = httptest.NewRequest(http.MethodGet, "/log", nil)
			RBACAuthorization(fsar, cache, cacheExpirationDuration, cacheExpirationDuration)(c)
			assert.Equal(t, tc.expected, res.Code)
			assert.Equal(t, tc.sarRequests, len(fsar.sar))
			ra := fsar.sar[0].Spec.ResourceAttributes
			assert.Equal(t, group, ra.Group)
			assert.Equal(t, resource, ra.Resource)
			assert.Equal(t, version, ra.Version)
			assert.Equal(t, "get", ra.Verb)
			assert.Equal(t, "ns", ra.Namespace)
			assert.Equal(t, tc.resourceAllowed, cache.Get(fsar.sar[0].Spec.String()), "Cache should be populated with the proper value.")
			if tc.resourceAllowed {
				ra = fsar.sar[1].Spec.ResourceAttributes
				assert.NotEqual(t, group, ra.Group)
				assert.Equal(t, "pods/log", ra.Resource)
				assert.Equal(t, "v1", ra.Version)
				assert.Equal(t, "get", ra.Verb)
				assert.Equal(t, "ns", ra.Namespace)
				assert.Equal(t, tc.logsAllowed, cache.Get(fsar.sar[1].Spec.String()), "Cache should be populated with the proper value.")
			}
		})
	}
}
