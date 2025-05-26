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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	fakeUserInfo            = apiAuthnv1.UserInfo{Username: "fakeusername", UID: "fakeuid", Groups: []string{"fakegroup"}}
	cacheExpirationDuration = 10 * time.Minute
)

type fakeTokenReview struct {
	authenticated bool
	tr            *apiAuthnv1.TokenReview
}

func (c *fakeTokenReview) Create(_ context.Context, tokenReview *apiAuthnv1.TokenReview, _ metav1.CreateOptions) (*apiAuthnv1.TokenReview, error) {
	tokenReview.Status.Authenticated = c.authenticated
	if c.authenticated {
		tokenReview.Status.User = fakeUserInfo
	}
	c.tr = tokenReview
	return tokenReview, nil
}

func createHeader(key, value string) http.Header {
	header := http.Header{}
	header.Set(key, value)
	return header
}

func TestInvalidAuthHeader(t *testing.T) {

	tests := []struct {
		name   string
		header http.Header
	}{
		{
			name:   "no auth header",
			header: http.Header{},
		},
		{
			name:   "header without bearer keyword",
			header: createHeader("Authorization", "fakeusername"),
		},
		{
			name:   "header with typo",
			header: createHeader("Authoriation", "Bearer fakeusername"),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cacheStorage := cache.New()
			ftr := &fakeTokenReview{authenticated: false}
			res := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(res)
			c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
			c.Request.Header = tc.header

			Authentication(ftr, cacheStorage, cacheExpirationDuration, cacheExpirationDuration)(c)

			assert.Equal(t, http.StatusBadRequest, res.Code)
			assert.Equal(t, nil, cacheStorage.Get("fakeusername"), "Cache shouldn't be populated at this point in the code.")

			_, usrExists := c.Get("user")
			assert.False(t, usrExists, "user should not be set")
		})
	}
}

func TestAuthentication(t *testing.T) {
	tests := []struct {
		name          string
		authenticated bool
		expected      int
		user          apiAuthnv1.UserInfo
	}{
		{
			name:          "Not authenticated",
			authenticated: false,
			expected:      http.StatusUnauthorized,
			user:          apiAuthnv1.UserInfo{},
		},
		{
			name:          "Authenticated",
			authenticated: true,
			expected:      http.StatusOK,
			user:          fakeUserInfo,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cacheStorage := cache.New()
			ftr := &fakeTokenReview{authenticated: tc.authenticated}
			res := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(res)
			c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
			c.Request.Header.Add("Authorization", "Bearer faketoken")

			Authentication(ftr, cacheStorage, cacheExpirationDuration, cacheExpirationDuration)(c)

			assert.Equal(t, tc.expected, res.Code)
			assert.Equal(t, tc.authenticated, ftr.authenticated)
			assert.Equal(t, tc.user, ftr.tr.Status.User)
			usr, exists := c.Get("user")
			assert.Equal(t, tc.authenticated, exists)
			if tc.authenticated {
				assert.Equal(t, newDefaultInfoFromAuthN(tc.user), cacheStorage.Get("faketoken"), "Cache should be populated with user data when authorized.")
				assert.Equal(t, usr, newDefaultInfoFromAuthN(ftr.tr.Status.User))
			} else {
				assert.Equal(t, false, cacheStorage.Get("faketoken"), "Cache should be populated with 'false' when unauthorized.")
				assert.Empty(t, usr)
			}
		})
	}
}

func TestAuthenticationCache(t *testing.T) {
	tests := []struct {
		name          string
		authenticated bool
		expected      any
	}{
		{
			name:          "authenticated",
			authenticated: true,
			expected:      fakeUserInfo,
		},
		{
			name:          "unauthenticated",
			authenticated: false,
			expected:      false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ftr := &fakeTokenReview{authenticated: tc.authenticated}
			cacheStorage := cache.New()
			if tc.authenticated {
				cacheStorage.Set("faketoken", fakeUserInfo, cacheExpirationDuration)
			} else {
				cacheStorage.Set("faketoken", false, cacheExpirationDuration)
			}
			res := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(res)
			c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
			c.Request.Header.Add("Authorization", "Bearer faketoken")

			Authentication(ftr, cacheStorage, cacheExpirationDuration, cacheExpirationDuration)(c)

			value := cacheStorage.Get("faketoken")
			assert.Equal(t, tc.expected, value)
		})
	}
}

func TestDifferentAuthenticationExpirations(t *testing.T) {
	tests := []struct {
		name          string
		authenticated bool
		expected      any
	}{
		{
			name:          "authenticated",
			authenticated: true,
			expected:      fakeUserInfo,
		},
		{
			name:          "unauthenticated",
			authenticated: false,
			expected:      nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ftr := &fakeTokenReview{authenticated: tc.authenticated}
			cacheStorage := cache.New()
			res := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(res)
			c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
			c.Request.Header.Add("Authorization", "Bearer faketoken")

			Authentication(ftr, cacheStorage, 10*time.Minute, -10*time.Minute)(c)

			if expectedUser, ok := tc.expected.(apiAuthnv1.UserInfo); ok {
				assert.Equal(t, newDefaultInfoFromAuthN(expectedUser), cacheStorage.Get("faketoken"), "Authentication should be cached.")
			} else {
				assert.Equal(t, tc.expected, cacheStorage.Get("faketoken"), "Cache should be 'nil' because of negative expiration time.")
			}
		})
	}
}
