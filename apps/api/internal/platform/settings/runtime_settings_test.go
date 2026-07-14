package settings

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"ideaboard/api/internal/platform/adminaction"
	"ideaboard/api/internal/platform/validation"
	"ideaboard/api/internal/testdb"
)

func TestRuntimeSettingsOperationalUpdatePersistsAuditAndTracksRestartRequired(t *testing.T) {
	ctx := context.Background()
	db := testdb.Open(t)
	service := NewRuntimeSettingsService(NewSystemConfigRepository(db))
	service.SetEffectiveOperationalSettings(DefaultTaskRuntimeSetting(), DefaultEventRetentionSetting())
	action, err := adminaction.New(42, "  capacity tuning  ")
	if err != nil {
		t.Fatal(err)
	}

	updated, err := service.UpdateOperationalSettings(ctx, OperationalSettings{
		TaskRuntime: TaskRuntimeSetting{
			WorkerConcurrency:      12,
			TaskTimeoutSeconds:     240,
			NoCapacityRetrySeconds: 9,
			MaxRetry:               31,
		},
		OutboundPolicy: OutboundPolicySetting{AllowHosts: []string{"  INTERNAL.example.com:8443 ", "10.0.0.8", "internal.example.com:8443"}},
		EventRetention: EventRetentionSetting{NATSRetentionDays: 30},
	}, action)
	if err != nil {
		t.Fatalf("update operational settings failed: %v", err)
	}

	wantTask := TaskRuntimeSetting{WorkerConcurrency: 12, TaskTimeoutSeconds: 240, NoCapacityRetrySeconds: 9, MaxRetry: 31}
	wantOutbound := OutboundPolicySetting{AllowHosts: []string{"internal.example.com:8443", "10.0.0.8"}}
	wantRetention := EventRetentionSetting{NATSRetentionDays: 30}
	if updated.TaskRuntime.Configured != wantTask || updated.TaskRuntime.Effective != DefaultTaskRuntimeSetting() || !updated.TaskRuntime.RestartRequired {
		t.Fatalf("task runtime state = %+v", updated.TaskRuntime)
	}
	if !equalStringSlices(updated.OutboundPolicy.Configured.AllowHosts, wantOutbound.AllowHosts) || !equalStringSlices(updated.OutboundPolicy.Effective.AllowHosts, wantOutbound.AllowHosts) || updated.OutboundPolicy.RestartRequired {
		t.Fatalf("outbound policy state = %+v", updated.OutboundPolicy)
	}
	if updated.EventRetention.Configured != wantRetention || updated.EventRetention.Effective != DefaultEventRetentionSetting() || !updated.EventRetention.RestartRequired {
		t.Fatalf("event retention state = %+v", updated.EventRetention)
	}

	var actionType string
	var actorID int64
	var reason string
	var metadataRaw []byte
	if err := db.QueryRowContext(ctx, `
SELECT action_type, actor_id, reason, metadata
FROM system_settings_admin_actions
ORDER BY id DESC
LIMIT 1`).Scan(&actionType, &actorID, &reason, &metadataRaw); err != nil {
		t.Fatalf("read operational settings audit: %v", err)
	}
	if actionType != "update_operational_settings" || actorID != 42 || reason != "capacity tuning" {
		t.Fatalf("audit = type=%q actor=%d reason=%q", actionType, actorID, reason)
	}
	var metadata map[string]any
	if err := json.Unmarshal(metadataRaw, &metadata); err != nil {
		t.Fatalf("decode audit metadata: %v", err)
	}
	if metadata["task_runtime"] == nil || metadata["outbound_policy"] == nil || metadata["event_retention"] == nil {
		t.Fatalf("audit metadata = %#v, want all operational groups", metadata)
	}
}

func TestRuntimeSettingsOperationalUpdateRejectsInvalidAdminAction(t *testing.T) {
	service := NewRuntimeSettingsService(nil)
	_, err := service.UpdateOperationalSettings(t.Context(), DefaultOperationalSettings(), adminaction.Metadata{})
	if err == nil || !validation.Is(err) || !errors.Is(err, validation.Error("管理员操作信息无效")) {
		t.Fatalf("UpdateOperationalSettings() error = %v, want validation error", err)
	}
}

func TestNormalizeOperationalSettingsRejectsInvalidInputs(t *testing.T) {
	valid := DefaultOperationalSettings()
	tests := []struct {
		name    string
		setting OperationalSettings
	}{
		{name: "worker concurrency", setting: func() OperationalSettings { value := valid; value.TaskRuntime.WorkerConcurrency = 0; return value }()},
		{name: "task timeout", setting: func() OperationalSettings {
			value := valid
			value.TaskRuntime.TaskTimeoutSeconds = 31_536_001
			return value
		}()},
		{name: "capacity retry", setting: func() OperationalSettings { value := valid; value.TaskRuntime.NoCapacityRetrySeconds = 0; return value }()},
		{name: "max retry", setting: func() OperationalSettings { value := valid; value.TaskRuntime.MaxRetry = 0; return value }()},
		{name: "retention", setting: func() OperationalSettings {
			value := valid
			value.EventRetention.NATSRetentionDays = 3661
			return value
		}()},
		{name: "host scheme", setting: func() OperationalSettings {
			value := valid
			value.OutboundPolicy.AllowHosts = []string{"https://internal.example.com"}
			return value
		}()},
		{name: "host wildcard", setting: func() OperationalSettings {
			value := valid
			value.OutboundPolicy.AllowHosts = []string{"*.example.com"}
			return value
		}()},
		{name: "too many hosts", setting: func() OperationalSettings {
			value := valid
			value.OutboundPolicy.AllowHosts = make([]string, maxOutboundAllowHosts+1)
			return value
		}()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := NormalizeOperationalSettings(tt.setting); err == nil {
				t.Fatalf("NormalizeOperationalSettings(%+v) succeeded, want error", tt.setting)
			}
		})
	}
}

