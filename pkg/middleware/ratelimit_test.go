// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package middleware

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
)

func newTestRouter(handlers ...gin.HandlerFunc) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/test", append(handlers, func(c *gin.Context) {
		c.Status(http.StatusOK)
	})...)
	return r
}

func TestRateLimiter(t *testing.T) {
	const burst = 3
	r := newTestRouter(RateLimiter(float64(burst), burst))
	var ok, limited int
	for range burst * 2 {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		r.ServeHTTP(w, req)
		switch w.Code {
		case http.StatusOK:
			ok++
		case http.StatusTooManyRequests:
			limited++
		default:
			t.Errorf("unexpected status %d", w.Code)
		}
	}

	if ok != burst {
		t.Errorf("RateLimiter ok = %d, want %d", ok, burst)
	}
	if limited != burst {
		t.Errorf("RateLimiter limited = %d, want %d", limited, burst)
	}
}

func TestConcurrentLimiter(t *testing.T) {
	const max = 3
	const total = max * 2

	// started is closed once all goroutines have begun their ServeHTTP call.
	// release is closed to let the held goroutines finish.
	started := make(chan struct{})
	release := make(chan struct{})
	var startOnce sync.Once

	// Track how many goroutines have started.
	var startCount atomic.Int32

	r := newTestRouter(
		// Record that this goroutine has started before the semaphore check.
		func(c *gin.Context) {
			if startCount.Add(1) == total {
				startOnce.Do(func() { close(started) })
			}
			c.Next()
		},
		ConcurrentLimiter(max),
		func(c *gin.Context) {
			<-release
		},
	)

	var ok, limited int32
	var wg sync.WaitGroup
	wg.Add(total)

	for range total {
		go func() {
			defer wg.Done()
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			r.ServeHTTP(w, req)
			switch w.Code {
			case http.StatusOK:
				atomic.AddInt32(&ok, 1)
			case http.StatusTooManyRequests:
				atomic.AddInt32(&limited, 1)
			}
		}()
	}

	// Wait until all goroutines have entered ServeHTTP, then release the held ones.
	<-started
	close(release)
	wg.Wait()

	if int(ok) != max {
		t.Errorf("ConcurrentLimiter ok = %d, want %d", ok, max)
	}
	if int(limited) != total-max {
		t.Errorf("ConcurrentLimiter limited = %d, want %d", limited, total-max)
	}
}

