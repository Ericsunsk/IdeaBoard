package tests

import (
	"encoding/json"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/platform/settings"
	"ideaboard/api/internal/testsupport"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type adminBrandingSettingsResponse struct {
	Code int                      `json:"code"`
	Msg  string                   `json:"msg"`
	Data settings.BrandingSetting `json:"data"`
}

type adminMediaUploadSettingsResponse struct {
	Code int                         `json:"code"`
	Msg  string                      `json:"msg"`
	Data settings.MediaUploadSetting `json:"data"`
}

type adminOutboundPolicyResponse struct {
	Code int                            `json:"code"`
	Msg  string                         `json:"msg"`
	Data settings.OutboundPolicySetting `json:"data"`
}

func newAdminHandlerForRuntimeSettings(db *pgxpool.Pool) *AdminHandler {
	return NewAdminHandler(nil, settings.NewRuntimeSettingsService(settings.NewSystemConfigRepository(db)), nil)
}

func TestAdminRuntimeSettingsGetFailsWhenRepositoryUnavailable(t *testing.T) {

	handler := NewAdminHandler(nil, settings.NewRuntimeSettingsService(nil), nil)
	tests := []struct {
		name     string
		path     string
		register func(*gin.Engine)
	}{
		{
			name: "registration",
			path: "/admin/settings/registration",
			register: func(router *gin.Engine) {
				router.GET("/admin/settings/registration", handler.GetRegistrationSetting)
			},
		},
		{
			name: "rate limit",
			path: "/admin/settings/rate-limit",
			register: func(router *gin.Engine) {
				router.GET("/admin/settings/rate-limit", handler.GetRateLimitSettings)
			},
		},
		{
			name: "media upload",
			path: "/admin/settings/media-upload",
			register: func(router *gin.Engine) {
				router.GET("/admin/settings/media-upload", handler.GetMediaUploadSettings)
			},
		},
		{
			name: "branding",
			path: "/admin/settings/branding",
			register: func(router *gin.Engine) {
				router.GET("/admin/settings/branding", handler.GetBrandingSettings)
			},
		},
		{
			name: "outbound policy",
			path: "/admin/settings/outbound-policy",
			register: func(router *gin.Engine) {
				router.GET("/admin/settings/outbound-policy", handler.GetOutboundPolicy)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := gin.New()
			tt.register(router)

			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusInternalServerError {
				t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusInternalServerError, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), "读取系统设置失败") {
				t.Fatalf("body = %s, want runtime settings read failure message", rec.Body.String())
			}
		})
	}
}