func TestRuntimeOperationalSettingsUseCallerFallbackWhenConfigsAreMissing(t *testing.T) {
	ctx := context.Background()
	db := testdb.Open(t)
	service := NewRuntimeSettingsService(NewSystemConfigRepository(db))
	if _, err := db.ExecContext(ctx, `DELETE FROM system_configs WHERE config_key = ANY($1::text[])`, []string{
		taskRuntimeWorkerConcurrencyKey,
		taskRuntimeTaskTimeoutSecondsKey,
		taskRuntimeNoCapacityRetrySecondsKey,
		taskRuntimeMaxRetryKey,
		eventRetentionNATSDaysKey,
	}); err != nil {
		t.Fatalf("delete operational defaults: %v", err)
	}

	taskFallback := TaskRuntimeSetting{WorkerConcurrency: 7, TaskTimeoutSeconds: 321, NoCapacityRetrySeconds: 11, MaxRetry: 19}
	retentionFallback := EventRetentionSetting{NATSRetentionDays: 14}
	if got, err := service.ResolveTaskRuntime(ctx, taskFallback); err != nil || got != taskFallback {
		t.Fatalf("task fallback = %+v, err=%v", got, err)
	}
	if got, err := service.ResolveEventRetention(ctx, retentionFallback); err != nil || got != retentionFallback {
		t.Fatalf("retention fallback = %+v, err=%v", got, err)
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

	branding, err := service.UpdateBranding(ctx, BrandingSetting{ProjectName: "  Design Ops  ", LogoURL: "  /uploads/site/assets/logo.png  "})
	if err != nil {
		t.Fatalf("update branding failed: %v", err)
	}
	if branding != (BrandingSetting{ProjectName: "Design Ops", LogoURL: "/uploads/site/assets/logo.png"}) {
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

	googleNative, err := service.UpdateGoogleNativeGeneration(ctx, GoogleNativeGenerationSetting{Mode: GoogleNativeGenerationModeCanary, CanaryPercent: 25})
	if err != nil {
		t.Fatalf("update google native generation failed: %v", err)
	}
	if googleNative != (GoogleNativeGenerationSetting{Mode: GoogleNativeGenerationModeCanary, CanaryPercent: 25}) {
		t.Fatalf("google native generation = %+v, want persisted payload", googleNative)
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

	gotGoogleNative, err := service.GetGoogleNativeGeneration(ctx)
	if err != nil {
		t.Fatalf("get google native generation failed: %v", err)
	}
	if gotGoogleNative != googleNative {
		t.Fatalf("google native generation readback = %+v, want %+v", gotGoogleNative, googleNative)
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
	if got, err := service.GetGoogleNativeGeneration(ctx); err != nil || got != DefaultGoogleNativeGenerationSetting() {
		t.Fatalf("google native generation default = %+v, err=%v", got, err)
	}
}

func TestEvaluateGoogleNativeGenerationModes(t *testing.T) {
	enabled := EvaluateGoogleNativeGeneration(GoogleNativeGenerationSetting{
		Mode:          GoogleNativeGenerationModeEnabled,
		CanaryPercent: 0,
	}, 42)
	if !enabled.Allowed {
		t.Fatalf("enabled evaluation = %+v, want allowed", enabled)
	}

	disabled := EvaluateGoogleNativeGeneration(GoogleNativeGenerationSetting{
		Mode:          GoogleNativeGenerationModeDisabled,
		CanaryPercent: 100,
	}, 42)
	if disabled.Allowed || disabled.Message == "" {
		t.Fatalf("disabled evaluation = %+v, want blocked with message", disabled)
	}

	canaryZero := EvaluateGoogleNativeGeneration(GoogleNativeGenerationSetting{
		Mode:          GoogleNativeGenerationModeCanary,
		CanaryPercent: 0,
	}, 42)
	if canaryZero.Allowed {
		t.Fatalf("canary 0%% evaluation = %+v, want blocked", canaryZero)
	}

	canaryFull := EvaluateGoogleNativeGeneration(GoogleNativeGenerationSetting{
		Mode:          GoogleNativeGenerationModeCanary,
		CanaryPercent: 100,
	}, 42)
	if !canaryFull.Allowed {
		t.Fatalf("canary 100%% evaluation = %+v, want allowed", canaryFull)
	}

	included, excluded := false, false
	for userID := int64(1); userID <= 200; userID++ {
		evaluation := EvaluateGoogleNativeGeneration(GoogleNativeGenerationSetting{
			Mode:          GoogleNativeGenerationModeCanary,
			CanaryPercent: 50,
		}, userID)
		if evaluation.Bucket < 0 || evaluation.Bucket >= 100 {
			t.Fatalf("bucket = %d, want [0,100)", evaluation.Bucket)
		}
		if evaluation.Allowed != (evaluation.Bucket < 50) {
			t.Fatalf("evaluation = %+v, want allowed iff bucket < percent", evaluation)
		}
		included = included || evaluation.Allowed
		excluded = excluded || !evaluation.Allowed
	}
	if !included || !excluded {
		t.Fatalf("canary 50%% should include and exclude users, included=%v excluded=%v", included, excluded)
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
