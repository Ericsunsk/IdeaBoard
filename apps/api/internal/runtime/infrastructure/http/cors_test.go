package http

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestBuildCORSMiddlewareProductionIgnoresLegacyAllowlist(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(BuildCORSMiddleware("production", []string{"https://app.example.com"}))
	router.OPTIONS("/api/health", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodOptions, "/api/health", nil)
	req.Header.Set("Origin", "https://app.example.com")
	req.Header.Set("Access-Control-Request-Method", http.MethodPost)
	req.Header.Set("Access-Control-Request-Headers", "idempotency-key")
	res := httptest.NewRecorder()

	router.ServeHTTP(res, req)

	if res.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", res.Code, http.StatusForbidden)
	}
	if got := res.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want empty", got)
	}
}

func TestBuildCORSMiddlewareProductionConfiguredAllowlistRejectsCrossOrigin(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(BuildCORSMiddleware("production", []string{"https://app.example.com"}))
	router.GET("/api/health", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	req.Header.Set("Origin", "https://app.example.com")
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)

	if res.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", res.Code, http.StatusForbidden)
	}
}

func TestOriginPolicyDecideKeepsTransportHeadersAtTheSeam(t *testing.T) {
	policy := newOriginPolicy("development", []string{"https://app.example.com"})
	req := httptest.NewRequest(http.MethodOptions, "/api/health", nil)
	req.Header.Set("Origin", "https://app.example.com")
	req.Header.Set("Access-Control-Request-Method", http.MethodPost)
	decision := policy.Decide(req)
	if !decision.Allowed || !decision.Preflight {
		t.Fatalf("decision = %+v, want allowed preflight", decision)
	}
	if decision.Headers["Access-Control-Allow-Headers"] == "" {
		t.Fatal("preflight decision omitted allowed headers")
	}
}

func TestBuildCORSMiddlewareProductionNoAllowlistRejectsCrossOrigin(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(BuildCORSMiddleware("production", nil))
	router.GET("/api/health", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	req.Header.Set("Origin", "https://app.example.com")
	res := httptest.NewRecorder()

	router.ServeHTTP(res, req)

	if res.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", res.Code, http.StatusForbidden)
	}
	if got := res.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want empty", got)
	}
}

func TestBuildCORSMiddlewareProductionNoAllowlistAllowsNoOrigin(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(BuildCORSMiddleware("production", nil))
	router.GET("/api/health", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	res := httptest.NewRecorder()

	router.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", res.Code, http.StatusOK)
	}
}

func TestBuildCORSMiddlewareProductionNoAllowlistAllowsSameOrigin(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(BuildCORSMiddleware("production", nil))
	router.POST("/api/user/tasks/submit", func(c *gin.Context) {
		c.Status(http.StatusCreated)
	})

	req := httptest.NewRequest(http.MethodPost, "https://ideaboard.art/api/user/tasks/submit", nil)
	req.Host = "ideaboard.art"
	req.Header.Set("Origin", "https://ideaboard.art")
	req.Header.Set("X-Forwarded-Proto", "https")
	res := httptest.NewRecorder()

	router.ServeHTTP(res, req)

	if res.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", res.Code, http.StatusCreated)
	}
	if got := res.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("same-origin request unexpectedly returned ACAO = %q", got)
	}
}

func TestBuildCORSMiddlewareRejectsDisallowedPreflight(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(BuildCORSMiddleware("development", []string{"http://localhost:5173"}))
	router.OPTIONS("/api/health", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodOptions, "/api/health", nil)
	req.Header.Set("Origin", "https://evil.example.com")
	res := httptest.NewRecorder()

	router.ServeHTTP(res, req)

	if res.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", res.Code, http.StatusForbidden)
	}
}

func TestBuildCORSMiddlewareRejectsDisallowedSimpleRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(BuildCORSMiddleware("development", []string{"http://localhost:5173"}))
	router.GET("/api/health", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	req.Header.Set("Origin", "https://evil.example.com")
	res := httptest.NewRecorder()

	router.ServeHTTP(res, req)

	if res.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", res.Code, http.StatusForbidden)
	}
	if got := res.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want empty", got)
	}
}

func TestBuildCORSMiddlewareInvalidProductionAllowlistDoesNotFallbackToNoop(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(BuildCORSMiddleware("production", []string{"not-a-valid-origin"}))
	router.Any("/api/health", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	for _, method := range []string{http.MethodOptions, http.MethodGet} {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/api/health", nil)
			req.Header.Set("Origin", "https://app.example.com")
			res := httptest.NewRecorder()

			router.ServeHTTP(res, req)

			if res.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want %d", res.Code, http.StatusForbidden)
			}
		})
	}
}