func TestAdminOutboundPolicyUpdateRequiresActorAndAppliesImmediately(t *testing.T) {
	db := newUserRepoTestDB(t)
	service := settings.NewRuntimeSettingsService(settings.NewSystemConfigRepository(db))
	handler := NewAdminHandler(nil, service, nil)

	validBody := `{"allow_hosts":["internal.example.com:8443"]}`

	t.Run("missing actor", func(t *testing.T) {
		router := gin.New()
		router.PUT("/admin/settings/outbound-policy", handler.UpdateOutboundPolicy)
		req := httptest.NewRequest(http.MethodPut, "/admin/settings/outbound-policy", strings.NewReader(validBody))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusUnauthorized, rec.Body.String())
		}
	})

	t.Run("missing allow_hosts", func(t *testing.T) {
		router := gin.New()
		router.Use(func(c *gin.Context) { testsupport.SetPrincipal(t, c, 42, principal.RoleAdmin); c.Next() })
		router.PUT("/admin/settings/outbound-policy", handler.UpdateOutboundPolicy)
		req := httptest.NewRequest(http.MethodPut, "/admin/settings/outbound-policy", strings.NewReader(`{}`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "allow_hosts") {
			t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("valid update", func(t *testing.T) {
		router := gin.New()
		router.Use(func(c *gin.Context) { testsupport.SetPrincipal(t, c, 42, principal.RoleAdmin); c.Next() })
		router.PUT("/admin/settings/outbound-policy", handler.UpdateOutboundPolicy)
		req := httptest.NewRequest(http.MethodPut, "/admin/settings/outbound-policy", strings.NewReader(validBody))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
		}
		var payload adminOutboundPolicyResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if len(payload.Data.AllowHosts) != 1 || payload.Data.AllowHosts[0] != "internal.example.com:8443" {
			t.Fatalf("outbound policy = %+v", payload.Data)
		}
		effective, err := service.GetOutboundPolicy(t.Context())
		if err != nil || len(effective.AllowHosts) != 1 || effective.AllowHosts[0] != "internal.example.com:8443" {
			t.Fatalf("effective outbound policy = %+v, err=%v", effective, err)
		}
	})
}

func TestAdminBrandingSettingsDefaultsWhenConfigMissing(t *testing.T) {

	db := newUserRepoTestDB(t)
	handler := newAdminHandlerForRuntimeSettings(db)
	router := gin.New()
	router.GET("/admin/settings/branding", handler.GetBrandingSettings)

	req := httptest.NewRequest(http.MethodGet, "/admin/settings/branding", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got adminBrandingSettingsResponse
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	if got.Data.ProjectName != settings.DefaultBrandingSetting().ProjectName || got.Data.LogoURL != "" {
		t.Fatalf("branding = %+v, want default name and empty logo", got.Data)
	}
}

func TestAdminBrandingSettingsUpdateNormalizesAndPersists(t *testing.T) {

	db := newUserRepoTestDB(t)
	handler := newAdminHandlerForRuntimeSettings(db)
	router := gin.New()
	router.GET("/admin/settings/branding", handler.GetBrandingSettings)
	router.PUT("/admin/settings/branding", handler.UpdateBrandingSettings)

	req := httptest.NewRequest(http.MethodPut, "/admin/settings/branding", strings.NewReader(`{"project_name":"  Design Ops  ","logo_url":"  /uploads/site/assets/logo.png  "}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var updated adminBrandingSettingsResponse
	if err := json.NewDecoder(rec.Body).Decode(&updated); err != nil {
		t.Fatalf("decode update response failed: %v", err)
	}
	if updated.Data.ProjectName != "Design Ops" || updated.Data.LogoURL != "/uploads/site/assets/logo.png" {
		t.Fatalf("updated branding = %+v, want trimmed values", updated.Data)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/admin/settings/branding", nil)
	getRec := httptest.NewRecorder()
	router.ServeHTTP(getRec, getReq)

	var got adminBrandingSettingsResponse
	if err := json.NewDecoder(getRec.Body).Decode(&got); err != nil {
		t.Fatalf("decode get response failed: %v", err)
	}
	if got.Data != updated.Data {
		t.Fatalf("persisted branding = %+v, want %+v", got.Data, updated.Data)
	}
}

func TestAdminBrandingSettingsRejectsInvalidPayload(t *testing.T) {

	db := newUserRepoTestDB(t)
	handler := newAdminHandlerForRuntimeSettings(db)
	router := gin.New()
	router.PUT("/admin/settings/branding", handler.UpdateBrandingSettings)

	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "blank project name",
			body: `{"project_name":"  ","logo_url":""}`,
			want: "project_name 不能为空",
		},
		{
			name: "too long project name",
			body: `{"project_name":"` + strings.Repeat("名", 41) + `","logo_url":""}`,
			want: "project_name 不能超过 40 个字符",
		},
		{
			name: "invalid logo url",
			body: `{"project_name":"IdeaBoard","logo_url":"javascript:alert(1)"}`,
			want: "logo_url 必须为空、/uploads/site/assets/ 路径或 http(s) URL",
		},
		{
			name: "unsafe upload logo path",
			body: `{"project_name":"IdeaBoard","logo_url":"/uploads/site/assets/../secret.png"}`,
			want: "logo_url 必须为空、/uploads/site/assets/ 路径或 http(s) URL",
		},
		{
			name: "private upload logo path",
			body: `{"project_name":"IdeaBoard","logo_url":"/uploads/profile/OWNR1A/avatar/logo.png"}`,
			want: "logo_url 必须为空、/uploads/site/assets/ 路径或 http(s) URL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPut, "/admin/settings/branding", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), tt.want) {
				t.Fatalf("body = %s, want %q", rec.Body.String(), tt.want)
			}
		})
	}
}

func TestAdminMediaUploadSettingsDefaultsWhenConfigMissing(t *testing.T) {

	db := newUserRepoTestDB(t)
	handler := newAdminHandlerForRuntimeSettings(db)
	router := gin.New()
	router.GET("/admin/settings/media-upload", handler.GetMediaUploadSettings)

	req := httptest.NewRequest(http.MethodGet, "/admin/settings/media-upload", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got adminMediaUploadSettingsResponse
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	if got.Data != settings.DefaultMediaUploadSetting() {
		t.Fatalf("media upload settings = %+v, want %+v", got.Data, settings.DefaultMediaUploadSetting())
	}
}

func TestAdminMediaUploadSettingsUpdatePersists(t *testing.T) {

	db := newUserRepoTestDB(t)
	handler := newAdminHandlerForRuntimeSettings(db)
	router := gin.New()
	router.GET("/admin/settings/media-upload", handler.GetMediaUploadSettings)
	router.PUT("/admin/settings/media-upload", handler.UpdateMediaUploadSettings)

	req := httptest.NewRequest(http.MethodPut, "/admin/settings/media-upload", strings.NewReader(`{"upload_max_concurrent":4}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var updated adminMediaUploadSettingsResponse
	if err := json.NewDecoder(rec.Body).Decode(&updated); err != nil {
		t.Fatalf("decode update response failed: %v", err)
	}
	want := settings.MediaUploadSetting{UploadMaxConcurrent: 4}
	if updated.Data != want {
		t.Fatalf("updated media upload settings = %+v, want %+v", updated.Data, want)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/admin/settings/media-upload", nil)
	getRec := httptest.NewRecorder()
	router.ServeHTTP(getRec, getReq)

	var got adminMediaUploadSettingsResponse
	if err := json.NewDecoder(getRec.Body).Decode(&got); err != nil {
		t.Fatalf("decode get response failed: %v", err)
	}
	if got.Data != want {
		t.Fatalf("persisted media upload settings = %+v, want %+v", got.Data, want)
	}
}

func TestAdminMediaUploadSettingsRejectsInvalidPayload(t *testing.T) {

	db := newUserRepoTestDB(t)
	handler := newAdminHandlerForRuntimeSettings(db)
	router := gin.New()
	router.PUT("/admin/settings/media-upload", handler.UpdateMediaUploadSettings)

	for _, body := range []string{
		`{"upload_max_concurrent":0}`,
		`{"upload_max_concurrent":65}`,
	} {
		req := httptest.NewRequest(http.MethodPut, "/admin/settings/media-upload", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "upload_max_concurrent 必须在 1 到 64 之间") {
			t.Fatalf("body = %s, want upload_max_concurrent validation message", rec.Body.String())
		}
	}
}
