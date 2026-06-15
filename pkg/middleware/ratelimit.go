// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package middleware

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/kubearchive/kubearchive/pkg/abort"
	"golang.org/x/time/rate"
)

// RateLimiter returns middleware that enforces a token-bucket rate limit.
// Requests exceeding the limit are rejected immediately with 429.
func RateLimiter(rps float64, burst int) gin.HandlerFunc {
	limiter := rate.NewLimiter(rate.Limit(rps), burst)
	return func(c *gin.Context) {
		if !limiter.Allow() {
			abort.Abort(c, errors.New("too many requests"), http.StatusTooManyRequests)
			return
		}
		c.Next()
	}
}

// ConcurrentLimiter returns middleware that enforces a maximum number of
// in-flight requests using a semaphore channel. Requests that arrive when
// the semaphore is full are rejected immediately with 429.
func ConcurrentLimiter(max int) gin.HandlerFunc {
	sem := make(chan struct{}, max)
	return func(c *gin.Context) {
		select {
		case sem <- struct{}{}:
		default:
			abort.Abort(c, errors.New("too many requests"), http.StatusTooManyRequests)
			return
		}
		defer func() { <-sem }()
		c.Next()
	}
}
