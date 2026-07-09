// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestUserRateLimiter_PerUserIsolation(t *testing.T) {
	// Each user gets their own bucket of size burst=2.
	// Sending burst*2 requests per user should allow exactly burst per user
	// and reject the rest — regardless of what other users are doing.
	const burst = 2
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/log", UserRateLimiter(float64(burst), burst), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	sendN := func(n int, header, value string) (ok, limited int) {
		for range n {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/log", nil)
			if header != "" {
				req.Header.Set(header, value)
			}
			router.ServeHTTP(w, req)
			switch w.Code {
			case http.StatusOK:
				ok++
			case http.StatusTooManyRequests:
				limited++
			default:
				t.Errorf("unexpected status %d", w.Code)
			}
		}
		return
	}

	// User A exhausts their budget.
	okA, limitedA := sendN(burst*2, "Impersonate-User", "user-a")
	if okA != burst {
		t.Errorf("user-a ok = %d, want %d", okA, burst)
	}
	if limitedA != burst {
		t.Errorf("user-a limited = %d, want %d", limitedA, burst)
	}

	// User B has an independent budget — none of A's requests should affect B.
	okB, limitedB := sendN(burst, "Impersonate-User", "user-b")
	if okB != burst {
		t.Errorf("user-b ok = %d, want %d (A's exhausted budget must not affect B)", okB, burst)
	}
	if limitedB != 0 {
		t.Errorf("user-b limited = %d, want 0", limitedB)
	}
}

