// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0
package middleware

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
)

type LoggerConfig struct {
	PathLoggingLevel map[string]string // /livez=DEBUG for example
}

func Logger(conf LoggerConfig) gin.HandlerFunc {
	pathLevels := map[string]slog.Level{}
	for path, val := range conf.PathLoggingLevel {
		var level slog.Level
		err := level.UnmarshalText([]byte(val))
		if err != nil {
			slog.Warn("Log level does not exist, using 'INFO' instead.", "level", val, "path", path)
			level = slog.LevelInfo
		}

		pathLevels[path] = level
	}

	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		end := time.Now()

		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery
		if query != "" {
			path += "?" + query
		}

		logLevel, ok := pathLevels[path]
		if !ok {
			logLevel = slog.LevelInfo
		}

		slog.Log(
			c.Request.Context(),
			logLevel,
			"Served request",
			"method", c.Request.Method,
			"status", c.Writer.Status(),
			"latency", end.Sub(start),
			"client", c.ClientIP(),
			"path", path,
			"errors", c.Errors.ByType(1).String(),
		)
	}

}
