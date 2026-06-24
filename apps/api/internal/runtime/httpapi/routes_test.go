package httpapi

import (
	"bytes"
	"context"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"ideaboard/api/internal/platform/settings"

	"github.com/gin-gonic/gin"
)

type stubBrandingSettingsProvider struct {
	setting settings.BrandingSetting
	err     error
}

func (p stubBrandingSettingsProvider) GetBranding(_ context.Context) (settings.BrandingSetting, error) {
	return p.setting, p.err
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

func TestRouteConstantsAreGroupRelative(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET(RoutePublicSettingsBranding, func(c *gin.Context) {})
	userGroup := router.Group("/api/user")
	userGroup.POST(RouteUserProfileAvatar, func(c *gin.Context) {})
	userGroup.POST(RouteUserBoardUploads, func(c *gin.Context) {})
	userGroup.GET(RouteUserDownload, func(c *gin.Context) {})

	adminGroup := router.Group("/api/admin")
	adminGroup.POST(RouteAdminUpload, func(c *gin.Context) {})
	adminGroup.GET(RouteAdminSettingsBranding, func(c *gin.Context) {})
	adminGroup.GET(RouteAdminSettingsMediaUpload, func(c *gin.Context) {})
	adminGroup.GET(RouteAdminSettingsPublicURLs, func(c *gin.Context) {})
	adminGroup.GET(RouteAdminSettingsRateLimit, func(c *gin.Context) {})
	adminGroup.GET(RouteAdminSettingsTaskMonitor, func(c *gin.Context) {})

	want := map[string]bool{
		http.MethodGet + " /api/settings/branding":                     false,
		http.MethodPost + " /api/user/profile/avatar":                  false,
		http.MethodPost + " /api/user/boards/:board_id/uploads":        false,
		http.MethodGet + " /api/user/download":                         false,
		http.MethodPost + " /api/admin/upload":                         false,
		http.MethodGet + " /api/admin/settings/branding":               false,
		http.MethodGet + " /api/admin/settings/media-upload":           false,
		http.MethodGet + " /api/admin/settings/public-urls":            false,
		http.MethodGet + " /api/admin/settings/rate-limit":             false,
		http.MethodGet + " /api/admin/settings/task-monitor":           false,
		http.MethodPost + " /api/user/api/user/profile/avatar":         true,
		http.MethodPost + " /api/admin/api/admin/upload":               true,
		http.MethodGet + " /api/admin/api/admin/settings/branding":     true,
		http.MethodGet + " /api/admin/api/admin/settings/media-upload": true,
		http.MethodGet + " /api/admin/api/admin/settings/public-urls":  true,
		http.MethodGet + " /api/admin/api/admin/settings/rate-limit":   true,
		http.MethodGet + " /api/admin/api/admin/settings/task-monitor": true,
	}
	for _, route := range router.Routes() {
		key := route.Method + " " + route.Path
		if shouldRemainMissing, ok := want[key]; ok {
			if shouldRemainMissing {
				t.Fatalf("route %s should not be registered", key)
			}
			want[key] = true
		}
	}
	for key, found := range want {
		if !found {
			t.Fatalf("route %s was not registered", key)
		}
	}
}

func TestPublicBrandingSettingsHandlerReturnsBranding(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET(RoutePublicSettingsBranding, publicBrandingSettingsHandler(stubBrandingSettingsProvider{
		setting: settings.BrandingSetting{
			ProjectName: "Design Ops",
			LogoURL:     "/uploads/admin/logo.png",
		},
	}))

	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, RoutePublicSettingsBranding, nil)
	router.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", res.Code, http.StatusOK, res.Body.String())
	}
	body := res.Body.String()
	if !strings.Contains(body, `"project_name":"Design Ops"`) || !strings.Contains(body, `"logo_url":"/uploads/admin/logo.png"`) {
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
