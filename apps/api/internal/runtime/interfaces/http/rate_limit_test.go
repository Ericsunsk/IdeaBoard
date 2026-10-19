package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"ideaboard/api/internal/platform/config"
	"ideaboard/api/internal/platform/settings"

	"github.com/gin-gonic/gin"
)

type mutableRateLimitSettingsProvider struct {
	mu      sync.Mutex
	setting settings.RateLimitSetting
}

type deadlineObservingRateLimitProvider struct {
	deadline time.Time
}

func (p *deadlineObservingRateLimitProvider) GetRateLimit(ctx context.Context) (settings.RateLimitSetting, error) {
	p.deadline, _ = ctx.Deadline()
	return settings.DefaultRateLimitSetting(), nil
}

func (p *mutableRateLimitSettingsProvider) GetRateLimit(_ context.Context) (settings.RateLimitSetting, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.setting, nil
}

func (p *mutableRateLimitSettingsProvider) set(setting settings.RateLimitSetting) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.setting = setting
}

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
	router.Use(rateLimitMiddleware(nil))
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

func TestRateLimitSettingsLookupUsesBoundedContext(t *testing.T) {
	provider := &deadlineObservingRateLimitProvider{}
	started := time.Now()
	resolveRateLimitPolicy(context.Background(), provider)

	if provider.deadline.IsZero() {
		t.Fatal("rate-limit settings lookup should receive a deadline")
	}
	if provider.deadline.Before(started) || provider.deadline.After(started.Add(rateLimitSettingsTimeout+time.Second)) {
		t.Fatalf("deadline = %v, want within %v from start %v", provider.deadline, rateLimitSettingsTimeout, started)
	}
}

