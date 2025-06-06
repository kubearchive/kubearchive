// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0
package auth

import (
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kubearchive/kubearchive/pkg/cache"
	"github.com/stretchr/testify/assert"
	apiAuthnv1 "k8s.io/api/authentication/v1"
	apiAuthzv1 "k8s.io/api/authorization/v1"
	"k8s.io/apiserver/pkg/authentication/user"
)

const (
	impersonatedUsername = "impersonated-username"
	impersonatedUID      = "impersonated-uid"
	impersonatedGroup1   = "group1"
	impersonatedGroup2   = "group2"
	impersonatedEmail    = "john.doe@kubearchive.org"
)

var (
	basicImpersonatedUser = &user.DefaultInfo{
		Name:   impersonatedUsername,
		Groups: []string{user.AllAuthenticated},
	}
	requesterUser = &user.DefaultInfo{
		Name:   "requesterFakeUsername",
		UID:    "requesterFakeUID",
		Groups: []string{"requesterFakeGroup"},
	}
)

func TestImpersonationSkipped(t *testing.T) {

	tests := []struct {
		name                  string
		authImpersonate       string
		impersonateUserHeader http.Header
		expectedUserInContext *user.DefaultInfo
	}{
		{
			name:                  "feature flag OFF and no impersonation data",
			authImpersonate:       "false",
			impersonateUserHeader: http.Header{},
			expectedUserInContext: requesterUser,
		},
		{
			name:                  "feature flag OFF but impersonation data",
			authImpersonate:       "false",
			impersonateUserHeader: createHeader(apiAuthnv1.ImpersonateUserHeader, impersonatedUsername),
			expectedUserInContext: requesterUser,
		},
		{
			name:                  "feature flag ON but no impersonation data",
			authImpersonate:       "true",
			impersonateUserHeader: http.Header{},
			expectedUserInContext: requesterUser,
		},
		{
			name:                  "feature flag ON with impersonation data",
			authImpersonate:       "true",
			impersonateUserHeader: createHeader(apiAuthnv1.ImpersonateUserHeader, impersonatedUsername),
			expectedUserInContext: basicImpersonatedUser,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cacheStorage := cache.New()
			t.Setenv(impersonateFlag, tt.authImpersonate)
			fsar := &fakeSubjectAccessReviews{allowed: []bool{true}}
			res := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(res)
			c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
			c.Set("user", requesterUser)
			c.Request.Header = tt.impersonateUserHeader

			Impersonation(fsar, cacheStorage, cacheExpirationDuration, cacheExpirationDuration)(c)

			assert.Equal(t, http.StatusOK, res.Code)
			usr, usrExists := c.Get("user")
			if !usrExists {
				t.Fatal("user not found in context")
			}
			userInContext, ok := usr.(*user.DefaultInfo)
			if !ok {
				t.Fatalf("user in context in unexpected type: %T", usr)
			}
			assert.Equal(t, tt.expectedUserInContext, userInContext)
		})
	}
}

