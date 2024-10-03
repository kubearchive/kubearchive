// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0
package middleware

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
)

func Logger(c *gin.Context) {
	start := time.Now()
	c.Next()
	end := time.Now()

	path := c.Request.URL.Path
	query := c.Request.URL.RawQuery
	if query != "" {
		path += "?" + query
	}

	slog.Info(
		"Served request",
		"method", c.Request.Method,
		"status", c.Writer.Status(),
		"latency", end.Sub(start),
		"client", c.ClientIP(),
		"path", path,
		"errors", c.Errors.ByType(1).String(),
	)
}