func TestRateLimitMiddlewareRejectsAfterBurst(t *testing.T) {
	resetRateLimitStore()
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(rateLimitMiddleware(nil))
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

func TestRateLimitMiddlewareAppliesRuntimeSettingsChanges(t *testing.T) {
	resetRateLimitStore()
	gin.SetMode(gin.TestMode)

	provider := &mutableRateLimitSettingsProvider{}
	provider.set(settings.RateLimitSetting{WindowSeconds: 60, MaxRequests: 1})

	router := gin.New()
	router.Use(rateLimitMiddleware(provider))
	router.GET("/ping", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.RemoteAddr = "203.0.113.12:12345"
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("first status = %d, want %d", res.Code, http.StatusOK)
	}

	req = httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.RemoteAddr = "203.0.113.12:12345"
	res = httptest.NewRecorder()
	router.ServeHTTP(res, req)
	if res.Code != http.StatusTooManyRequests {
		t.Fatalf("second status = %d, want %d", res.Code, http.StatusTooManyRequests)
	}

	provider.set(settings.RateLimitSetting{WindowSeconds: 60, MaxRequests: 2})

	for i := 0; i < 2; i++ {
		req = httptest.NewRequest(http.MethodGet, "/ping", nil)
		req.RemoteAddr = "203.0.113.12:12345"
		res = httptest.NewRecorder()
		router.ServeHTTP(res, req)
		if res.Code != http.StatusOK {
			t.Fatalf("post-update request %d status = %d, want %d", i+1, res.Code, http.StatusOK)
		}
	}
}

func TestRateLimitTokenRefillAfterIdle(t *testing.T) {
	now := time.Unix(1_000, 0)
	resetRateLimitStoreWithClock(func() time.Time { return now })
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(rateLimitMiddleware(nil))
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
	router.Use(rateLimitMiddleware(nil))
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
	now := time.Unix(3_000, 0)
	resetRateLimitStoreWithClock(func() time.Time { return now })
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(rateLimitMiddleware(nil))
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
	if passed > defaultRateLimitBurst {
		t.Fatalf("%d requests passed (burst=%d), too many for concurrent access", passed, defaultRateLimitBurst)
	}
	t.Logf("passed=%d limited=%d total=%d", ok.Load(), limited.Load(), goroutines*perRoutine)
}

func TestRateLimitUsesRemoteAddr(t *testing.T) {
	resetRateLimitStore()
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(rateLimitMiddleware(nil))
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

func TestClientIPResolvesRealIPBehindDockerBridgeGateway(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	if err := router.SetTrustedProxies((config.AppConfig{Env: "production"}).TrustedProxies()); err != nil {
		t.Fatalf("SetTrustedProxies(%v) error = %v", (config.AppConfig{Env: "production"}).TrustedProxies(), err)
	}
	router.GET("/ip", func(c *gin.Context) {
		c.String(http.StatusOK, clientIP(c))
	})

	// Under Docker bridge port mapping the container sees RemoteAddr as
	// the bridge gateway IP; the host Nginx has already restored the real
	// client IP into X-Forwarded-For.
	cases := map[string]struct {
		remoteAddr string
		xff        string
		want       string
	}{
		"single XFF through 172.16/12 gateway":  {remoteAddr: "172.18.0.1:41234", xff: "203.0.113.99", want: "203.0.113.99"},
		"proxy-appended XFF through gateway":    {remoteAddr: "172.18.0.1:41234", xff: "203.0.113.99, 172.18.0.1", want: "203.0.113.99"},
		"single XFF through 10/8 gateway":       {remoteAddr: "10.0.0.1:41234", xff: "198.51.100.23", want: "198.51.100.23"},
		"single XFF through 192.168/16 gateway": {remoteAddr: "192.168.65.1:41234", xff: "198.51.100.24", want: "198.51.100.24"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			res := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/ip", nil)
			req.RemoteAddr = tc.remoteAddr
			req.Header.Set("X-Forwarded-For", tc.xff)
			router.ServeHTTP(res, req)
			if res.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", res.Code, http.StatusOK)
			}
			if got := res.Body.String(); got != tc.want {
				t.Fatalf("clientIP = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestClientIPIgnoresForgedXFFFromUntrustedDirectClient(t *testing.T) {
	// Security boundary note: with private ranges trusted, anyone able to
	// reach 6000 directly could forge X-Forwarded-For and evade per-IP
	// rate limiting. This trust holds only because compose publishes 6000
	// on 127.0.0.1 behind the same-origin host Nginx gateway. A direct
	// public client presents an untrusted RemoteAddr, and Gin then ignores
	// XFF entirely and returns the RemoteAddr host.
	gin.SetMode(gin.TestMode)

	router := gin.New()
	if err := router.SetTrustedProxies((config.AppConfig{Env: "production"}).TrustedProxies()); err != nil {
		t.Fatalf("SetTrustedProxies(%v) error = %v", (config.AppConfig{Env: "production"}).TrustedProxies(), err)
	}
	router.GET("/ip", func(c *gin.Context) {
		c.String(http.StatusOK, clientIP(c))
	})

	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ip", nil)
	req.RemoteAddr = "198.51.100.7:54321"
	req.Header.Set("X-Forwarded-For", "203.0.113.99")
	router.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", res.Code, http.StatusOK)
	}
	if got := res.Body.String(); got != "198.51.100.7" {
		t.Fatalf("clientIP = %q, want RemoteAddr host (forged XFF must be ignored)", got)
	}
}

func TestRateLimitDistinguishesClientsBehindSharedGateway(t *testing.T) {
	// Regression for the gateway single-bucket collapse: without gateway
	// trust every client behind the Docker bridge shares one gateway-IP
	// bucket; with trust each real client IP gets its own bucket.
	resetRateLimitStore()
	gin.SetMode(gin.TestMode)

	router := gin.New()
	if err := router.SetTrustedProxies((config.AppConfig{Env: "production"}).TrustedProxies()); err != nil {
		t.Fatalf("SetTrustedProxies(%v) error = %v", (config.AppConfig{Env: "production"}).TrustedProxies(), err)
	}
	router.Use(rateLimitMiddleware(nil))
	router.GET("/ping", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	doReq := func(realIP string) int {
		res := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/ping", nil)
		req.RemoteAddr = "172.18.0.1:41234"
		req.Header.Set("X-Forwarded-For", realIP)
		router.ServeHTTP(res, req)
		return res.Code
	}

	// Exhaust client A's burst through the shared gateway.
	for i := 0; i < defaultRateLimitBurst; i++ {
		if code := doReq("203.0.113.50"); code != http.StatusOK {
			t.Fatalf("client A request %d status = %d, want %d", i+1, code, http.StatusOK)
		}
	}
	if code := doReq("203.0.113.50"); code != http.StatusTooManyRequests {
		t.Fatalf("client A overflow status = %d, want %d", code, http.StatusTooManyRequests)
	}
	// Client B behind the same gateway must still have a full bucket.
	if code := doReq("203.0.113.51"); code != http.StatusOK {
		t.Fatalf("client B status = %d, want %d (buckets must not collapse into the gateway IP)", code, http.StatusOK)
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
	router.Use(rateLimitMiddleware(nil))
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
	router.Use(rateLimitMiddleware(nil))
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
