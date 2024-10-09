// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"fmt"
	"net/http"
	"time"

	"github.com/kubearchive/kubearchive/cmd/api/abort"
	"github.com/kubearchive/kubearchive/pkg/cache"

	"github.com/gin-gonic/gin"
	apiAuthnv1 "k8s.io/api/authentication/v1"
	apiAuthzv1 "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientAuthzv1 "k8s.io/client-go/kubernetes/typed/authorization/v1"
)

func RBACAuthorization(sari clientAuthzv1.SubjectAccessReviewInterface, cache *cache.Cache, cacheExpirationAuthorized, cacheExpirationUnauthorized time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		usr, ok := c.Get("user")
		if !ok {
			abort.Abort(c, "user not found in context", http.StatusInternalServerError)
			return
		}
		userInfo, ok := usr.(apiAuthnv1.UserInfo)
		if !ok {
			abort.Abort(c, fmt.Sprintf("unexpected user type in context: %T", usr), http.StatusInternalServerError)
			return
		}

		allowed := cache.Get(userInfo.Username)
		if allowed != nil {
			if allowed != true {
				abort.Abort(c, "Unauthorized", http.StatusUnauthorized)
				return
			}

			c.Next()
			return
		}

		verb := "list"
		if c.Param("name") != "" {
			verb = "get"
		}

		sar, err := sari.Create(c.Request.Context(), &apiAuthzv1.SubjectAccessReview{
			Spec: apiAuthzv1.SubjectAccessReviewSpec{
				User:   userInfo.Username,
				Groups: userInfo.Groups,
				ResourceAttributes: &apiAuthzv1.ResourceAttributes{
					Namespace: c.Param("namespace"),
					Group:     c.Param("group"),
					Version:   c.Param("version"),
					Resource:  c.Param("resourceType"),
					Name:      c.Param("name"),
					Verb:      verb,
				},
			},
		}, metav1.CreateOptions{})

		if err != nil {
			abort.Abort(c, fmt.Sprintf("Unexpected error on SAR: %s", err.Error()), http.StatusInternalServerError)
			return
		}

		cache.Set(userInfo.Username, sar.Status.Allowed, cacheExpirationAuthorized)

		if !sar.Status.Allowed {
			cache.Set(userInfo.Username, sar.Status.Allowed, cacheExpirationUnauthorized)
			abort.Abort(c, "Unauthorized", http.StatusUnauthorized)
			return
		}
	}
}
