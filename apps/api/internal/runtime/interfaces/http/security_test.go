package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"ideaboard/api/internal/platform/settings"

	"github.com/gin-gonic/gin"
)

func TestSecurityHeadersMiddlewareSetsHardeningHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(SecurityHeadersMiddleware())
	router.GET("/ping", func(c *gin.Context) { c.Status(http.StatusOK) })

	res := httptest.NewRecorder()
	router.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/ping", nil))

	for header, want := range map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"Referrer-Policy":        "no-referrer",
	} {
		if got := res.Header().Get(header); got != want {
			t.Fatalf("%s = %q, want %q", header, got, want)
		}
	}
	if got := res.Header().Get("Permissions-Policy"); !strings.Contains(got, "camera=()") {
		t.Fatalf("Permissions-Policy = %q, want camera=() restriction", got)
	}
}

func TestUploadBypassPathsDeriveFromRouteConstants(t *testing.T) {
	if UserAPIGroupPath+RouteUserProfileAvatar != "/api/user/profile/avatar" {
		t.Fatalf("avatar bypass path drifted: %q", UserAPIGroupPath+RouteUserProfileAvatar)
	}
	if AdminAPIGroupPath+RouteAdminUpload != "/api/admin/upload" {
		t.Fatalf("admin upload bypass path drifted: %q", AdminAPIGroupPath+RouteAdminUpload)
	}
	if UserAPIGroupPath+RouteUserBoardUploads != "/api/user/boards/:board_id/uploads" {
		t.Fatalf("board upload route drifted: %q", UserAPIGroupPath+RouteUserBoardUploads)
	}
	if UserAPIGroupPath+RouteUserEvents != "/api/user/events" {
		t.Fatalf("user events route drifted: %q", UserAPIGroupPath+RouteUserEvents)
	}
	if UploadsStaticRoutePrefix != "/uploads/" {
		t.Fatalf("uploads prefix drifted: %q", UploadsStaticRoutePrefix)
	}
}

func TestRequestTimeoutMiddlewareEnforcesDeadline(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RequestTimeoutMiddleware(20 * time.Millisecond))
	router.GET("/slow", func(c *gin.Context) {
		select {
		case <-c.Request.Context().Done():
			if c.Request.Context().Err() != context.DeadlineExceeded {
				t.Errorf("context err = %v, want DeadlineExceeded", c.Request.Context().Err())
			}
			return
		case <-time.After(2 * time.Second):
			t.Error("handler should observe timeout, not sleep through it")
			return
		}
	})

	res := httptest.NewRecorder()
	router.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/slow", nil))
	if res.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d; body=%s", res.Code, http.StatusServiceUnavailable, res.Body.String())
	}
}

func TestRequestTimeoutMiddlewareExemptsStreamsAndUploads(t *testing.T) {
	for path, method := range map[string]string{
		UserAPIGroupPath + RouteUserEvents:        http.MethodGet,
		RoutePaymentStripeWebhook:                 http.MethodPost,
		UploadsStaticRoutePrefix + "canvas/a.png": http.MethodGet,
		UserAPIGroupPath + RouteUserProfileAvatar: http.MethodPost,
		AdminAPIGroupPath + RouteAdminUpload:      http.MethodPost,
		UserAPIGroupPath + RouteUserVisionExtract: http.MethodPost,
		UserAPIGroupPath + "/boards/b1/uploads":   http.MethodPost,
		UserAPIGroupPath + RouteUserDownload:      http.MethodGet,
	} {
		req := httptest.NewRequest(method, path, nil)
		if method == http.MethodPost {
			req.ContentLength = 1
		}
		if !isTimeoutExemptRequest(req) {
			t.Fatalf("%s %s should be timeout-exempt", method, path)
		}
	}

	jsonReq := httptest.NewRequest(http.MethodPost, "/api/user/tasks/submit", nil)
	if isTimeoutExemptRequest(jsonReq) {
		t.Fatal("task submit should not be timeout-exempt")
	}
}

type blockingRateLimitProvider struct {
	started chan struct{}
	once    sync.Once
}

func (p *blockingRateLimitProvider) GetRateLimit(ctx context.Context) (settings.RateLimitSetting, error) {
	p.once.Do(func() { close(p.started) })
	<-ctx.Done()
	return settings.RateLimitSetting{}, ctx.Err()
}

func TestRequestTimeoutCoversRateLimitSettingsLookup(t *testing.T) {
	gin.SetMode(gin.TestMode)
	provider := &blockingRateLimitProvider{started: make(chan struct{})}
	router := gin.New()
	router.Use(RequestTimeoutMiddleware(20 * time.Millisecond))
	router.Use(rateLimitMiddleware(provider))
	router.GET("/slow-policy", func(c *gin.Context) { c.Status(http.StatusOK) })

	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/slow-policy", nil)
	req.RemoteAddr = "203.0.113.50:12345"
	router.ServeHTTP(res, req)

	select {
	case <-provider.started:
	default:
		t.Fatal("rate-limit settings provider was not called")
	}
	if res.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d; body=%s", res.Code, http.StatusServiceUnavailable, res.Body.String())
	}
}
