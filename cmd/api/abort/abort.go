// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package abort

import (
	"log/slog"

	"github.com/gin-gonic/gin"
)

func Abort(c *gin.Context, msg string, code int) {
	slog.Error(msg)
	c.JSON(code, gin.H{"message": msg})
	c.Abort()
}
