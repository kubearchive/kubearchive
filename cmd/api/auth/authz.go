// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/kubearchive/kubearchive/cmd/api/abort"
	"github.com/kubearchive/kubearchive/pkg/cache"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/gin-gonic/gin"
	apiAuthnv1 "k8s.io/api/authentication/v1"
	apiAuthzv1 "k8s.io/api/authorization/v1"
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

		verb := "list"
		if c.Param("name") != "" {
			verb = "get"
		}

		sarSpecs := []apiAuthzv1.SubjectAccessReviewSpec{
			{
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
			}}

		if strings.HasSuffix(c.Request.URL.Path, "/log") {
			logSpec := apiAuthzv1.SubjectAccessReviewSpec{
				User:   userInfo.Username,
				Groups: userInfo.Groups,
				ResourceAttributes: &apiAuthzv1.ResourceAttributes{
					Namespace: c.Param("namespace"),
					Version:   "v1",
					Resource:  "pods/log",
					Verb:      "get",
				},
			}
			sarSpecs = append(sarSpecs, logSpec)
		}

		for _, sarSpec := range sarSpecs {
			allowed := cache.Get(sarSpec.String())
			if allowed != nil {
				if allowed != true {
					abort.Abort(c, "Unauthorized", http.StatusUnauthorized)
					return
				}
				continue
			}
			sar, err := sari.Create(c.Request.Context(), &apiAuthzv1.SubjectAccessReview{
				Spec: sarSpec,
			}, metav1.CreateOptions{})

			if err != nil {
				abort.Abort(c, fmt.Sprintf("Unexpected error on SAR: %s", err.Error()), http.StatusInternalServerError)
				return
			}

			cache.Set(sarSpec.String(), sar.Status.Allowed, cacheExpirationAuthorized)

			if !sar.Status.Allowed {
				cache.Set(sarSpec.String(), sar.Status.Allowed, cacheExpirationUnauthorized)
				abort.Abort(c, "Unauthorized", http.StatusUnauthorized)
				return
			}
		}
	}
}