func TestImpersonationHeadersValidation(t *testing.T) {

	tests := []struct {
		name                  string
		headers               map[string][]string
		fakeSarOutcomes       []bool
		expectedStatusCode    int
		expectedUserInContext *user.DefaultInfo
	}{
		{
			name: "only impersonated UID",
			headers: map[string][]string{
				apiAuthnv1.ImpersonateUIDHeader: {impersonatedUID},
			},
			fakeSarOutcomes:       []bool{true},
			expectedStatusCode:    http.StatusBadRequest,
			expectedUserInContext: requesterUser,
		},
		{
			name: "only impersonated group",
			headers: map[string][]string{
				apiAuthnv1.ImpersonateGroupHeader: {impersonatedGroup1, impersonatedGroup2},
			},
			fakeSarOutcomes:       []bool{true},
			expectedStatusCode:    http.StatusBadRequest,
			expectedUserInContext: requesterUser,
		},
		{
			name: "only impersonated extras",
			headers: map[string][]string{
				apiAuthnv1.ImpersonateUserExtraHeaderPrefix + "email": {impersonatedEmail},
			},
			fakeSarOutcomes:       []bool{true},
			expectedStatusCode:    http.StatusBadRequest,
			expectedUserInContext: requesterUser,
		},
		{
			name: "only impersonated user",
			headers: map[string][]string{
				apiAuthnv1.ImpersonateUserHeader: {impersonatedUsername},
			},
			fakeSarOutcomes:       []bool{true},
			expectedStatusCode:    http.StatusOK,
			expectedUserInContext: basicImpersonatedUser,
		},
		{
			name: "all impersonated info",
			headers: map[string][]string{
				apiAuthnv1.ImpersonateUserHeader:                      {impersonatedUsername},
				apiAuthnv1.ImpersonateUIDHeader:                       {impersonatedUID},
				apiAuthnv1.ImpersonateGroupHeader:                     {impersonatedGroup1, impersonatedGroup2},
				apiAuthnv1.ImpersonateUserExtraHeaderPrefix + "email": {impersonatedEmail},
			},
			fakeSarOutcomes:    []bool{true, true, true, true, true},
			expectedStatusCode: http.StatusOK,
			expectedUserInContext: &user.DefaultInfo{
				Name:   impersonatedUsername,
				UID:    impersonatedUID,
				Groups: []string{impersonatedGroup1, impersonatedGroup2, user.AllAuthenticated},
				Extra:  map[string][]string{textproto.CanonicalMIMEHeaderKey("email"): {impersonatedEmail}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cacheStorage := cache.New()
			t.Setenv(impersonateFlag, "true")
			fsar := &fakeSubjectAccessReviews{allowed: tt.fakeSarOutcomes}
			res := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(res)
			c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
			c.Set("user", requesterUser)
			for key, values := range tt.headers {
				for _, value := range values {
					c.Request.Header.Add(key, value)
				}
			}
			Impersonation(fsar, cacheStorage, cacheExpirationDuration, cacheExpirationDuration)(c)

			assert.Equal(t, tt.expectedStatusCode, res.Code)
			if tt.expectedStatusCode == http.StatusOK {
				assert.Equal(t, len(tt.fakeSarOutcomes), len(fsar.sar))
			}
			usr, usrExists := c.Get("user")
			if !usrExists {
				t.Fatal("user not found in context")
			}
			userInContext, ok := usr.(*user.DefaultInfo)
			if !ok {
				t.Fatalf("user in context in unexpected type: %T", usr)
			}
			assert.Equal(t, tt.expectedUserInContext, userInContext)
		})
	}
}

func TestImpersonationAuthz(t *testing.T) {
	expectedSars := []apiAuthzv1.SubjectAccessReviewSpec{
		{
			User:   requesterUser.Name,
			UID:    requesterUser.UID,
			Groups: requesterUser.Groups,
			ResourceAttributes: &apiAuthzv1.ResourceAttributes{
				Name:     impersonatedUsername,
				Verb:     "impersonate",
				Resource: "users",
			},
			Extra: map[string]apiAuthzv1.ExtraValue{},
		},
		{
			User:   requesterUser.Name,
			UID:    requesterUser.UID,
			Groups: requesterUser.Groups,
			ResourceAttributes: &apiAuthzv1.ResourceAttributes{
				Name:     impersonatedGroup1,
				Verb:     "impersonate",
				Resource: "groups",
			},
			Extra: map[string]apiAuthzv1.ExtraValue{},
		},
		{
			User:   requesterUser.Name,
			UID:    requesterUser.UID,
			Groups: requesterUser.Groups,
			ResourceAttributes: &apiAuthzv1.ResourceAttributes{
				Name:     impersonatedGroup2,
				Verb:     "impersonate",
				Resource: "groups",
			},
			Extra: map[string]apiAuthzv1.ExtraValue{},
		},
		{
			User:   requesterUser.Name,
			UID:    requesterUser.UID,
			Groups: requesterUser.Groups,
			ResourceAttributes: &apiAuthzv1.ResourceAttributes{
				Name:     user.AllAuthenticated,
				Verb:     "impersonate",
				Resource: "groups",
			},
			Extra: map[string]apiAuthzv1.ExtraValue{},
		},
	}
	tests := []struct {
		name                  string
		headers               map[string][]string
		fakeSarOutcomes       []bool
		expectedNumberOfSar   int
		expectedStatusCode    int
		expectedSars          []apiAuthzv1.SubjectAccessReviewSpec
		expectedUserInContext *user.DefaultInfo
	}{
		{
			name: "Several successful permission requests",
			headers: map[string][]string{
				apiAuthnv1.ImpersonateUserHeader:  {impersonatedUsername},
				apiAuthnv1.ImpersonateGroupHeader: {impersonatedGroup1, impersonatedGroup2},
			},
			fakeSarOutcomes:     []bool{true, true, true},
			expectedNumberOfSar: 3,
			expectedStatusCode:  http.StatusOK,

			expectedUserInContext: &user.DefaultInfo{
				Name:   impersonatedUsername,
				Groups: []string{impersonatedGroup1, impersonatedGroup2, user.AllAuthenticated},
			},
		},
		{
			name: "Several failed permission requests",
			headers: map[string][]string{
				apiAuthnv1.ImpersonateUserHeader:  {impersonatedUsername},
				apiAuthnv1.ImpersonateGroupHeader: {impersonatedGroup1, impersonatedGroup2},
			},
			fakeSarOutcomes:       []bool{false, false, false},
			expectedNumberOfSar:   1,
			expectedStatusCode:    http.StatusUnauthorized,
			expectedUserInContext: requesterUser,
		},
		{
			name: "After several successful permission requests one fails",
			headers: map[string][]string{
				apiAuthnv1.ImpersonateUserHeader:  {impersonatedUsername},
				apiAuthnv1.ImpersonateGroupHeader: {impersonatedGroup1, impersonatedGroup2},
			},
			fakeSarOutcomes:       []bool{true, false, false},
			expectedNumberOfSar:   2,
			expectedStatusCode:    http.StatusUnauthorized,
			expectedUserInContext: requesterUser,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cacheStorage := cache.New()
			t.Setenv(impersonateFlag, "true")
			fsar := &fakeSubjectAccessReviews{allowed: tt.fakeSarOutcomes}
			res := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(res)
			c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
			c.Set("user", requesterUser)
			for key, values := range tt.headers {
				for _, value := range values {
					c.Request.Header.Add(key, value)
				}
			}
			Impersonation(fsar, cacheStorage, cacheExpirationDuration, cacheExpirationDuration)(c)

			assert.Equal(t, tt.expectedStatusCode, res.Code)
			assert.Equal(t, tt.expectedNumberOfSar, len(fsar.sar))
			for idx, sar := range fsar.sar {
				assert.Equal(t, expectedSars[idx], sar.Spec)
			}
			usr, usrExists := c.Get("user")
			if !usrExists {
				t.Fatal("user not found in context")
			}
			userInContext, ok := usr.(*user.DefaultInfo)
			if !ok {
				t.Fatalf("user in context in unexpected type: %T", usr)
			}
			assert.Equal(t, tt.expectedUserInContext, userInContext)
		})
	}
}

