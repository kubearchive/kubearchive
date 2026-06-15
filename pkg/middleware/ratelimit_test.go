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
	for i := 0; i < burst*2; i++ {
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

	for i := 0; i < total; i++ {
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
