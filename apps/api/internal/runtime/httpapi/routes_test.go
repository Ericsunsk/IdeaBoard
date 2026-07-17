package httpapi

import (
	"bytes"
	"context"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	"ideaboard/api/internal/accountpool/accounts"
	"ideaboard/api/internal/accountpool/providerendpoints"
	"ideaboard/api/internal/billing"
	"ideaboard/api/internal/billingrecords"
	"ideaboard/api/internal/boards"
	"ideaboard/api/internal/credits"
	"ideaboard/api/internal/identity/auth"
	"ideaboard/api/internal/identity/users"
	"ideaboard/api/internal/media/assets"
	"ideaboard/api/internal/media/storage"
	"ideaboard/api/internal/platform/config"
	"ideaboard/api/internal/platform/settings"
	"ideaboard/api/internal/prompts"
	"ideaboard/api/internal/tasking/taskservice"
	"ideaboard/api/internal/tasking/taskstats"
	"ideaboard/api/internal/testdb"
	"ideaboard/api/internal/usage"

	"github.com/gin-gonic/gin"
)

type stubBrandingSettingsProvider struct {
	setting settings.BrandingSetting
	err     error
}

func (p stubBrandingSettingsProvider) GetBranding(_ context.Context) (settings.BrandingSetting, error) {
	return p.setting, p.err
}

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
	adminGroup.GET(RouteAdminSettingsGoogleNativeGeneration, func(c *gin.Context) {})

	want := map[string]bool{
		http.MethodGet + " /api/settings/branding":                                 false,
		http.MethodPost + " /api/user/profile/avatar":                              false,
		http.MethodPost + " /api/user/boards/:board_id/uploads":                    false,
		http.MethodGet + " /api/user/download":                                     false,
		http.MethodPost + " /api/admin/upload":                                     false,
		http.MethodGet + " /api/admin/settings/branding":                           false,
		http.MethodGet + " /api/admin/settings/media-upload":                       false,
		http.MethodGet + " /api/admin/settings/public-urls":                        false,
		http.MethodGet + " /api/admin/settings/rate-limit":                         false,
		http.MethodGet + " /api/admin/settings/task-monitor":                       false,
		http.MethodGet + " /api/admin/settings/generation/google-native":           false,
		http.MethodPost + " /api/user/api/user/profile/avatar":                     true,
		http.MethodPost + " /api/admin/api/admin/upload":                           true,
		http.MethodGet + " /api/admin/api/admin/settings/branding":                 true,
		http.MethodGet + " /api/admin/api/admin/settings/media-upload":             true,
		http.MethodGet + " /api/admin/api/admin/settings/public-urls":              true,
		http.MethodGet + " /api/admin/api/admin/settings/rate-limit":               true,
		http.MethodGet + " /api/admin/api/admin/settings/task-monitor":             true,
		http.MethodGet + " /api/admin/api/admin/settings/generation/google-native": true,
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

func TestAdminRoutesRequireAuthenticatedAdminAccess(t *testing.T) {
	gin.SetMode(gin.TestMode)

	authUserID := "auth-admin-route-user"
	authServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/internal/session" {
			http.NotFound(w, r)
			return
		}
		if cookie := r.Header.Get("Cookie"); !strings.Contains(cookie, "better-auth.session_token=valid") {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"msg":"success","data":{"session":{"auth_user_id":"` + authUserID + `","username":"route-user","claim_identity":"route-user@users.ideaboard.local","claim_identity_kind":"trusted_canonical_registration","claim_identity_verified":true}}}`))
	}))
	defer authServer.Close()

	db, pool := testdb.OpenWithPool(t)
	userRepo := users.NewUserRepository(db)
	routeUser := &users.User{
		Username:   "route-user",
		AuthUserID: &authUserID,
		Status:     1,
		Role:       "user",
	}
	if err := userRepo.Create(context.Background(), routeUser); err != nil {
		t.Fatalf("seed local user failed: %v", err)
	}

	authDeps := AuthDeps{
		AuthServiceClient: auth.NewServiceClient(config.AuthServiceConfig{
			BaseURL:        authServer.URL,
			TimeoutSeconds: 1,
		}),
		UserRepo: userRepo,
	}
	router := gin.New()
	RegisterRoutes(router, newAdminRouteGuardTestDependencies(authDeps), fstest.MapFS{}, FrontendRouteOptions{})

	adminRoutes := collectAdminRoutes(t, router)
	assertAdminRouteSet(t, adminRoutes)

	for _, route := range adminRoutes {
		t.Run(route.Method+" "+route.Path, func(t *testing.T) {
			noCookie := httptest.NewRecorder()
			router.ServeHTTP(noCookie, newAdminRouteGuardRequest(t, route, ""))
			if noCookie.Code != http.StatusUnauthorized {
				t.Fatalf("no-cookie status = %d, want %d; body=%s", noCookie.Code, http.StatusUnauthorized, noCookie.Body.String())
			}

			localUser := httptest.NewRecorder()
			router.ServeHTTP(localUser, newAdminRouteGuardRequest(t, route, "better-auth.session_token=valid"))
			if localUser.Code != http.StatusForbidden {
				t.Fatalf("local user status = %d, want %d; body=%s", localUser.Code, http.StatusForbidden, localUser.Body.String())
			}
		})
	}
	if err := userRepo.UpdateRoleByID(context.Background(), routeUser.ID, "admin"); err != nil {
		t.Fatalf("promote route user to admin: %v", err)
	}
	adminDeps := newAdminRouteGuardTestDependencies(authDeps)
	adminDeps.AdminRoutes.CreditHandler = credits.NewHandler(credits.NewService(pool))
	router = gin.New()
	RegisterRoutes(router, adminDeps, fstest.MapFS{}, FrontendRouteOptions{})
	adminRecorder := httptest.NewRecorder()
	adminRequest := httptest.NewRequest(http.MethodPost, "/api/admin/credits/grants", bytes.NewBufferString(`{"user_id":`+strconv.FormatInt(routeUser.ID, 10)+`,"amount_credits":1,"idempotency_key":"route-admin-grant","reason":"route auth test","business_reference_id":"route-admin-grant"}`))
	adminRequest.Header.Set("Content-Type", "application/json")
	adminRequest.Header.Set("Cookie", "better-auth.session_token=valid")
	router.ServeHTTP(adminRecorder, adminRequest)
	if adminRecorder.Code != http.StatusOK {
		t.Fatalf("admin Grant status = %d, want %d; body=%s", adminRecorder.Code, http.StatusOK, adminRecorder.Body.String())
	}
}

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

type adminRouteUnderTest struct {
	Method string
	Path   string
}

func newAdminRouteGuardTestDependencies(authDeps AuthDeps) Dependencies {
	return Dependencies{
		Auth: authDeps,
		UserRoutes: UserRouteDeps{
			AuthHandler:      auth.NewAuthHandler(nil),
			TaskHandler:      taskservice.NewHandler(nil),
			BoardHandler:     boards.NewBoardHandler(nil, nil),
			LineDraftHandler: assets.NewLineDraftHandler(nil),
			BillingHandler:   billing.NewHandler(nil, nil),
			BillingRecords:   billingrecords.NewHandler(nil),
			CreditHandler:    credits.NewHandler(nil),
			UsageHandler:     usage.NewHandler(nil),
			Uploads: UploadDeps{
				UserRepo:       authDeps.UserRepo,
				UploadSettings: &fakeMediaUploadSettingsProvider{},
			},
		},
		AdminRoutes: AdminRouteDeps{
			AdminHandler:            users.NewAdminHandler(nil, nil, nil),
			BillingHandler:          billing.NewHandler(nil, nil),
			CreditHandler:           credits.NewHandler(nil),
			AccountHandler:          accounts.NewAccountHandler(nil),
			TaskHandler:             taskservice.NewHandler(nil),
			TaskStatsHandler:        taskstats.NewHandler(nil, nil),
			ProviderEndpointHandler: providerendpoints.NewProviderEndpointHandler(nil),
			StorageHandler:          storage.NewStorageHandler(nil),
			LineDraftHandler:        assets.NewLineDraftHandler(nil),
			FeaturePromptHandler:    prompts.NewFeaturePromptHandler(nil),
			UploadSettings:          &fakeMediaUploadSettingsProvider{},
		},
		StaticUploads: StaticUploadDeps{Auth: authDeps},
		CORSMiddleware: func(c *gin.Context) {
			c.Next()
		},
		RateLimit: stubRateLimitSettingsProvider{
			setting: settings.RateLimitSetting{
				WindowSeconds: 1,
				MaxRequests:   10000,
			},
		},
	}
}

func collectAdminRoutes(t *testing.T, router *gin.Engine) []adminRouteUnderTest {
	t.Helper()

	var routes []adminRouteUnderTest
	for _, route := range router.Routes() {
		if route.Path == "/api/admin" || strings.HasPrefix(route.Path, "/api/admin/") {
			routes = append(routes, adminRouteUnderTest{
				Method: route.Method,
				Path:   route.Path,
			})
		}
	}
	if len(routes) == 0 {
		t.Fatal("no /api/admin routes were registered")
	}
	sort.Slice(routes, func(i, j int) bool {
		if routes[i].Path == routes[j].Path {
			return routes[i].Method < routes[j].Method
		}
		return routes[i].Path < routes[j].Path
	})
	return routes
}

func assertAdminRouteSet(t *testing.T, routes []adminRouteUnderTest) {
	t.Helper()

	const rewardBatchRevokeRoute = "POST /api/admin/rewards/campaigns/:campaign_id/batches/:batch_id/revoke"

	want := map[string]bool{
		"DELETE /api/admin/accounts/:id":                              false,
		"DELETE /api/admin/accounts/:id/models":                       false,
		"DELETE /api/admin/billing/catalog/:sku":                      false,
		"DELETE /api/admin/provider-endpoints/:id":                    false,
		"DELETE /api/admin/rewards/campaigns/:campaign_id":            false,
		"DELETE /api/admin/line-draft-modes/:id":                      false,
		"DELETE /api/admin/storage/profiles/:id":                      false,
		"DELETE /api/admin/users/:id":                                 false,
		"GET /api/admin/accounts":                                     false,
		"GET /api/admin/accounts/:id":                                 false,
		"GET /api/admin/accounts/:id/queue":                           false,
		"GET /api/admin/accounts/limits":                              false,
		"GET /api/admin/provider-endpoints":                           false,
		"GET /api/admin/feature-prompts":                              false,
		"GET /api/admin/feature-prompts/:slug":                        false,
		"GET /api/admin/credits/wallets/:user_id":                     false,
		"GET /api/admin/credits/wallets/:user_id/ledger":              false,
		"GET /api/admin/credits/health":                               false,
		"GET /api/admin/credits/reconciliation/runs":                  false,
		"GET /api/admin/credits/reconciliation/runs/:run_id":          false,
		"GET /api/admin/credits/reconciliation/runs/:run_id/findings": false,
		"GET /api/admin/billing/catalog":                              false,
		"GET /api/admin/billing/catalog/:sku/prices":                  false,
		"GET /api/admin/rewards/campaigns":                            false,
		"GET /api/admin/rewards/campaigns/:campaign_id":               false,
		"GET /api/admin/rewards/campaigns/:campaign_id/batches":       false,
		"GET /api/admin/rewards/campaigns/:campaign_id/redemptions":   false,
		"GET /api/admin/line-draft-modes":                             false,
		"GET /api/admin/monitor":                                      false,
		"GET /api/admin/monitor/stream":                               false,
		"GET /api/admin/settings/branding":                            false,
		"GET /api/admin/settings/media-upload":                        false,
		"GET /api/admin/settings/operational":                         false,
		"GET /api/admin/settings/public-urls":                         false,
		"GET /api/admin/settings/rate-limit":                          false,
		"GET /api/admin/settings/registration":                        false,
		"GET /api/admin/settings/task-monitor":                        false,
		"GET /api/admin/settings/generation/google-native":            false,
		"GET /api/admin/statistics":                                   false,
		"GET /api/admin/storage/profiles":                             false,
		"GET /api/admin/storage/profiles/:id/bucket-info":             false,
		"GET /api/admin/storage/providers":                            false,
		"GET /api/admin/tasks":                                        false,
		"GET /api/admin/tasks/:task_id":                               false,
		"GET /api/admin/users":                                        false,
		"POST /api/admin/accounts":                                    false,
		"POST /api/admin/credits/grants":                              false,
		"POST /api/admin/credits/deductions":                          false,
		"POST /api/admin/credits/corrections":                         false,
		"POST /api/admin/credits/refunds":                             false,
		"POST /api/admin/credits/reconciliation/runs":                 false,
		"POST /api/admin/billing/catalog/:sku/prices":                 false,
		"POST /api/admin/rewards/campaigns":                           false,
		"POST /api/admin/rewards/campaigns/:campaign_id/public-code":  false,
		"POST /api/admin/rewards/campaigns/:campaign_id/batches":      false,
		rewardBatchRevokeRoute:                                        false,
		"POST /api/admin/rewards/codes/lookup":                        false,
		"POST /api/admin/rewards/codes/:code_id/revoke":               false,
		"POST /api/admin/rewards/campaigns/:campaign_id/activate":     false,
		"POST /api/admin/rewards/campaigns/:campaign_id/cancel":       false,
		"POST /api/admin/rewards/campaigns/:campaign_id/pause":        false,
		"POST /api/admin/rewards/campaigns/:campaign_id/resume":       false,
		"POST /api/admin/accounts/:id/start":                          false,
		"POST /api/admin/accounts/:id/stop":                           false,
		"POST /api/admin/provider-endpoints":                          false,
		"POST /api/admin/provider-endpoints/detect-models":            false,
		"POST /api/admin/line-draft-modes":                            false,
		"POST /api/admin/statistics/backfill":                         false,
		"POST /api/admin/storage/profiles":                            false,
		"POST /api/admin/storage/profiles/:id/apply":                  false,
		"POST /api/admin/storage/profiles/:id/test":                   false,
		"POST /api/admin/tasks/submit":                                false,
		"POST /api/admin/tasks/bulk-delete":                           false,
		"POST /api/admin/upload":                                      false,
		"POST /api/admin/users/:id/ban":                               false,
		"POST /api/admin/users/:id/revoke-sessions":                   false,
		"POST /api/admin/users/:id/unban":                             false,
		"PATCH /api/admin/accounts/:id/models":                        false,
		"PUT /api/admin/accounts/:id":                                 false,
		"PUT /api/admin/billing/catalog/:sku/availability":            false,
		"PUT /api/admin/provider-endpoints/:id":                       false,
		"PUT /api/admin/rewards/campaigns/:campaign_id":               false,
		"PUT /api/admin/feature-prompts/:slug":                        false,
		"PUT /api/admin/line-draft-modes/:id":                         false,
		"PUT /api/admin/settings/branding":                            false,
		"PUT /api/admin/settings/media-upload":                        false,
		"PUT /api/admin/settings/operational":                         false,
		"PUT /api/admin/settings/public-urls":                         false,
		"PUT /api/admin/settings/rate-limit":                          false,
		"PUT /api/admin/settings/registration":                        false,
		"PUT /api/admin/settings/task-monitor":                        false,
		"PUT /api/admin/settings/generation/google-native":            false,
		"PUT /api/admin/storage/profiles/:id":                         false,
		"PUT /api/admin/users/:id/role":                               false,
	}

	for _, route := range routes {
		key := adminRouteKey(route)
		if _, ok := want[key]; !ok {
			t.Fatalf("registered admin route %s is not covered by this regression test's route set", key)
		}
		want[key] = true
	}
	for key, found := range want {
		if !found {
			t.Fatalf("expected admin route %s was not registered", key)
		}
	}
}

func newAdminRouteGuardRequest(t *testing.T, route adminRouteUnderTest, cookie string) *http.Request {
	t.Helper()

	body := strings.NewReader("")
	if route.Method == http.MethodPost || route.Method == http.MethodPut || route.Method == http.MethodPatch || route.Method == http.MethodDelete {
		body = strings.NewReader("{}")
	}
	req := httptest.NewRequest(route.Method, sampleAdminRoutePath(t, route.Path), body)
	if body.Len() > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	if cookie != "" {
		req.Header.Set("Cookie", cookie)
	}
	return req
}

func sampleAdminRoutePath(t *testing.T, routePath string) string {
	t.Helper()

	parts := strings.Split(routePath, "/")
	for i, part := range parts {
		switch {
		case strings.HasPrefix(part, ":"):
			parts[i] = sampleAdminRouteParam(strings.TrimPrefix(part, ":"))
		case strings.HasPrefix(part, "*"):
			parts[i] = "sample-file"
		}
	}
	path := strings.Join(parts, "/")
	if strings.Contains(path, ":") || strings.Contains(path, "*") {
		t.Fatalf("admin route %s was not converted to a concrete sample path: %s", routePath, path)
	}
	return path
}

func sampleAdminRouteParam(name string) string {
	switch name {
	case "id":
		return "123"
	case "slug":
		return "text-to-image"
	case "task_id":
		return "task-123"
	case "user_id":
		return "42"
	default:
		return "sample"
	}
}

func adminRouteKey(route adminRouteUnderTest) string {
	return route.Method + " " + route.Path
}
