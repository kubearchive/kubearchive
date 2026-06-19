// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package middleware

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"

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

var rateLimiter gin.HandlerFunc = nil
var concurrentLimiter gin.HandlerFunc = nil

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
	if rateLimiter == nil {
		limiter := rate.NewLimiter(rate.Limit(rps), burst)
		rateLimiter = func(c *gin.Context) {
			if !limiter.Allow() {
				abort.Abort(c, errors.New("too many requests"), http.StatusTooManyRequests)
				return
			}
			c.Next()
		}
	}
	return rateLimiter
}

// ConcurrentLimiter returns middleware that enforces a maximum number of
// in-flight requests using a semaphore channel. Requests that arrive when
// the semaphore is full are rejected immediately with 429.
func ConcurrentLimiter(max int) gin.HandlerFunc {
	if concurrentLimiter == nil {
		sem := make(chan struct{}, max)
		concurrentLimiter = func(c *gin.Context) {
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
	return concurrentLimiter
}

func getEnvFloat64(envVar string, defaultVal float64) (float64, error) {
	var err error
	var result float64 = defaultVal
	if v := os.Getenv(envVar); v != "" {
		if r, err := strconv.ParseFloat(v, 64); err == nil {
			result = r
		} else {
			slog.Warn("Could not parse env var, using default", "var", defaultVal, "value", fmt.Sprintf("%q", v))
			result = defaultVal
		}
	}
	return result, err
}

func getEnvInt(envVar string, defaultVal int) (int, error) {
	var err error
	var result int = defaultVal
	if v := os.Getenv(envVar); v != "" {
		if r, err := strconv.Atoi(v); err == nil {
			result = r
		} else {
			slog.Warn("Could not parse env var, using default", "var", defaultVal, "value", fmt.Sprintf("%q", v))
			result = defaultVal
		}
	}
	return result, err
}

func GetRateLimitConfig() RateLimitConfig {
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

	cfg.OverallRPS, _ = getEnvFloat64(rateLimitOverallRPSEnvVar, cfg.OverallRPS)
	cfg.LogRPS, _ = getEnvFloat64(rateLimitLogRPSEnvVar, cfg.LogRPS)
	cfg.OverallBurst, _ = getEnvInt(rateLimitOverallBurstEnvVar, cfg.OverallBurst)
	cfg.LogBurst, _ = getEnvInt(rateLimitLogBurstEnvVar, cfg.LogBurst)
	cfg.MaxConcurrent, _ = getEnvInt(maxConcurrentRequestsEnvVar, cfg.MaxConcurrent)
	cfg.MaxConcurrentLog, _ = getEnvInt(maxConcurrentLogRequestsEnvVar, cfg.MaxConcurrentLog)

	if cfg.OverallRPS <= 0 || cfg.LogRPS <= 0 ||
		cfg.OverallBurst <= 0 || cfg.LogBurst <= 0 ||
		cfg.MaxConcurrent <= 0 || cfg.MaxConcurrentLog <= 0 {
		slog.Error("Invalid rate limit configuration: all values must be positive")
		os.Exit(1)
	}

	return cfg
}