func TestUserRateLimiter_KeyPrecedence(t *testing.T) {
	// Verify that the key falls through: Impersonate-User > Bearer token > IP.
	// We use burst=1 so a second identical-key request is always rejected, and
	// a different-key request is always accepted.
	const burst = 1

	tests := []struct {
		name        string
		impersonate string
		auth        string
		// Two requests with the same headers: first must be 200, second must be 429.
	}{
		{name: "impersonate header takes priority over bearer", impersonate: "alice", auth: "Bearer tok-alice"},
		{name: "bearer token used when no impersonate header", impersonate: "", auth: "Bearer tok-bob"},
		{name: "full auth header value used when not bearer prefix", impersonate: "", auth: "Basic dXNlcjpwYXNz"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset by creating a fresh router for each sub-test.
			r := gin.New()
			r.GET("/log", UserRateLimiter(float64(burst), burst), func(c *gin.Context) {
				c.Status(http.StatusOK)
			})
			doReq := func() int {
				w := httptest.NewRecorder()
				req := httptest.NewRequest(http.MethodGet, "/log", nil)
				if tt.impersonate != "" {
					req.Header.Set("Impersonate-User", tt.impersonate)
				}
				if tt.auth != "" {
					req.Header.Set("Authorization", tt.auth)
				}
				r.ServeHTTP(w, req)
				return w.Code
			}
			if got := doReq(); got != http.StatusOK {
				t.Errorf("%s: first request = %d, want 200", tt.name, got)
			}
			if got := doReq(); got != http.StatusTooManyRequests {
				t.Errorf("%s: second request = %d, want 429", tt.name, got)
			}
		})
	}

	// Confirm Impersonate-User beats Authorization: same token but different
	// impersonate values should give each their own bucket.
	r2 := gin.New()
	r2.GET("/log", UserRateLimiter(float64(burst), burst), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	sendWith := func(impersonate, auth string) int {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/log", nil)
		if impersonate != "" {
			req.Header.Set("Impersonate-User", impersonate)
		}
		if auth != "" {
			req.Header.Set("Authorization", auth)
		}
		r2.ServeHTTP(w, req)
		return w.Code
	}

	if got := sendWith("alice", "Bearer shared-token"); got != http.StatusOK {
		t.Errorf("alice first = %d, want 200", got)
	}
	// alice's bucket is now empty; bob uses same bearer but different impersonate — fresh bucket.
	if got := sendWith("bob", "Bearer shared-token"); got != http.StatusOK {
		t.Errorf("bob first (same bearer, different impersonate) = %d, want 200", got)
	}
	// alice's second request must be rejected.
	if got := sendWith("alice", "Bearer shared-token"); got != http.StatusTooManyRequests {
		t.Errorf("alice second = %d, want 429", got)
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
				OverallRPS:   100,
				LogRPS:       10,
				OverallBurst: 300,
				LogBurst:     30,
			},
		},
		{
			name: "override OverallRPS only",
			envVars: map[string]string{
				"API_RATE_LIMIT_OVERALL_RPS": "200",
			},
			want: RateLimitConfig{
				OverallRPS:   200,
				LogRPS:       10,
				OverallBurst: 300,
				LogBurst:     30,
			},
		},
		{
			name: "override LogRPS only",
			envVars: map[string]string{
				"API_RATE_LIMIT_LOG_RPS": "20",
			},
			want: RateLimitConfig{
				OverallRPS:   100,
				LogRPS:       20,
				OverallBurst: 300,
				LogBurst:     30,
			},
		},
		{
			name: "override OverallBurst only",
			envVars: map[string]string{
				"API_RATE_LIMIT_OVERALL_BURST": "500",
			},
			want: RateLimitConfig{
				OverallRPS:   100,
				LogRPS:       10,
				OverallBurst: 500,
				LogBurst:     30,
			},
		},
		{
			name: "override LogBurst only",
			envVars: map[string]string{
				"API_RATE_LIMIT_LOG_BURST": "50",
			},
			want: RateLimitConfig{
				OverallRPS:   100,
				LogRPS:       10,
				OverallBurst: 300,
				LogBurst:     50,
			},
		},
		{
			name: "all overridden",
			envVars: map[string]string{
				"API_RATE_LIMIT_OVERALL_RPS":   "250",
				"API_RATE_LIMIT_LOG_RPS":       "25",
				"API_RATE_LIMIT_OVERALL_BURST": "600",
				"API_RATE_LIMIT_LOG_BURST":     "60",
			},
			want: RateLimitConfig{
				OverallRPS:   250,
				LogRPS:       25,
				OverallBurst: 600,
				LogBurst:     60,
			},
		},
		{
			name: "float RPS values",
			envVars: map[string]string{
				"API_RATE_LIMIT_OVERALL_RPS": "0.5",
				"API_RATE_LIMIT_LOG_RPS":     "0.25",
			},
			want: RateLimitConfig{
				OverallRPS:   0.5,
				LogRPS:       0.25,
				OverallBurst: 300,
				LogBurst:     30,
			},
		},
		{
			name: "invalid OverallRPS uses default",
			envVars: map[string]string{
				"API_RATE_LIMIT_OVERALL_RPS": "not-a-number",
			},
			want: RateLimitConfig{
				OverallRPS:   100,
				LogRPS:       10,
				OverallBurst: 300,
				LogBurst:     30,
			},
		},
		{
			name: "invalid LogRPS uses default",
			envVars: map[string]string{
				"API_RATE_LIMIT_LOG_RPS": "invalid",
			},
			want: RateLimitConfig{
				OverallRPS:   100,
				LogRPS:       10,
				OverallBurst: 300,
				LogBurst:     30,
			},
		},
		{
			name: "invalid OverallBurst uses default",
			envVars: map[string]string{
				"API_RATE_LIMIT_OVERALL_BURST": "xyz",
			},
			want: RateLimitConfig{
				OverallRPS:   100,
				LogRPS:       10,
				OverallBurst: 300,
				LogBurst:     30,
			},
		},
		{
			name: "invalid LogBurst uses default",
			envVars: map[string]string{
				"API_RATE_LIMIT_LOG_BURST": "abc",
			},
			want: RateLimitConfig{
				OverallRPS:   100,
				LogRPS:       10,
				OverallBurst: 300,
				LogBurst:     30,
			},
		},
		{
			name: "mixed valid and invalid values",
			envVars: map[string]string{
				"API_RATE_LIMIT_OVERALL_RPS":   "150",
				"API_RATE_LIMIT_LOG_RPS":       "invalid",
				"API_RATE_LIMIT_OVERALL_BURST": "450",
				"API_RATE_LIMIT_LOG_BURST":     "not-an-int",
			},
			want: RateLimitConfig{
				OverallRPS:   150,
				LogRPS:       10,
				OverallBurst: 450,
				LogBurst:     30,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.envVars {
				t.Setenv(k, v)
			}

			got, err := GetRateLimitConfig()
			if err != nil {
				t.Fatalf("GetRateLimitConfig() unexpected error = %v", err)
			}

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
		})
	}
}

