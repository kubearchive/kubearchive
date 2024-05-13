// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"fmt"
	"github.com/gin-gonic/gin"
	authzv1 "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientAuthzv1 "k8s.io/client-go/kubernetes/typed/authorization/v1"
	"net/http"
)

func RBACAuthorization(sari clientAuthzv1.SubjectAccessReviewInterface) gin.HandlerFunc {
	// FIXME Hardcoded credentials should be extracted from context when authN layer is in place
	user := "user"
	groups := []string{"system-authenticated"}

	return func(c *gin.Context) {
		group := c.Param("group")
		version := c.Param("version")
		// FIXME Resource type should be extracted from context instead from the plural
		resource := c.Param("resourceType")
		namespace := c.Param("namespace")

		sar, err := sari.Create(c, &authzv1.SubjectAccessReview{
			Spec: authzv1.SubjectAccessReviewSpec{
				User:   user,
				Groups: groups,
				ResourceAttributes: &authzv1.ResourceAttributes{
					Namespace: namespace,
					Group:     fmt.Sprintf("%s/%s", group, version),
					Resource:  resource,
					Verb:      "get",
				},
			},
		}, metav1.CreateOptions{})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"message": "Unexpected error on SAR"})
			c.Abort()
			return
		}
		if !sar.Status.Allowed {
			c.JSON(http.StatusUnauthorized, gin.H{"message": "Unauthorized"})
			c.Abort()
			return
		}
		c.Next()
	}
}
