package httpapi

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func resetRateLimitStore() {
	resetRateLimitStoreWithClock(time.Now)
}

func resetRateLimitStoreWithClock(now func() time.Time) {
	rateLimitStore = &ipRateLimiterStore{
		entries:    make(map[string]*rateLimiterEntry),
		maxEntries: defaultRateLimitMaxTrackedIPs,
		now:        now,
	}
}

func TestRateLimitMiddlewareAllowsInitialBurst(t *testing.T) {
	resetRateLimitStore()
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(rateLimitMiddleware())
	router.GET("/ping", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	for i := 0; i < defaultRateLimitBurst; i++ {
		res := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/ping", nil)
		req.RemoteAddr = "203.0.113.10:12345"
		router.ServeHTTP(res, req)

		if res.Code != http.StatusOK {
			t.Fatalf("request %d status = %d, want %d", i+1, res.Code, http.StatusOK)
		}
	}
}

func TestRateLimitMiddlewareRejectsAfterBurst(t *testing.T) {
	resetRateLimitStore()
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(rateLimitMiddleware())
	router.GET("/ping", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	for i := 0; i < defaultRateLimitBurst; i++ {
		res := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/ping", nil)
		req.RemoteAddr = "203.0.113.11:12345"
		router.ServeHTTP(res, req)
		if res.Code != http.StatusOK {
			t.Fatalf("burst request %d status = %d, want %d", i+1, res.Code, http.StatusOK)
		}
	}

	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.RemoteAddr = "203.0.113.11:12345"
	router.ServeHTTP(res, req)

	if res.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want %d", res.Code, http.StatusTooManyRequests)
	}
}

func TestRateLimitTokenRefillAfterIdle(t *testing.T) {
	now := time.Unix(1_000, 0)
	resetRateLimitStoreWithClock(func() time.Time { return now })
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(rateLimitMiddleware())
	router.GET("/ping", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	ip := "203.0.113.20:12345"

	// Exhaust burst
	for i := 0; i < defaultRateLimitBurst; i++ {
		res := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/ping", nil)
		req.RemoteAddr = ip
		router.ServeHTTP(res, req)
		if res.Code != http.StatusOK {
			t.Fatalf("burst request %d status = %d", i+1, res.Code)
		}
	}

	// Should be rate-limited now
	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.RemoteAddr = ip
	router.ServeHTTP(res, req)
	if res.Code != http.StatusTooManyRequests {
		t.Fatal("expected 429 after burst exhausted")
	}

	now = now.Add(20 * time.Millisecond)

	res = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.RemoteAddr = ip
	router.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("status after refill = %d, want %d", res.Code, http.StatusOK)
	}

	res = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.RemoteAddr = ip
	router.ServeHTTP(res, req)
	if res.Code != http.StatusTooManyRequests {
		t.Fatalf("second status after refill = %d, want %d", res.Code, http.StatusTooManyRequests)
	}
}

func TestRateLimitLargeTimeJumpDoesNotOverflow(t *testing.T) {
	now := time.Unix(2_000, 0)
	resetRateLimitStoreWithClock(func() time.Time { return now })
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(rateLimitMiddleware())
	router.GET("/ping", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	ip := "203.0.113.30:12345"
	for i := 0; i < defaultRateLimitBurst; i++ {
		res := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/ping", nil)
		req.RemoteAddr = ip
		router.ServeHTTP(res, req)
		if res.Code != http.StatusOK {
			t.Fatalf("burst request %d status = %d", i+1, res.Code)
		}
	}

	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.RemoteAddr = ip
	router.ServeHTTP(res, req)
	if res.Code != http.StatusTooManyRequests {
		t.Fatalf("status before idle = %d, want %d", res.Code, http.StatusTooManyRequests)
	}

	now = now.Add(24 * time.Hour)
	for i := 0; i < defaultRateLimitBurst; i++ {
		res := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/ping", nil)
		req.RemoteAddr = ip
		router.ServeHTTP(res, req)
		if res.Code != http.StatusOK {
			t.Fatalf("post-idle burst request %d status = %d", i+1, res.Code)
		}
	}

	res = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.RemoteAddr = ip
	router.ServeHTTP(res, req)
	if res.Code != http.StatusTooManyRequests {
		t.Fatalf("status after post-idle burst = %d, want %d", res.Code, http.StatusTooManyRequests)
	}
}

func TestRateLimitConcurrentRequestsDontOverflow(t *testing.T) {
	resetRateLimitStore()
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(rateLimitMiddleware())
	router.GET("/ping", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	const (
		goroutines = 20
		perRoutine = defaultRateLimitBurst / 2
	)
	var ok, limited atomic.Int32
	var wg sync.WaitGroup

	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < perRoutine; i++ {
				res := httptest.NewRecorder()
				req := httptest.NewRequest(http.MethodGet, "/ping", nil)
				req.RemoteAddr = "203.0.113.40:12345"
				router.ServeHTTP(res, req)
				switch res.Code {
				case http.StatusOK:
					ok.Add(1)
				case http.StatusTooManyRequests:
					limited.Add(1)
				}
			}
		}()
	}
	wg.Wait()

	passed := int(ok.Load())
	if passed > defaultRateLimitBurst+10 {
		t.Fatalf("%d requests passed (burst=%d), too many for concurrent access", passed, defaultRateLimitBurst)
	}
	t.Logf("passed=%d limited=%d total=%d", ok.Load(), limited.Load(), goroutines*perRoutine)
}

func TestRateLimitUsesRemoteAddr(t *testing.T) {
	resetRateLimitStore()
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(rateLimitMiddleware())
	router.GET("/ping", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	// No X-Forwarded-For, only RemoteAddr
	for i := 0; i < defaultRateLimitBurst; i++ {
		res := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/ping", nil)
		req.RemoteAddr = "10.0.0.1:9999"
		router.ServeHTTP(res, req)
		if res.Code != http.StatusOK {
			t.Fatalf("RemoteAddr request %d status = %d", i+1, res.Code)
		}
	}

	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.RemoteAddr = "10.0.0.1:9999"
	router.ServeHTTP(res, req)
	if res.Code != http.StatusTooManyRequests {
		t.Fatal("expected 429 from RemoteAddr-based limiting")
	}
}

func TestClientIPFallsBackToRawRemoteAddrWhenGinCannotParse(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/ping", nil)
	c.Request.RemoteAddr = "not-a-host-port"

	if got := clientIP(c); got != "not-a-host-port" {
		t.Fatalf("clientIP fallback = %q, want raw RemoteAddr", got)
	}
}

func TestRateLimitEvictsOldestOnOverflow(t *testing.T) {
	now := time.Unix(3_000, 0)
	resetRateLimitStoreWithClock(func() time.Time { return now })
	gin.SetMode(gin.TestMode)

	rateLimitStore.mu.Lock()
	rateLimitStore.maxEntries = 3
	rateLimitStore.mu.Unlock()

	router := gin.New()
	router.Use(rateLimitMiddleware())
	router.GET("/ping", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	// Fill up to max with 3 IPs
	makeReq := func(ip string) int {
		res := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/ping", nil)
		req.RemoteAddr = ip
		router.ServeHTTP(res, req)
		now = now.Add(time.Second)
		return res.Code
	}

	makeReq("10.0.0.1:1")
	makeReq("10.0.0.2:1")
	makeReq("10.0.0.3:1") // pool full

	// 4th IP should evict oldest
	code := makeReq("10.0.0.4:1")
	if code != http.StatusOK {
		t.Fatalf("4th IP status=%d, should be allowed", code)
	}

	// Old IP1 should be evicted, so IP1 gets new entry with full burst
	rateLimitStore.mu.Lock()
	_, existsOld := rateLimitStore.entries["10.0.0.1"]
	_, existsNew := rateLimitStore.entries["10.0.0.4"]
	rateLimitStore.mu.Unlock()

	if existsOld {
		t.Fatal("oldest IP should have been evicted on overflow")
	}
	if !existsNew {
		t.Fatal("new IP should exist after eviction")
	}
}

func TestRateLimitCleansStaleEntries(t *testing.T) {
	now := time.Unix(4_000, 0)
	resetRateLimitStoreWithClock(func() time.Time { return now })
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(rateLimitMiddleware())
	router.GET("/ping", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.RemoteAddr = "203.0.113.60:12345"
	router.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("first request status = %d", res.Code)
	}

	now = now.Add(rateLimitEntryTTL + rateLimitCleanupEvery + time.Second)

	res = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.RemoteAddr = "203.0.113.61:12345"
	router.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("second request status = %d", res.Code)
	}

	rateLimitStore.mu.Lock()
	_, staleExists := rateLimitStore.entries["203.0.113.60"]
	_, freshExists := rateLimitStore.entries["203.0.113.61"]
	rateLimitStore.mu.Unlock()

	if staleExists {
		t.Fatal("stale entry should have been cleaned")
	}
	if !freshExists {
		t.Fatal("fresh entry should exist")
	}
}