func TestImpersonationCache(t *testing.T) {

	sarDef := apiAuthzv1.SubjectAccessReviewSpec{
		User:   requesterUser.Name,
		UID:    requesterUser.UID,
		Groups: requesterUser.Groups,
		ResourceAttributes: &apiAuthzv1.ResourceAttributes{
			Name:     impersonatedUsername,
			Resource: "users",
			Verb:     "impersonate",
		},
	}
	tests := []struct {
		name                  string
		allowed               bool
		expected              int
		expectedUserInContext *user.DefaultInfo
	}{
		{
			name:                  "Cache authorizes although the action is unauthorized",
			allowed:               false,
			expected:              http.StatusOK,
			expectedUserInContext: basicImpersonatedUser,
		},
		{
			name:                  "Cache unauthorizes although the action is authorized",
			allowed:               true,
			expected:              http.StatusUnauthorized,
			expectedUserInContext: requesterUser,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cacheStorage := cache.New()
			fsar := &fakeSubjectAccessReviews{allowed: []bool{tt.allowed}}
			cacheStorage.Set(sarDef.String(), !tt.allowed, cacheExpirationDuration)
			t.Setenv(impersonateFlag, "true")
			res := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(res)
			c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
			c.Set("user", requesterUser)
			c.Request.Header = createHeader(apiAuthnv1.ImpersonateUserHeader, impersonatedUsername)
			Impersonation(fsar, cacheStorage, cacheExpirationDuration, cacheExpirationDuration)(c)

			assert.Equal(t, tt.expected, res.Code)
			usr, usrExists := c.Get("user")
			if !usrExists {
				t.Fatal("user not found in context")
			}
			userInContext, ok := usr.(*user.DefaultInfo)
			if !ok {
				t.Fatalf("user in context in unexpected type: %T", usr)
			}
			assert.Equal(t, tt.expectedUserInContext, userInContext)
			assert.Empty(t, len(fsar.sar))
		})
	}
}

