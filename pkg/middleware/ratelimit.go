// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package middleware

import (
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/kubearchive/kubearchive/pkg/abort"
	"golang.org/x/time/rate"
)

const (
	rateLimitOverallRPSEnvVar      = "API_RATE_LIMIT_OVERALL_RPS"
	rateLimitLogRPSEnvVar          = "API_RATE_LIMIT_LOG_RPS"
	rateLimitOverallBurstEnvVar    = "API_RATE_LIMIT_OVERALL_BURST"
	rateLimitLogBurstEnvVar        = "API_RATE_LIMIT_LOG_BURST"
	maxConcurrentRequestsEnvVar    = "API_MAX_CONCURRENT_REQUESTS"
	maxConcurrentLogRequestsEnvVar = "API_MAX_CONCURRENT_LOG_REQUESTS"
)

// RateLimitConfig holds the configuration for the API rate limiters.
type RateLimitConfig struct {
	OverallRPS       float64
	LogRPS           float64
	OverallBurst     int
	LogBurst         int
	MaxConcurrent    int
	MaxConcurrentLog int
}

// RateLimiter returns middleware that enforces a token-bucket rate limit.
// Requests exceeding the limit are rejected immediately with 429 and a
// Retry-After header indicating how many seconds to wait (RFC 6585 §4).
func RateLimiter(rps float64, burst int) gin.HandlerFunc {
	limiter := rate.NewLimiter(rate.Limit(rps), burst)
	return func(c *gin.Context) {
		r := limiter.Reserve()
		if d := r.Delay(); d > 0 {
			r.Cancel()
			retryAfter := max(int(math.Ceil(d.Seconds())), 1)
			c.Header("Retry-After", strconv.Itoa(retryAfter))
			abort.Abort(c, errors.New("too many requests"), http.StatusTooManyRequests)
			return
		}
		c.Next()
	}
}

// userKey extracts a stable, opaque identifier for the caller from the
// request. The precedence order is:
//  1. Impersonate-User header — set by the nginx proxy for impersonated calls.
//  2. Bearer token from the Authorization header — used as an opaque key
//     without decoding or validation.
//  3. Client IP — fallback for unauthenticated or direct callers.
func userKey(c *gin.Context) string {
	if u := c.GetHeader("Impersonate-User"); u != "" {
		return u
	}
	if auth := c.GetHeader("Authorization"); auth != "" {
		if strings.HasPrefix(auth, "Bearer ") {
			return auth[len("Bearer "):]
		}
		return auth
	}
	return c.ClientIP()
}

// UserRateLimiter returns middleware that enforces a per-user token-bucket
// rate limit. The user identity is derived from the Impersonate-User header
// (set by the nginx proxy), the Authorization bearer token, or the client IP
// as a last resort. Each unique identity gets its own limiter so one user's
// burst cannot starve others.
//
// Requests exceeding the per-user limit are rejected immediately with 429 and
// a Retry-After header indicating how many seconds to wait (RFC 6585 §4).
//
// TODO: evict stale entries if user population grows unbounded.
func UserRateLimiter(rps float64, burst int) gin.HandlerFunc {
	var limiters sync.Map
	return func(c *gin.Context) {
		key := userKey(c)
		v, _ := limiters.LoadOrStore(key, rate.NewLimiter(rate.Limit(rps), burst))
		limiter := v.(*rate.Limiter)
		r := limiter.Reserve()
		if d := r.Delay(); d > 0 {
			r.Cancel()
			retryAfter := max(int(math.Ceil(d.Seconds())), 1)
			c.Header("Retry-After", strconv.Itoa(retryAfter))
			abort.Abort(c, errors.New("too many requests"), http.StatusTooManyRequests)
			return
		}
		c.Next()
	}
}

// ConcurrentLimiter returns middleware that enforces a maximum number of
// in-flight requests using a semaphore channel. Requests that arrive when
// the semaphore is full are rejected immediately with 429 and a static
// Retry-After: 1 header (RFC 6585 §4).
func ConcurrentLimiter(max int) gin.HandlerFunc {
	sem := make(chan struct{}, max)
	return func(c *gin.Context) {
		select {
		case sem <- struct{}{}:
		default:
			c.Header("Retry-After", "1")
			abort.Abort(c, errors.New("too many requests"), http.StatusTooManyRequests)
			return
		}
		defer func() { <-sem }()
		c.Next()
	}
}

// sanitizeLogValue strips newline and carriage-return characters from s to
// prevent log injection when env var values are written to structured logs.
func sanitizeLogValue(s string) string {
	s = strings.ReplaceAll(s, "\n", "")
	s = strings.ReplaceAll(s, "\r", "")
	return fmt.Sprintf("%q", s)
}

func getEnvFloat64(envVar string, defaultVal float64) float64 {
	result := defaultVal
	if v := os.Getenv(envVar); v != "" {
		if r, err := strconv.ParseFloat(v, 64); err == nil {
			if math.IsInf(r, 0) || math.IsNaN(r) {
				slog.Warn("Invalid float value for env var, using default", "var", envVar, "value", v)
			} else {
				result = r
			}
		} else {
			slog.Warn("Could not parse env var, using default", "var", envVar, "value", sanitizeLogValue(v))
		}
	}
	return result
}

func getEnvInt(envVar string, defaultVal int) int {
	result := defaultVal
	if v := os.Getenv(envVar); v != "" {
		if r, err := strconv.Atoi(v); err == nil {
			result = r
		} else {
			slog.Warn("Could not parse env var, using default", "var", envVar, "value", sanitizeLogValue(v))
		}
	}
	return result
}

func GetRateLimitConfig() (RateLimitConfig, error) {
	// Rate limits sized as last line of defense after HAProxy route limiting (60 conn/sec)
	// and DB connection pooling (SetMaxOpenConns=10). Defaults allow legitimate CI/CD bursts
	// (incident: 8 builds = 824 events/sec) while preventing resource exhaustion from
	// pathological traffic (incident: 24k concurrent external requests caused OOM).
	// Normal baseline: 0.085 req/s. Log routes use tighter limits (10s avg Loki latency).
	cfg := RateLimitConfig{
		OverallRPS:       100,
		LogRPS:           10,
		OverallBurst:     300,
		LogBurst:         30,
		MaxConcurrent:    200,
		MaxConcurrentLog: 20,
	}

	cfg.OverallRPS = getEnvFloat64(rateLimitOverallRPSEnvVar, cfg.OverallRPS)
	cfg.LogRPS = getEnvFloat64(rateLimitLogRPSEnvVar, cfg.LogRPS)
	cfg.OverallBurst = getEnvInt(rateLimitOverallBurstEnvVar, cfg.OverallBurst)
	cfg.LogBurst = getEnvInt(rateLimitLogBurstEnvVar, cfg.LogBurst)
	cfg.MaxConcurrent = getEnvInt(maxConcurrentRequestsEnvVar, cfg.MaxConcurrent)
	cfg.MaxConcurrentLog = getEnvInt(maxConcurrentLogRequestsEnvVar, cfg.MaxConcurrentLog)

	if cfg.OverallRPS <= 0 || cfg.LogRPS <= 0 ||
		cfg.OverallBurst <= 0 || cfg.LogBurst <= 0 ||
		cfg.MaxConcurrent <= 0 || cfg.MaxConcurrentLog <= 0 {

		slog.Error("Invalid rate limit configuration: all values must be positive")
		return RateLimitConfig{}, errors.New("invalid rate limit configuration: all values must be positive")
	}

	return cfg, nil
}
