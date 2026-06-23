package settings

import (
	"context"
	"testing"

	"ideaboard/api/internal/testdb"
)

func TestRuntimeSettingsServiceUpdatesAndReadsTypedSettings(t *testing.T) {
	ctx := context.Background()
	db := testdb.Open(t)
	repo := NewSystemConfigRepository(db)
	service := NewRuntimeSettingsService(repo)

	registration, err := service.UpdateRegistration(ctx, RegistrationSetting{Enabled: true})
	if err != nil {
		t.Fatalf("update registration failed: %v", err)
	}
	if registration != (RegistrationSetting{Enabled: true}) {
		t.Fatalf("registration = %+v, want enabled", registration)
	}

	rateLimit, err := service.UpdateRateLimit(ctx, RateLimitSetting{WindowSeconds: 120, MaxRequests: 60})
	if err != nil {
		t.Fatalf("update rate limit failed: %v", err)
	}
	if rateLimit != (RateLimitSetting{WindowSeconds: 120, MaxRequests: 60}) {
		t.Fatalf("rate limit = %+v, want persisted payload", rateLimit)
	}

	branding, err := service.UpdateBranding(ctx, BrandingSetting{ProjectName: "  Design Ops  ", LogoURL: "  /uploads/admin/logo.png  "})
	if err != nil {
		t.Fatalf("update branding failed: %v", err)
	}
	if branding != (BrandingSetting{ProjectName: "Design Ops", LogoURL: "/uploads/admin/logo.png"}) {
		t.Fatalf("branding = %+v, want normalized values", branding)
	}

	taskMonitor := TaskMonitorSetting{
		ListPageSize:           24,
		AutoRefreshEnabled:     false,
		RefreshIntervalSeconds: 10,
		ResultPreviewEnabled:   false,
	}
	if _, err := service.UpdateTaskMonitor(ctx, taskMonitor); err != nil {
		t.Fatalf("update task monitor failed: %v", err)
	}

	publicURL, err := service.UpdatePublicURL(ctx, PublicURLSetting{PublicBaseURL: "  https://app.example.com/  ", PublicAssetBaseURL: "  https://api.example.com/uploads/  "})
	if err != nil {
		t.Fatalf("update public URL failed: %v", err)
	}
	if publicURL != (PublicURLSetting{PublicBaseURL: "https://app.example.com", PublicAssetBaseURL: "https://api.example.com/uploads"}) {
		t.Fatalf("public URL = %+v, want normalized values", publicURL)
	}

	mediaUpload, err := service.UpdateMediaUpload(ctx, MediaUploadSetting{UploadMaxConcurrent: 12})
	if err != nil {
		t.Fatalf("update media upload failed: %v", err)
	}
	if mediaUpload != (MediaUploadSetting{UploadMaxConcurrent: 12}) {
		t.Fatalf("media upload = %+v, want persisted payload", mediaUpload)
	}

	gotRegistration, err := service.GetRegistration(ctx)
	if err != nil {
		t.Fatalf("get registration failed: %v", err)
	}
	if gotRegistration != registration {
		t.Fatalf("registration readback = %+v, want %+v", gotRegistration, registration)
	}

	gotRateLimit, err := service.GetRateLimit(ctx)
	if err != nil {
		t.Fatalf("get rate limit failed: %v", err)
	}
	if gotRateLimit != rateLimit {
		t.Fatalf("rate limit readback = %+v, want %+v", gotRateLimit, rateLimit)
	}

	gotBranding, err := service.GetBranding(ctx)
	if err != nil {
		t.Fatalf("get branding failed: %v", err)
	}
	if gotBranding != branding {
		t.Fatalf("branding readback = %+v, want %+v", gotBranding, branding)
	}

	gotTaskMonitor, err := service.GetTaskMonitor(ctx)
	if err != nil {
		t.Fatalf("get task monitor failed: %v", err)
	}
	if gotTaskMonitor != taskMonitor {
		t.Fatalf("task monitor readback = %+v, want %+v", gotTaskMonitor, taskMonitor)
	}

	gotPublicURL, err := service.GetPublicURL(ctx)
	if err != nil {
		t.Fatalf("get public URL failed: %v", err)
	}
	if gotPublicURL != publicURL {
		t.Fatalf("public URL readback = %+v, want %+v", gotPublicURL, publicURL)
	}

	gotMediaUpload, err := service.GetMediaUpload(ctx)
	if err != nil {
		t.Fatalf("get media upload failed: %v", err)
	}
	if gotMediaUpload != mediaUpload {
		t.Fatalf("media upload readback = %+v, want %+v", gotMediaUpload, mediaUpload)
	}
}