func TestImpersonateServiceAccount(t *testing.T) {
	expectedSAUser := &user.DefaultInfo{
		Name:   "system:serviceaccount:my-namespace:my-serviceaccount",
		Groups: []string{"system:serviceaccounts", "system:serviceaccounts:my-namespace", user.AllAuthenticated},
	}

	tests := []struct {
		name         string
		headers      map[string][]string
		sarOutcomes  []bool
		expectedSars []apiAuthzv1.SubjectAccessReviewSpec
	}{
		{
			name: "service account with group",
			headers: map[string][]string{
				apiAuthnv1.ImpersonateUserHeader:  {"system:serviceaccount:my-namespace:my-serviceaccount"},
				apiAuthnv1.ImpersonateGroupHeader: {"system:serviceaccounts", "system:serviceaccounts:my-namespace", user.AllAuthenticated},
			},
			sarOutcomes: []bool{true, true, true, true},
			expectedSars: []apiAuthzv1.SubjectAccessReviewSpec{
				{
					User:   requesterUser.Name,
					UID:    requesterUser.UID,
					Groups: requesterUser.Groups,
					ResourceAttributes: &apiAuthzv1.ResourceAttributes{
						Name:      "system:serviceaccount:my-namespace:my-serviceaccount",
						Namespace: "my-namespace",
						Verb:      "impersonate",
						Resource:  "serviceaccounts",
					},
					Extra: map[string]apiAuthzv1.ExtraValue{},
				},
				{
					User:   requesterUser.Name,
					UID:    requesterUser.UID,
					Groups: requesterUser.Groups,
					ResourceAttributes: &apiAuthzv1.ResourceAttributes{
						Name:     "system:serviceaccounts",
						Verb:     "impersonate",
						Resource: "groups",
					},
					Extra: map[string]apiAuthzv1.ExtraValue{},
				},
				{
					User:   requesterUser.Name,
					UID:    requesterUser.UID,
					Groups: requesterUser.Groups,
					ResourceAttributes: &apiAuthzv1.ResourceAttributes{
						Name:     "system:serviceaccounts:my-namespace",
						Verb:     "impersonate",
						Resource: "groups",
					},
					Extra: map[string]apiAuthzv1.ExtraValue{},
				},
				{
					User:   requesterUser.Name,
					UID:    requesterUser.UID,
					Groups: requesterUser.Groups,
					ResourceAttributes: &apiAuthzv1.ResourceAttributes{
						Name:     user.AllAuthenticated,
						Verb:     "impersonate",
						Resource: "groups",
					},
					Extra: map[string]apiAuthzv1.ExtraValue{},
				},
			},
		},
		{
			name: "service account without group",
			headers: map[string][]string{
				apiAuthnv1.ImpersonateUserHeader: {"system:serviceaccount:my-namespace:my-serviceaccount"},
			},
			sarOutcomes: []bool{true},
			expectedSars: []apiAuthzv1.SubjectAccessReviewSpec{
				{
					User:   requesterUser.Name,
					UID:    requesterUser.UID,
					Groups: requesterUser.Groups,
					ResourceAttributes: &apiAuthzv1.ResourceAttributes{
						Name:      "system:serviceaccount:my-namespace:my-serviceaccount",
						Namespace: "my-namespace",
						Verb:      "impersonate",
						Resource:  "serviceaccounts",
					},
					Extra: map[string]apiAuthzv1.ExtraValue{},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cacheStorage := cache.New()
			fsar := &fakeSubjectAccessReviews{allowed: tt.sarOutcomes}
			t.Setenv(impersonateFlag, "true")
			res := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(res)
			c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
			c.Set("user", requesterUser)
			for key, values := range tt.headers {
				for _, value := range values {
					c.Request.Header.Add(key, value)
				}
			}
			Impersonation(fsar, cacheStorage, cacheExpirationDuration, cacheExpirationDuration)(c)

			assert.Equal(t, http.StatusOK, res.Code)
			for idx, sar := range tt.expectedSars {
				assert.Equal(t, sar, fsar.sar[idx].Spec)
			}
			usr, usrExists := c.Get("user")
			if !usrExists {
				t.Fatal("user not found in context")
			}
			userInContext, ok := usr.(*user.DefaultInfo)
			if !ok {
				t.Fatalf("user in context in unexpected type: %T", usr)
			}
			assert.Equal(t, expectedSAUser, userInContext)
		})
	}
}
