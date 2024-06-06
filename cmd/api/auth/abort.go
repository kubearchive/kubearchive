// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"github.com/gin-gonic/gin"
	"log"
)

func abort(c *gin.Context, msg string, code int) {
	log.Println(msg)
	c.JSON(code, gin.H{"message": msg})
	c.Abort()
}
