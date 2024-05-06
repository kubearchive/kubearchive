// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"context"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"net/http/httptest"
	"testing"
)

const (
	group    = "stable.example.com/v1"
	resource = "crontabs"
)

type fakeSubjectAccessReviews struct {
	allowed bool
	sar     *v1.SubjectAccessReview
}

func (c *fakeSubjectAccessReviews) Create(ctx context.Context, subjectAccessReview *v1.SubjectAccessReview, opts metav1.CreateOptions) (*v1.SubjectAccessReview, error) {
	subjectAccessReview.Status.Allowed = c.allowed
	c.sar = subjectAccessReview
	return subjectAccessReview, nil
}

func testHTTPResponse(t *testing.T, router *gin.Engine, status int) {
	res := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", fmt.Sprintf("/%s/%s", group, resource), nil)
	router.ServeHTTP(res, req)
	assert.Equal(t, status, res.Code)
}

func TestAuthZMiddleware(t *testing.T) {

	tests := []struct {
		name       string
		authorized bool
		expected   int
	}{
		{
			name:       "Unauthorized",
			authorized: false,
			expected:   http.StatusUnauthorized,
		},
		{
			name:       "Authorized",
			authorized: true,
			expected:   http.StatusOK,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fsar := &fakeSubjectAccessReviews{allowed: tc.authorized}
			router := gin.Default()
			router.GET("/:group/:version/:resourceType", RBACAuthorization(fsar),
				func(ctx *gin.Context) {
					if tc.authorized {
						ctx.Status(tc.expected)
					} else {
						t.Fail()
					}
				})
			testHTTPResponse(t, router, tc.expected)
			ra := fsar.sar.Spec.ResourceAttributes
			assert.Equal(t, group, ra.Group)
			assert.Equal(t, resource, ra.Resource)
			assert.Equal(t, "get", ra.Verb)
			assert.Equal(t, "", ra.Namespace)
		})
	}
}
