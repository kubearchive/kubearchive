// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"fmt"
	"net/http"

	"github.com/kubearchive/kubearchive/cmd/api/abort"

	"github.com/gin-gonic/gin"
	apiAuthnv1 "k8s.io/api/authentication/v1"
	apiAuthzv1 "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientAuthzv1 "k8s.io/client-go/kubernetes/typed/authorization/v1"
)

func RBACAuthorization(sari clientAuthzv1.SubjectAccessReviewInterface) gin.HandlerFunc {

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

		sar, err := sari.Create(c.Request.Context(), &apiAuthzv1.SubjectAccessReview{
			Spec: apiAuthzv1.SubjectAccessReviewSpec{
				User:   userInfo.Username,
				Groups: userInfo.Groups,
				ResourceAttributes: &apiAuthzv1.ResourceAttributes{
					Namespace: c.Param("namespace"),
					Group:     c.Param("group"),
					Version:   c.Param("version"),
					Resource:  c.Param("resourceType"),
					Verb:      "get",
				},
			},
		}, metav1.CreateOptions{})

		if err != nil {
			abort.Abort(c, fmt.Sprintf("Unexpected error on SAR: %s", err.Error()), http.StatusInternalServerError)
			return
		}
		if !sar.Status.Allowed {
			abort.Abort(c, "Unauthorized", http.StatusUnauthorized)
			return
		}
		c.Next()
	}
}