func TestGetRateLimitConfig(t *testing.T) {
	tests := []struct {
		name    string
		envVars map[string]string
		want    RateLimitConfig
	}{
		{
			name:    "all defaults",
			envVars: map[string]string{},
			want: RateLimitConfig{
				OverallRPS:       100,
				LogRPS:           10,
				OverallBurst:     300,
				LogBurst:         30,
				MaxConcurrent:    200,
				MaxConcurrentLog: 20,
			},
		},
		{
			name: "override OverallRPS only",
			envVars: map[string]string{
				"API_RATE_LIMIT_OVERALL_RPS": "200",
			},
			want: RateLimitConfig{
				OverallRPS:       200,
				LogRPS:           10,
				OverallBurst:     300,
				LogBurst:         30,
				MaxConcurrent:    200,
				MaxConcurrentLog: 20,
			},
		},
		{
			name: "override LogRPS only",
			envVars: map[string]string{
				"API_RATE_LIMIT_LOG_RPS": "20",
			},
			want: RateLimitConfig{
				OverallRPS:       100,
				LogRPS:           20,
				OverallBurst:     300,
				LogBurst:         30,
				MaxConcurrent:    200,
				MaxConcurrentLog: 20,
			},
		},
		{
			name: "override OverallBurst only",
			envVars: map[string]string{
				"API_RATE_LIMIT_OVERALL_BURST": "500",
			},
			want: RateLimitConfig{
				OverallRPS:       100,
				LogRPS:           10,
				OverallBurst:     500,
				LogBurst:         30,
				MaxConcurrent:    200,
				MaxConcurrentLog: 20,
			},
		},
		{
			name: "override LogBurst only",
			envVars: map[string]string{
				"API_RATE_LIMIT_LOG_BURST": "50",
			},
			want: RateLimitConfig{
				OverallRPS:       100,
				LogRPS:           10,
				OverallBurst:     300,
				LogBurst:         50,
				MaxConcurrent:    200,
				MaxConcurrentLog: 20,
			},
		},
		{
			name: "override MaxConcurrent only",
			envVars: map[string]string{
				"API_MAX_CONCURRENT_REQUESTS": "400",
			},
			want: RateLimitConfig{
				OverallRPS:       100,
				LogRPS:           10,
				OverallBurst:     300,
				LogBurst:         30,
				MaxConcurrent:    400,
				MaxConcurrentLog: 20,
			},
		},
		{
			name: "override MaxConcurrentLog only",
			envVars: map[string]string{
				"API_MAX_CONCURRENT_LOG_REQUESTS": "40",
			},
			want: RateLimitConfig{
				OverallRPS:       100,
				LogRPS:           10,
				OverallBurst:     300,
				LogBurst:         30,
				MaxConcurrent:    200,
				MaxConcurrentLog: 40,
			},
		},
		{
			name: "all overridden",
			envVars: map[string]string{
				"API_RATE_LIMIT_OVERALL_RPS":      "250",
				"API_RATE_LIMIT_LOG_RPS":          "25",
				"API_RATE_LIMIT_OVERALL_BURST":    "600",
				"API_RATE_LIMIT_LOG_BURST":        "60",
				"API_MAX_CONCURRENT_REQUESTS":     "500",
				"API_MAX_CONCURRENT_LOG_REQUESTS": "50",
			},
			want: RateLimitConfig{
				OverallRPS:       250,
				LogRPS:           25,
				OverallBurst:     600,
				LogBurst:         60,
				MaxConcurrent:    500,
				MaxConcurrentLog: 50,
			},
		},
		{
			name: "float RPS values",
			envVars: map[string]string{
				"API_RATE_LIMIT_OVERALL_RPS": "0.5",
				"API_RATE_LIMIT_LOG_RPS":     "0.25",
			},
			want: RateLimitConfig{
				OverallRPS:       0.5,
				LogRPS:           0.25,
				OverallBurst:     300,
				LogBurst:         30,
				MaxConcurrent:    200,
				MaxConcurrentLog: 20,
			},
		},
		{
			name: "invalid OverallRPS uses default",
			envVars: map[string]string{
				"API_RATE_LIMIT_OVERALL_RPS": "not-a-number",
			},
			want: RateLimitConfig{
				OverallRPS:       100,
				LogRPS:           10,
				OverallBurst:     300,
				LogBurst:         30,
				MaxConcurrent:    200,
				MaxConcurrentLog: 20,
			},
		},
		{
			name: "invalid LogRPS uses default",
			envVars: map[string]string{
				"API_RATE_LIMIT_LOG_RPS": "invalid",
			},
			want: RateLimitConfig{
				OverallRPS:       100,
				LogRPS:           10,
				OverallBurst:     300,
				LogBurst:         30,
				MaxConcurrent:    200,
				MaxConcurrentLog: 20,
			},
		},
		{
			name: "invalid OverallBurst uses default",
			envVars: map[string]string{
				"API_RATE_LIMIT_OVERALL_BURST": "xyz",
			},
			want: RateLimitConfig{
				OverallRPS:       100,
				LogRPS:           10,
				OverallBurst:     300,
				LogBurst:         30,
				MaxConcurrent:    200,
				MaxConcurrentLog: 20,
			},
		},
		{
			name: "invalid LogBurst uses default",
			envVars: map[string]string{
				"API_RATE_LIMIT_LOG_BURST": "abc",
			},
			want: RateLimitConfig{
				OverallRPS:       100,
				LogRPS:           10,
				OverallBurst:     300,
				LogBurst:         30,
				MaxConcurrent:    200,
				MaxConcurrentLog: 20,
			},
		},
		{
			name: "invalid MaxConcurrent uses default",
			envVars: map[string]string{
				"API_MAX_CONCURRENT_REQUESTS": "bad-value",
			},
			want: RateLimitConfig{
				OverallRPS:       100,
				LogRPS:           10,
				OverallBurst:     300,
				LogBurst:         30,
				MaxConcurrent:    200,
				MaxConcurrentLog: 20,
			},
		},
		{
			name: "invalid MaxConcurrentLog uses default",
			envVars: map[string]string{
				"API_MAX_CONCURRENT_LOG_REQUESTS": "wrong",
			},
			want: RateLimitConfig{
				OverallRPS:       100,
				LogRPS:           10,
				OverallBurst:     300,
				LogBurst:         30,
				MaxConcurrent:    200,
				MaxConcurrentLog: 20,
			},
		},
		{
			name: "mixed valid and invalid values",
			envVars: map[string]string{
				"API_RATE_LIMIT_OVERALL_RPS":      "150",
				"API_RATE_LIMIT_LOG_RPS":          "invalid",
				"API_RATE_LIMIT_OVERALL_BURST":    "450",
				"API_RATE_LIMIT_LOG_BURST":        "not-an-int",
				"API_MAX_CONCURRENT_REQUESTS":     "300",
				"API_MAX_CONCURRENT_LOG_REQUESTS": "bad",
			},
			want: RateLimitConfig{
				OverallRPS:       150,
				LogRPS:           10,
				OverallBurst:     450,
				LogBurst:         30,
				MaxConcurrent:    300,
				MaxConcurrentLog: 20,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.envVars {
				t.Setenv(k, v)
			}

			got, _ := GetRateLimitConfig()

			if got.OverallRPS != tt.want.OverallRPS {
				t.Errorf("GetRateLimitConfig() OverallRPS = %v, want %v", got.OverallRPS, tt.want.OverallRPS)
			}
			if got.LogRPS != tt.want.LogRPS {
				t.Errorf("GetRateLimitConfig() LogRPS = %v, want %v", got.LogRPS, tt.want.LogRPS)
			}
			if got.OverallBurst != tt.want.OverallBurst {
				t.Errorf("GetRateLimitConfig() OverallBurst = %v, want %v", got.OverallBurst, tt.want.OverallBurst)
			}
			if got.LogBurst != tt.want.LogBurst {
				t.Errorf("GetRateLimitConfig() LogBurst = %v, want %v", got.LogBurst, tt.want.LogBurst)
			}
			if got.MaxConcurrent != tt.want.MaxConcurrent {
				t.Errorf("GetRateLimitConfig() MaxConcurrent = %v, want %v", got.MaxConcurrent, tt.want.MaxConcurrent)
			}
			if got.MaxConcurrentLog != tt.want.MaxConcurrentLog {
				t.Errorf("GetRateLimitConfig() MaxConcurrentLog = %v, want %v", got.MaxConcurrentLog, tt.want.MaxConcurrentLog)
			}
		})
	}
}
