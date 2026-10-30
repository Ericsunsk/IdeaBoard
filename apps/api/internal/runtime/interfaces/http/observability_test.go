package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"ideaboard/api/internal/runtime/domain"

	"github.com/gin-gonic/gin"
)

type stubReadyChecker struct {
	status domain.ComponentStatus
}

func (s stubReadyChecker) CheckHealth(context.Context) domain.ComponentStatus {
	return s.status
}

func newObservabilityTestRouter(checker stubReadyChecker, useChecker bool) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	deps := Dependencies{
		CORSMiddleware: func(c *gin.Context) { c.Next() },
		RateLimit:      stubRateLimitSettingsProvider{},
	}
	if useChecker {
		deps.ReadyChecker = checker
	}
	RegisterRoutes(router, deps, fstest.MapFS{}, FrontendRouteOptions{})
	return router
}

func TestRequestIDMiddlewareMintsAndEchoes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RequestIDMiddleware())
	router.GET("/ping", func(c *gin.Context) {
		if RequestIDFromContext(c.Request.Context()) == "" {
			t.Error("request context should carry request ID")
		}
		c.String(http.StatusOK, c.GetString(RequestIDHeader))
	})

	res := httptest.NewRecorder()
	router.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/ping", nil))
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", res.Code, http.StatusOK)
	}
	minted := strings.TrimSpace(res.Body.String())
	if minted == "" || res.Header().Get(RequestIDHeader) != minted {
		t.Fatalf("minted request ID not echoed: header=%q body=%q", res.Header().Get(RequestIDHeader), minted)
	}

	echo := httptest.NewRecorder()
	echoReq := httptest.NewRequest(http.MethodGet, "/ping", nil)
	echoReq.Header.Set(RequestIDHeader, "client-123")
	router.ServeHTTP(echo, echoReq)
	if strings.TrimSpace(echo.Body.String()) != "client-123" || echo.Header().Get(RequestIDHeader) != "client-123" {
		t.Fatalf("client request ID not preserved: header=%q body=%q", echo.Header().Get(RequestIDHeader), echo.Body.String())
	}
}

func TestRecoveryMiddlewareRendersErrorEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RequestIDMiddleware())
	router.Use(RecoveryMiddleware())
	router.GET("/boom", func(*gin.Context) { panic("boom") })

	res := httptest.NewRecorder()
	router.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/boom", nil))
	if res.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d; body=%s", res.Code, http.StatusInternalServerError, res.Body.String())
	}
	var payload struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &payload); err != nil {
		t.Fatalf("recovery body is not the error envelope: %v; body=%s", err, res.Body.String())
	}
	if payload.Code != http.StatusInternalServerError || payload.Msg == "" {
		t.Fatalf("envelope = %+v, want 500 with safe message", payload)
	}
	if res.Header().Get(RequestIDHeader) == "" {
		t.Fatal("recovery response should carry request ID header")
	}
}

func TestHealthStaysShallowAndLivezExists(t *testing.T) {
	router := newObservabilityTestRouter(stubReadyChecker{}, false)
	for _, path := range []string{RouteHealth, "/api/livez"} {
		res := httptest.NewRecorder()
		router.ServeHTTP(res, httptest.NewRequest(http.MethodGet, path, nil))
		if res.Code != http.StatusOK {
			t.Fatalf("%s status = %d, want %d; body=%s", path, res.Code, http.StatusOK, res.Body.String())
		}
		if res.Header().Get(RequestIDHeader) == "" {
			t.Fatalf("%s should carry request ID header", path)
		}
	}
}

func TestReadyzWithoutCheckerStaysShallow(t *testing.T) {
	router := newObservabilityTestRouter(stubReadyChecker{}, false)
	res := httptest.NewRecorder()
	router.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/api/readyz", nil))
	if res.Code != http.StatusOK {
		t.Fatalf("readyz status = %d, want %d; body=%s", res.Code, http.StatusOK, res.Body.String())
	}
}

func TestReadyzReportsCheckerStatus(t *testing.T) {
	ready := newObservabilityTestRouter(stubReadyChecker{
		status: domain.ComponentStatus{Name: "database", Kind: domain.ComponentKindDatabase, Ready: true, Required: true},
	}, true)
	res := httptest.NewRecorder()
	ready.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/api/readyz", nil))
	if res.Code != http.StatusOK || !strings.Contains(res.Body.String(), `"ready":true`) {
		t.Fatalf("ready status = %d, want 200 with ready:true; body=%s", res.Code, res.Body.String())
	}

	unready := newObservabilityTestRouter(stubReadyChecker{
		status: domain.ComponentStatus{Name: "database", Kind: domain.ComponentKindDatabase, Ready: false, Required: true, Detail: " pool down"},
	}, true)
	unreadyRes := httptest.NewRecorder()
	unready.ServeHTTP(unreadyRes, httptest.NewRequest(http.MethodGet, "/api/readyz", nil))
	if unreadyRes.Code != http.StatusServiceUnavailable {
		t.Fatalf("unready status = %d, want %d; body=%s", unreadyRes.Code, http.StatusServiceUnavailable, unreadyRes.Body.String())
	}
	if unreadyRes.Header().Get(RequestIDHeader) == "" {
		t.Fatal("unready response should carry request ID header")
	}
}
