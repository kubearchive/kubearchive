// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	apiAuthnv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	fakeUserInfo = apiAuthnv1.UserInfo{Username: "fakeusername", UID: "fakeuid", Groups: []string{"fakegroup"}}
)

type fakeTokenReview struct {
	authenticated bool
	tr            *apiAuthnv1.TokenReview
}

func (c *fakeTokenReview) Create(ctx context.Context, tokenReview *apiAuthnv1.TokenReview, opts metav1.CreateOptions) (*apiAuthnv1.TokenReview, error) {
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
			ftr := &fakeTokenReview{authenticated: false}
			res := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(res)
			c.Request, _ = http.NewRequest(http.MethodGet, "", nil)
			c.Request.Header = tc.header
			Authentication(ftr)(c)
			assert.Equal(t, http.StatusBadRequest, res.Code)
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
			ftr := &fakeTokenReview{authenticated: tc.authenticated}
			res := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(res)
			c.Request, _ = http.NewRequest(http.MethodGet, "/", nil)
			c.Request.Header.Add("Authorization", "Bearer faketoken")
			Authentication(ftr)(c)
			assert.Equal(t, tc.expected, res.Code)
			assert.Equal(t, tc.authenticated, ftr.authenticated)
			assert.Equal(t, tc.user, ftr.tr.Status.User)
		})
	}
}
