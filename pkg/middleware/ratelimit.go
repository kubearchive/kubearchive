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
				result = defaultVal
			}
			result = r
		} else {
			slog.Warn("Could not parse env var, using default", "var", envVar, "value", sanitizeLogValue(v))
			result = defaultVal
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
			result = defaultVal
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
