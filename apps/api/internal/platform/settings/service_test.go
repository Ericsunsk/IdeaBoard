package settings

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"ideaboard/api/internal/platform/errorrender"
	"ideaboard/api/internal/testdb"
)

func TestRuntimeSettingsOutboundPolicyUpdatePersistsAuditAndAppliesImmediately(t *testing.T) {
	ctx := context.Background()
	db := testdb.Open(t)
	service := NewRuntimeSettingsService(NewSystemConfigRepository(db))

	updated, err := service.UpdateOutboundPolicy(ctx, OutboundPolicySetting{
		AllowHosts: []string{"  INTERNAL.example.com:08443 ", "10.0.0.8", "internal.example.com:8443"},
	}, 42)
	if err != nil {
		t.Fatalf("update outbound policy failed: %v", err)
	}

	wantOutbound := OutboundPolicySetting{AllowHosts: []string{"internal.example.com:8443", "10.0.0.8"}}
	if !equalStringSlices(updated.AllowHosts, wantOutbound.AllowHosts) {
		t.Fatalf("outbound policy = %+v", updated)
	}
	effective, err := service.GetOutboundPolicy(ctx)
	if err != nil || !equalStringSlices(effective.AllowHosts, wantOutbound.AllowHosts) {
		t.Fatalf("effective outbound policy = %+v, err=%v", effective, err)
	}

	var actionType string
	var actorID int64
	var metadataRaw []byte
	if err := db.QueryRow(ctx, `
SELECT action_type, actor_id, metadata
FROM system_settings_admin_actions
ORDER BY id DESC
LIMIT 1`).Scan(&actionType, &actorID, &metadataRaw); err != nil {
		t.Fatalf("read outbound policy audit: %v", err)
	}
	if actionType != "update_outbound_policy" || actorID != 42 {
		t.Fatalf("audit = type=%q actor=%d", actionType, actorID)
	}
	var metadata OutboundPolicySetting
	if err := json.Unmarshal(metadataRaw, &metadata); err != nil {
		t.Fatalf("decode audit metadata: %v", err)
	}
	if !equalStringSlices(metadata.AllowHosts, wantOutbound.AllowHosts) {
		t.Fatalf("audit metadata = %#v", metadata)
	}
}

func TestRuntimeSettingsOutboundPolicyUpdateRejectsInvalidActor(t *testing.T) {
	service := NewRuntimeSettingsService(nil)
	_, err := service.UpdateOutboundPolicy(t.Context(), DefaultOutboundPolicySetting(), 0)
	if err == nil || !errorrender.IsValidationError(err) || !errors.Is(err, errorrender.ValidationError("管理员身份无效")) {
		t.Fatalf("UpdateOutboundPolicy() error = %v, want validation error", err)
	}
}

func TestNormalizeOutboundPolicyAcceptsURLForms(t *testing.T) {
	normalized, err := NormalizeOutboundPolicySetting(OutboundPolicySetting{AllowHosts: []string{
		"https://internal.example.com:8443/v1",
		"HTTP://Internal.Example.COM./health",
		"  internal.example.com:8443  ",
		"internal.example.com.",
		"10.0.0.8",
	}})
	if err != nil {
		t.Fatalf("NormalizeOutboundPolicySetting() error = %v", err)
	}
	want := []string{"internal.example.com:8443", "internal.example.com", "10.0.0.8"}
	if !equalStringSlices(normalized.AllowHosts, want) {
		t.Fatalf("outbound policy = %+v, want %+v", normalized.AllowHosts, want)
	}
}

func TestNormalizeOutboundPolicyRejectsInvalidInputs(t *testing.T) {
	tests := []struct {
		name    string
		setting OutboundPolicySetting
	}{
		{name: "host userinfo", setting: OutboundPolicySetting{AllowHosts: []string{"https://user@internal.example.com"}}},
		{name: "host wildcard", setting: OutboundPolicySetting{AllowHosts: []string{"*.example.com"}}},
		{name: "too many hosts", setting: OutboundPolicySetting{AllowHosts: make([]string, maxOutboundAllowHosts+1)}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := NormalizeOutboundPolicySetting(tt.setting); err == nil {
				t.Fatalf("NormalizeOutboundPolicySetting(%+v) succeeded, want error", tt.setting)
			}
		})
	}
}