func TestRuntimeSettingsServiceDefaultsWhenConfigsAreMissing(t *testing.T) {
	ctx := context.Background()
	db := testdb.Open(t)
	service := NewRuntimeSettingsService(NewSystemConfigRepository(db))

	if got, err := service.GetRegistration(ctx); err != nil || got != DefaultRegistrationSetting() {
		t.Fatalf("registration default = %+v, err=%v", got, err)
	}
	if got, err := service.GetRateLimit(ctx); err != nil || got != DefaultRateLimitSetting() {
		t.Fatalf("rate limit default = %+v, err=%v", got, err)
	}
	if got, err := service.GetBranding(ctx); err != nil || got != DefaultBrandingSetting() {
		t.Fatalf("branding default = %+v, err=%v", got, err)
	}
	if got, err := service.GetTaskMonitor(ctx); err != nil || got != DefaultTaskMonitorSetting() {
		t.Fatalf("task monitor default = %+v, err=%v", got, err)
	}
	if got, err := service.GetPublicURL(ctx); err != nil || got != DefaultPublicURLSetting() {
		t.Fatalf("public URL default = %+v, err=%v", got, err)
	}
	if got, err := service.GetMediaUpload(ctx); err != nil || got != DefaultMediaUploadSetting() {
		t.Fatalf("media upload default = %+v, err=%v", got, err)
	}
}

func TestRuntimeSettingsPublicURLUsesBlankDefaultAndDBOverride(t *testing.T) {
	ctx := context.Background()
	db := testdb.Open(t)
	service := NewRuntimeSettingsService(NewSystemConfigRepository(db))

	got, err := service.GetPublicURL(ctx)
	if err != nil {
		t.Fatalf("get public URL default failed: %v", err)
	}
	if got != DefaultPublicURLSetting() {
		t.Fatalf("public URL default = %+v, want blank default %+v", got, DefaultPublicURLSetting())
	}

	updated, err := service.UpdatePublicURL(ctx, PublicURLSetting{
		PublicBaseURL:      "https://db-app.example.com/",
		PublicAssetBaseURL: "",
	})
	if err != nil {
		t.Fatalf("update public URL failed: %v", err)
	}
	wantUpdated := PublicURLSetting{PublicBaseURL: "https://db-app.example.com", PublicAssetBaseURL: ""}
	if updated != wantUpdated {
		t.Fatalf("updated public URL = %+v, want %+v", updated, wantUpdated)
	}

	got, err = service.GetPublicURL(ctx)
	if err != nil {
		t.Fatalf("get public URL override failed: %v", err)
	}
	if got != wantUpdated {
		t.Fatalf("public URL override = %+v, want %+v", got, wantUpdated)
	}
}

func TestNormalizePublicURLSettingRejectsNonBaseURLs(t *testing.T) {
	tests := []PublicURLSetting{
		{PublicBaseURL: "https://user:pass@app.example.com", PublicAssetBaseURL: ""},
		{PublicBaseURL: "https://app.example.com?token=1", PublicAssetBaseURL: ""},
		{PublicBaseURL: "https://app.example.com#section", PublicAssetBaseURL: ""},
		{PublicBaseURL: "", PublicAssetBaseURL: "https://assets.example.com?token=1"},
		{PublicBaseURL: "", PublicAssetBaseURL: "https://assets.example.com#uploads"},
	}

	for _, setting := range tests {
		if _, err := NormalizePublicURLSetting(setting); err == nil {
			t.Fatalf("NormalizePublicURLSetting(%+v) succeeded, want error", setting)
		}
	}
}

func TestResolvePublicBaseURLUsesRequestOrigin(t *testing.T) {
	got := ResolvePublicBaseURL(context.Background(), nil, "http", "request.example")
	if got != "http://request.example" {
		t.Fatalf("public base URL = %q, want request origin", got)
	}
}

func TestRuntimeSettingsMediaUploadUsesDefaultAndDBOverride(t *testing.T) {
	ctx := context.Background()
	db := testdb.Open(t)
	service := NewRuntimeSettingsService(NewSystemConfigRepository(db))

	got, err := service.GetMediaUpload(ctx)
	if err != nil {
		t.Fatalf("get media upload default failed: %v", err)
	}
	if got != DefaultMediaUploadSetting() {
		t.Fatalf("media upload default = %+v, want %+v", got, DefaultMediaUploadSetting())
	}

	updated, err := service.UpdateMediaUpload(ctx, MediaUploadSetting{UploadMaxConcurrent: 4})
	if err != nil {
		t.Fatalf("update media upload failed: %v", err)
	}
	if updated != (MediaUploadSetting{UploadMaxConcurrent: 4}) {
		t.Fatalf("updated media upload = %+v, want 4", updated)
	}

	got, err = service.GetMediaUpload(ctx)
	if err != nil {
		t.Fatalf("get media upload override failed: %v", err)
	}
	if got != updated {
		t.Fatalf("media upload override = %+v, want %+v", got, updated)
	}
}

func TestNormalizeMediaUploadSettingRejectsOutOfRangeValues(t *testing.T) {
	for _, setting := range []MediaUploadSetting{
		{UploadMaxConcurrent: 0},
		{UploadMaxConcurrent: 65},
	} {
		if _, err := NormalizeMediaUploadSetting(setting); err == nil {
			t.Fatalf("NormalizeMediaUploadSetting(%+v) succeeded, want error", setting)
		}
	}
}