func TestGetRateLimitConfig_InvalidValues(t *testing.T) {
	tests := []struct {
		name    string
		envVars map[string]string
		wantErr bool
	}{
		{
			name: "zero OverallRPS",
			envVars: map[string]string{
				"API_RATE_LIMIT_OVERALL_RPS": "0",
			},
			wantErr: true,
		},
		{
			name: "negative OverallRPS",
			envVars: map[string]string{
				"API_RATE_LIMIT_OVERALL_RPS": "-10",
			},
			wantErr: true,
		},
		{
			name: "zero LogRPS",
			envVars: map[string]string{
				"API_RATE_LIMIT_LOG_RPS": "0",
			},
			wantErr: true,
		},
		{
			name: "negative LogRPS",
			envVars: map[string]string{
				"API_RATE_LIMIT_LOG_RPS": "-5",
			},
			wantErr: true,
		},
		{
			name: "zero OverallBurst",
			envVars: map[string]string{
				"API_RATE_LIMIT_OVERALL_BURST": "0",
			},
			wantErr: true,
		},
		{
			name: "negative OverallBurst",
			envVars: map[string]string{
				"API_RATE_LIMIT_OVERALL_BURST": "-100",
			},
			wantErr: true,
		},
		{
			name: "zero LogBurst",
			envVars: map[string]string{
				"API_RATE_LIMIT_LOG_BURST": "0",
			},
			wantErr: true,
		},
		{
			name: "negative LogBurst",
			envVars: map[string]string{
				"API_RATE_LIMIT_LOG_BURST": "-50",
			},
			wantErr: true,
		},
		{
			name: "multiple invalid values",
			envVars: map[string]string{
				"API_RATE_LIMIT_OVERALL_RPS": "-100",
				"API_RATE_LIMIT_LOG_RPS":     "0",
			},
			wantErr: true,
		},
		{
			name: "all values zero",
			envVars: map[string]string{
				"API_RATE_LIMIT_OVERALL_RPS":   "0",
				"API_RATE_LIMIT_LOG_RPS":       "0",
				"API_RATE_LIMIT_OVERALL_BURST": "0",
				"API_RATE_LIMIT_LOG_BURST":     "0",
			},
			wantErr: true,
		},
		{
			name: "all values negative",
			envVars: map[string]string{
				"API_RATE_LIMIT_OVERALL_RPS":   "-1",
				"API_RATE_LIMIT_LOG_RPS":       "-1",
				"API_RATE_LIMIT_OVERALL_BURST": "-1",
				"API_RATE_LIMIT_LOG_BURST":     "-1",
			},
			wantErr: true,
		},
		{
			name: "valid config returns no error",
			envVars: map[string]string{
				"API_RATE_LIMIT_OVERALL_RPS":   "200",
				"API_RATE_LIMIT_LOG_RPS":       "20",
				"API_RATE_LIMIT_OVERALL_BURST": "600",
				"API_RATE_LIMIT_LOG_BURST":     "60",
			},
			wantErr: false,
		},
		{
			name:    "defaults return no error",
			envVars: map[string]string{},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.envVars {
				t.Setenv(k, v)
			}

			_, err := GetRateLimitConfig()

			if (err != nil) != tt.wantErr {
				t.Errorf("GetRateLimitConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
