package httpapi

// Route registration tests for the post-#809 surface: frontend fallback,
// branding, public search, and static uploads. Legacy /api/user and
// /api/admin route assertions were deleted with the hard cut (see
// contract_inventory_test.go for the legacy-zero gate).

import (
	"bytes"
	"context"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"testing/fstest"

	"ideaboard/api/internal/platform/settings"
	searchhttp "ideaboard/api/internal/searchcatalog/interfaces/http"

	"github.com/gin-gonic/gin"
)

type stubBrandingSettingsProvider struct {
	setting settings.BrandingSetting
	err     error
}

func (p stubBrandingSettingsProvider) GetBranding(_ context.Context) (settings.BrandingSetting, error) {
	return p.setting, p.err
}

// stubRateLimitSettingsProvider is shared with observability_test.go.
type stubRateLimitSettingsProvider struct {
	setting settings.RateLimitSetting
}

func (p stubRateLimitSettingsProvider) GetRateLimit(_ context.Context) (settings.RateLimitSetting, error) {
	return p.setting, nil
}

func TestRegisterFrontendRoutesSkipsMissingIndexQuietlyInDevelopment(t *testing.T) {
	var logs bytes.Buffer
	oldLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() {
		slog.SetDefault(oldLogger)
	})

	gin.SetMode(gin.TestMode)
	RegisterFrontendRoutes(gin.New(), fstest.MapFS{}, FrontendRouteOptions{})

	if strings.Contains(logs.String(), "embedded frontend index.html not found") {
		t.Fatalf("development route registration should not warn when embedded index is missing: %s", logs.String())
	}
}

func TestRegisterFrontendRoutesWarnsWhenMissingIndexInProduction(t *testing.T) {
	var logs bytes.Buffer
	oldLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() {
		slog.SetDefault(oldLogger)
	})

	gin.SetMode(gin.TestMode)
	RegisterFrontendRoutes(gin.New(), fstest.MapFS{}, FrontendRouteOptions{WarnMissingIndex: true})

	if !strings.Contains(logs.String(), "embedded frontend index.html not found") {
		t.Fatalf("production route registration should warn when embedded index is missing: %s", logs.String())
	}
}

func TestRegisterFrontendRoutesAcceptsEmbeddedIndex(t *testing.T) {
	gin.SetMode(gin.TestMode)
	RegisterFrontendRoutes(gin.New(), fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("<html></html>")},
	}, FrontendRouteOptions{WarnMissingIndex: true})
}

var _ fs.FS = fstest.MapFS{}

func TestRegisterStaticUploadRoutesDoesNotCreateUploadsDirectory(t *testing.T) {
	t.Chdir(t.TempDir())
	gin.SetMode(gin.TestMode)

	registerStaticUploadRoutes(gin.New(), StaticUploadDeps{})

	if _, err := os.Stat("uploads"); !os.IsNotExist(err) {
		t.Fatalf("uploads stat error = %v, want directory to remain absent", err)
	}
}

func TestPublicBrandingSettingsHandlerReturnsBranding(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET(RoutePublicSettingsBranding, publicBrandingSettingsHandler(stubBrandingSettingsProvider{
		setting: settings.BrandingSetting{
			ProjectName: "Design Ops",
			LogoURL:     "/uploads/site/assets/logo.png",
		},
	}))

	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, RoutePublicSettingsBranding, nil)
	router.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", res.Code, http.StatusOK, res.Body.String())
	}
	body := res.Body.String()
	if !strings.Contains(body, `"project_name":"Design Ops"`) || !strings.Contains(body, `"logo_url":"/uploads/site/assets/logo.png"`) {
		t.Fatalf("body = %s, want branding payload", body)
	}
}

func TestPublicBrandingSettingsHandlerRequiresProvider(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET(RoutePublicSettingsBranding, publicBrandingSettingsHandler(nil))

	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, RoutePublicSettingsBranding, nil)
	router.ServeHTTP(res, req)

	if res.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d; body=%s", res.Code, http.StatusInternalServerError, res.Body.String())
	}
}

func TestPublicSearchRouteDoesNotRequireAuthentication(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	registerPublicRoutes(router, nil, searchhttp.NewSearchHandler(nil), nil)

	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, RoutePublicSearch, nil)
	router.ServeHTTP(res, req)

	if res.Code == http.StatusUnauthorized {
		t.Fatalf("public search route unexpectedly required authentication: status=%d body=%s", res.Code, res.Body.String())
	}
	if res.Code != http.StatusServiceUnavailable || !strings.Contains(res.Body.String(), `"code":"search_unavailable"`) {
		t.Fatalf("status = %d body=%s, want unauthenticated search_unavailable 503", res.Code, res.Body.String())
	}
}
