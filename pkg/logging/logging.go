// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0
package logging

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/trace"
)

type contextKey string

const (
	loggingLevelEnvVar            = "LOG_LEVEL"
	traceIDKey         contextKey = "trace-id"
	spanIDKey          contextKey = "span-id"
)

// ContextHandler is an slog.Handler that adds context properties to
// the record being printed. Currently adding 'trace-id' and 'span-id'
// may add more in the future
type ContextHandler struct {
	slog.Handler
}

func (h *ContextHandler) Handle(ctx context.Context, r slog.Record) error {
	if traceID, ok := ctx.Value(traceIDKey).(string); ok {
		r.AddAttrs(slog.String(string(traceIDKey), traceID))
	}

	if spanID, ok := ctx.Value(spanIDKey).(string); ok {
		r.AddAttrs(slog.String(string(spanIDKey), spanID))
	}

	return h.Handler.Handle(ctx, r)
}

// ConfigureLogging retrieves the variable LOG_LEVEL
// from the environment and sets the slog default logger
// with a ContextHandler configured with the LOG_LEVEL level
// Returns an error if the level does not exist
func ConfigureLogging() error {
	levelText := os.Getenv(loggingLevelEnvVar)
	if levelText == "" {
		levelText = "INFO"
	}

	var level slog.Level
	err := level.UnmarshalText([]byte(levelText))
	if err != nil {
		return fmt.Errorf("log level '%s' does not exist", levelText)
	}

	textHandler := slog.NewTextHandler(
		os.Stdout,
		&slog.HandlerOptions{
			Level:     level,
			AddSource: true,
		},
	)

	slogLogger := slog.New(&ContextHandler{Handler: textHandler})
	slog.SetDefault(slogLogger)

	return nil
}

// TracingMiddleware sets trace and span keys in the request's context
// from the span already present in the context, if any, or it generates
// them
func TracingMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		span := trace.SpanContextFromContext(ctx)

		var traceID string
		if span.HasTraceID() {
			traceID = span.TraceID().String()
		} else {
			// 16 bytes as per definition: https://opentelemetry.io/docs/specs/otel/trace/api/#spancontext
			buf := make([]byte, 16)
			_, _ = rand.Read(buf)
			traceID = hex.EncodeToString(buf)
		}
		ctx = context.WithValue(ctx, traceIDKey, traceID)

		var spanID string
		if span.HasSpanID() {
			spanID = span.SpanID().String()
		} else {
			// 8 bytes as per definition: https://opentelemetry.io/docs/specs/otel/trace/api/#spancontext
			buf := make([]byte, 8)
			_, _ = rand.Read(buf)
			spanID = hex.EncodeToString(buf)
		}
		ctx = context.WithValue(ctx, spanIDKey, spanID)

		c.Request = c.Request.WithContext(ctx)

		c.Next()
	}
}