func TestRuntimeSettingsOutboundPolicyUsesClosedDefaultWhenConfigIsMissing(t *testing.T) {
	ctx := context.Background()
	db := testdb.Open(t)
	service := NewRuntimeSettingsService(NewSystemConfigRepository(db))
	if _, err := db.Exec(ctx, `DELETE FROM system_configs WHERE config_key = $1`, outboundPolicyAllowHostsKey); err != nil {
		t.Fatalf("delete outbound policy default: %v", err)
	}

	got, err := service.GetOutboundPolicy(ctx)
	if err != nil || len(got.AllowHosts) != 0 {
		t.Fatalf("outbound policy fallback = %+v, err=%v", got, err)
	}
}

func TestRuntimeSettingsOutboundPolicyFailsClosedWhenConfigIsInvalid(t *testing.T) {
	ctx := context.Background()
	db := testdb.Open(t)
	service := NewRuntimeSettingsService(NewSystemConfigRepository(db))
	if _, err := db.Exec(ctx, `
UPDATE system_configs
SET value = 'not-json'
WHERE config_key = $1`, outboundPolicyAllowHostsKey); err != nil {
		t.Fatalf("corrupt outbound policy: %v", err)
	}

	got, err := service.GetOutboundPolicy(ctx)
	if err == nil || len(got.AllowHosts) != 0 {
		t.Fatalf("outbound policy = %+v, err=%v, want closed policy with error", got, err)
	}
}

func equalStringSlices(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

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

	branding, err := service.UpdateBranding(ctx, BrandingSetting{ProjectName: "  Design Ops  ", LogoURL: "  uploads/site/assets/logo_abcdef1234567890.png  "})
	if err != nil {
		t.Fatalf("update branding failed: %v", err)
	}
	if branding != (BrandingSetting{ProjectName: "Design Ops", LogoURL: "uploads/site/assets/logo_abcdef1234567890.png"}) {
		t.Fatalf("branding = %+v, want normalized values", branding)
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
	if got, err := service.GetMediaUpload(ctx); err != nil || got != DefaultMediaUploadSetting() {
		t.Fatalf("media upload default = %+v, err=%v", got, err)
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

func TestNormalizeBrandingLogo(t *testing.T) {
	for _, value := range []string{"", "uploads/site/assets/logo_abcdef1234567890.png", "https://cdn.example.com/logo.png"} {
		if got, err := normalizeBrandingLogo(value); err != nil || got != value {
			t.Fatalf("normalize(%q) = %q, %v", value, got, err)
		}
	}
	for _, value := range []string{"/api/v1/public/branding/logo", "/uploads/site/assets/logo_abcdef1234567890.png", "javascript:alert(1)", "uploads/site/assets/../secret.png", "uploads/site/assets/", "ftp://example.com/logo.png"} {
		if _, err := normalizeBrandingLogo(value); err == nil {
			t.Fatalf("accepted %q", value)
		}
	}
}

func TestRuntimeSettingsBrandingLogoKeyspace(t *testing.T) {
	ctx := context.Background()
	service := NewRuntimeSettingsService(NewSystemConfigRepository(testdb.Open(t)))
	for _, value := range []string{"uploads/site/assets/logo_abcdef1234567890.png", "uploads/site/assets/logo_1111111111111111.png", "https://cdn.example.com/logo.png", ""} {
		updated, err := service.UpdateBranding(ctx, BrandingSetting{ProjectName: "Acme", LogoURL: value})
		if err != nil || updated.LogoURL != value {
			t.Fatalf("update %q: %+v, %v", value, updated, err)
		}
		got, err := service.GetBranding(ctx)
		if err != nil || got != updated {
			t.Fatalf("read %q: %+v, %v", value, got, err)
		}
	}
}
