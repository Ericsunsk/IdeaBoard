package httpapi

import (
	"bytes"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/gin-gonic/gin"
)

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
	userGroup := router.Group("/api/user")
	userGroup.POST(RouteUserProfileAvatar, func(c *gin.Context) {})
	userGroup.POST(RouteUserBoardUploads, func(c *gin.Context) {})
	userGroup.GET(RouteUserDownload, func(c *gin.Context) {})

	adminGroup := router.Group("/api/admin")
	adminGroup.POST(RouteAdminUpload, func(c *gin.Context) {})
	adminGroup.GET(RouteAdminSettingsRateLimit, func(c *gin.Context) {})

	want := map[string]bool{
		http.MethodPost + " /api/user/profile/avatar":                false,
		http.MethodPost + " /api/user/boards/:board_id/uploads":      false,
		http.MethodGet + " /api/user/download":                       false,
		http.MethodPost + " /api/admin/upload":                       false,
		http.MethodGet + " /api/admin/settings/rate-limit":           false,
		http.MethodPost + " /api/user/api/user/profile/avatar":       true,
		http.MethodPost + " /api/admin/api/admin/upload":             true,
		http.MethodGet + " /api/admin/api/admin/settings/rate-limit": true,
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
