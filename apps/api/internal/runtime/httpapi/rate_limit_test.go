package httpapi

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRateLimitMiddlewareAllowsInitialBurst(t *testing.T) {
	resetRateLimitState()
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
	resetRateLimitState()
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

func resetRateLimitState() {
	rateLimitStates = sync.Map{}
	rateLimitCount = atomic.Int64{}
}
