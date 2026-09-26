package httpapi

// Control Plane extended-family tests (#809-close): authorization
// matrix, audit record fields, header-only idempotency replay/conflict,
// and reason-required enforcement for the gateway extras, brand-logo
// overrides + proxy, OAuth providers, reward codes + redemptions,
// branding logo upload, and monitor stream routes, plus happy-path
// coverage for the dedicated OAuth/upload handlers and the workspace
// task adapter fail-closed seams.

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"ideaboard/api/internal/identity/auth"
	"ideaboard/api/internal/media/storage/ports"
	"ideaboard/api/internal/platform/contract"
	rewardhttp "ideaboard/api/internal/rewards/interfaces/http"
	rewardports "ideaboard/api/internal/rewards/ports"
)

// --- reward fakes (single-sourced legacy handlers under the control
// guard; responses translated into the contract envelope at the
// boundary) ---

type fakeRewardAdminCommands struct {
	creates int
	updates int
	toggles int
	deletes int
}

func (f *fakeRewardAdminCommands) CreateRewardCode(_ context.Context, cmd rewardports.CreateRewardCodeCommand) (rewardports.CreatedRewardCode, error) {
	f.creates++
	now := time.Now().UTC()
	return rewardports.CreatedRewardCode{
		RewardCode: rewardports.RewardCode{
			ID: 7, DisplayName: cmd.DisplayName, MaskedCodeHint: "****-AB12",
			CreditAmount: cmd.CreditAmount, MaxRedemptions: cmd.MaxRedemptions,
			Enabled: true, Status: rewardports.RewardCodeStatusEnabled,
			CreatedAt: now, UpdatedAt: now,
		},
		Plaintext: "SECRET-CODE",
	}, nil
}

func (f *fakeRewardAdminCommands) UpdateRewardCode(_ context.Context, cmd rewardports.UpdateRewardCodeCommand) (rewardports.UpdatedRewardCode, error) {
	f.updates++
	now := time.Now().UTC()
	return rewardports.UpdatedRewardCode{
		RewardCode: rewardports.RewardCode{
			ID: cmd.RewardCodeID, DisplayName: cmd.DisplayName,
			CreditAmount: cmd.CreditAmount, MaxRedemptions: cmd.MaxRedemptions,
			MaskedCodeHint: "****-AB12", Enabled: true,
			Status:    rewardports.RewardCodeStatusEnabled,
			CreatedAt: now, UpdatedAt: now,
		},
	}, nil
}

func (f *fakeRewardAdminCommands) SetRewardCodeEnabled(_ context.Context, cmd rewardports.SetRewardCodeEnabledCommand) (rewardports.RewardCode, error) {
	f.toggles++
	now := time.Now().UTC()
	status := rewardports.RewardCodeStatusEnabled
	if !cmd.Enabled {
		status = rewardports.RewardCodeStatusDisabled
	}
	return rewardports.RewardCode{
		ID: cmd.RewardCodeID, DisplayName: "code", MaskedCodeHint: "****-AB12",
		Enabled: cmd.Enabled, Status: status, CreatedAt: now, UpdatedAt: now,
	}, nil
}

func (f *fakeRewardAdminCommands) DeleteRewardCode(_ context.Context, cmd rewardports.DeleteRewardCodeCommand) (rewardports.DeletedRewardCode, error) {
	f.deletes++
	return rewardports.DeletedRewardCode{RewardCodeID: cmd.RewardCodeID}, nil
}

type fakeRewardAdminReads struct{}

func (fakeRewardAdminReads) SearchRewardCodes(_ context.Context, input rewardports.SearchRewardCodesInput) (rewardports.RewardCodePage, error) {
	now := time.Now().UTC()
	return rewardports.RewardCodePage{
		RewardCodes: []rewardports.RewardCode{{
			ID: 7, DisplayName: "code", MaskedCodeHint: "****-AB12",
			Enabled: true, Status: rewardports.RewardCodeStatusEnabled,
			CreatedAt: now, UpdatedAt: now,
		}},
		Total: 1,
	}, nil
}

func (fakeRewardAdminReads) ListAdminRedemptions(_ context.Context, input rewardports.ListAdminRedemptionsInput) (rewardports.AdminRedemptionPage, error) {
	return rewardports.AdminRedemptionPage{
		Redemptions: []rewardports.AdminRedemptionSummary{{
			ID: 3, RewardCodeID: input.RewardCodeID, UserID: controlUserAID,
			UserPublicID: testUUIDForUser(controlUserAID), DisplayName: "user-21",
			CreditedAmount: 50, RedeemedAt: time.Now().UTC(),
		}},
	}, nil
}

func (fakeRewardAdminReads) GetAdminRedemption(_ context.Context, input rewardports.GetAdminRedemptionInput) (rewardports.AdminRedemptionDetail, error) {
	return rewardports.AdminRedemptionDetail{
		ID: input.RedemptionID, RewardCodeID: input.RewardCodeID, UserID: controlUserAID,
		UserPublicID: testUUIDForUser(controlUserAID), DisplayName: "user-21",
		CreditedAmount: 50, RedeemedAt: time.Now().UTC(),
		CreditTransactionID: "ctxn_test", MaskedCodeHint: "****-AB12",
	}, nil
}

// fakeRewardIdentityProjection resolves redemption owners for the
// proxied reward read handlers (mirrors the production projector).
type fakeRewardIdentityProjection struct{}

func (fakeRewardIdentityProjection) ResolveIdentity(_ context.Context, userID int64) (rewardports.IdentityProjection, error) {
	return rewardports.IdentityProjection{PublicID: testUUIDForUser(userID), DisplayName: "user"}, nil
}

func (fakeRewardIdentityProjection) ResolveIdentities(_ context.Context, userIDs []int64) (map[int64]rewardports.IdentityProjection, error) {
	identities := make(map[int64]rewardports.IdentityProjection, len(userIDs))
	for _, userID := range userIDs {
		identities[userID] = rewardports.IdentityProjection{PublicID: testUUIDForUser(userID), DisplayName: "user"}
	}
	return identities, nil
}

// fakeControlBrandingUpload stores branding logos in memory for upload
// tests (no storage backend).
type fakeControlBrandingUpload struct {
	calls int
}

func (f *fakeControlBrandingUpload) UploadRefWithOptions(_ context.Context, file *multipart.FileHeader, _ ports.StorageStoreOptions) (*ports.StoredUploadRef, error) {
	f.calls++
	return &ports.StoredUploadRef{
		URL: "/uploads/site/assets/logo.png", StorageKey: "uploads/site/assets/logo.png",
		MIMEType: "image/png", Width: 120, Height: 40, SizeBytes: file.Size,
	}, nil
}

// --- OAuth + upload fakes ---

type fakeControlOAuth struct {
	caps        *auth.SocialProviderCapabilities
	summary     *auth.SocialProviderSettingsSummary
	getErr      error
	updateErr   error
	capsErr     error
	lastUpdate  auth.SocialProviderSettingsUpdate
	lastProvide string
}

func (f *fakeControlOAuth) GetSocialProviderCapabilities(context.Context) (*auth.SocialProviderCapabilities, error) {
	if f.capsErr != nil {
		return nil, f.capsErr
	}
	if f.caps != nil {
		return f.caps, nil
	}
	return &auth.SocialProviderCapabilities{Social: map[string]bool{"google": true, "discord": true}}, nil
}

func (f *fakeControlOAuth) GetSocialProviderSettings(_ context.Context, provider string) (*auth.SocialProviderSettingsSummary, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	if f.summary != nil {
		return f.summary, nil
	}
	return &auth.SocialProviderSettingsSummary{Provider: provider, Enabled: true, Configured: true}, nil
}

func (f *fakeControlOAuth) UpdateSocialProviderSettings(_ context.Context, provider string, input auth.SocialProviderSettingsUpdate) (*auth.SocialProviderSettingsSummary, error) {
	if f.updateErr != nil {
		return nil, f.updateErr
	}
	f.lastProvide = provider
	f.lastUpdate = input
	return &auth.SocialProviderSettingsSummary{Provider: provider, Enabled: input.Enabled, Configured: true}, nil
}

func TestControlExtendedAuthMatrix(t *testing.T) {
	h := newControlHarness(t, nil)
	reads := []controlCall{
		{method: http.MethodGet, path: "/api/control/v1/platform/ai-gateway/detect-models"},
		{method: http.MethodGet, path: "/api/control/v1/platform/ai-gateway/models-reference?model_id=openai/gpt-4o"},
		{method: http.MethodGet, path: "/api/control/v1/platform/ai-gateway/brand-logo?provider=openai"},
		{method: http.MethodGet, path: "/api/control/v1/platform/ai-gateway/models-observed"},
		{method: http.MethodGet, path: "/api/control/v1/platform/brand-logo-overrides"},
		{method: http.MethodGet, path: "/api/control/v1/users/oauth-providers/google"},
		{method: http.MethodGet, path: "/api/control/v1/rewards/codes/7/redemptions"},
		{method: http.MethodGet, path: "/api/control/v1/rewards/codes/7/redemptions/3"},
		{method: http.MethodGet, path: "/api/control/v1/platform/monitor/stream"},
	}
	mutations := []controlCall{
		{method: http.MethodPost, path: "/api/control/v1/platform/ai-gateway/route-preview", body: `{"model_id":"openai/gpt-4o"}`},
		{method: http.MethodPost, path: "/api/control/v1/platform/ai-gateway/test-connection", body: `{}`,
			headers: controlMutationHeaders("ext-matrix-1", "matrix reason")},
		{method: http.MethodPut, path: "/api/control/v1/platform/brand-logo-overrides/oauth-provider/google", body: `{"logo_key":"google-oauth"}`,
			headers: controlMutationHeaders("ext-matrix-2", "matrix reason")},
		{method: http.MethodDelete, path: "/api/control/v1/platform/brand-logo-overrides/oauth-provider/google",
			headers: controlMutationHeaders("ext-matrix-3", "matrix reason")},
		{method: http.MethodPut, path: "/api/control/v1/users/oauth-providers/google", body: `{"enabled":true,"client_id":"x"}`,
			headers: controlMutationHeaders("ext-matrix-4", "matrix reason")},
		{method: http.MethodPost, path: "/api/control/v1/rewards/codes/search", body: `{}`},
		{method: http.MethodPost, path: "/api/control/v1/rewards/codes", body: `{"display_name":"x","code":"ABCDEFGH","credit_amount":10,"max_redemptions":0}`,
			headers: controlMutationHeaders("ext-matrix-5", "matrix reason")},
		{method: http.MethodPatch, path: "/api/control/v1/rewards/codes/7", body: `{"display_name":"y","credit_amount":5,"max_redemptions":0}`,
			headers: controlMutationHeaders("ext-matrix-6", "matrix reason")},
		{method: http.MethodPatch, path: "/api/control/v1/rewards/codes/7/enabled", body: `{"enabled":false}`,
			headers: controlMutationHeaders("ext-matrix-7", "matrix reason")},
		{method: http.MethodDelete, path: "/api/control/v1/rewards/codes/7",
			headers: controlMutationHeaders("ext-matrix-8", "matrix reason")},
	}

	t.Run("anonymous denied with 401", func(t *testing.T) {
		for _, call := range append(reads, mutations...) {
			status, _, payload := h.serve(call)
			if status != http.StatusUnauthorized {
				t.Fatalf("%s %s anon status = %d, want 401", call.method, call.path, status)
			}
			if code := controlErrorCode(t, payload); code != string(contract.CodeUnauthorized) {
				t.Fatalf("anon code = %q, want unauthorized", code)
			}
		}
	})

	t.Run("non-admin denied with 403", func(t *testing.T) {
		for _, call := range append(reads, mutations...) {
			call.userID = controlUserAID
			call.role = "user"
			status, _, _ := h.serve(call)
			if status != http.StatusForbidden {
				t.Fatalf("%s %s member status = %d, want 403", call.method, call.path, status)
			}
		}
	})

	t.Run("least privilege family grant is scoped", func(t *testing.T) {
		// control:platform only: gateway read allowed, rewards + oauth denied.
		status, _, _ := h.serve(controlCall{
			method: http.MethodGet, path: "/api/control/v1/platform/ai-gateway/detect-models",
			userID: controlOperatorID, role: "admin", caps: []string{ControlCapabilityPlatform},
		})
		if status != http.StatusNotImplemented {
			t.Fatalf("scoped gateway status = %d, want 501 (nil dep, guard passed)", status)
		}
		for _, call := range []controlCall{
			{method: http.MethodGet, path: "/api/control/v1/rewards/codes/7/redemptions",
				userID: controlOperatorID, role: "admin", caps: []string{ControlCapabilityPlatform}},
			{method: http.MethodGet, path: "/api/control/v1/users/oauth-providers/google",
				userID: controlOperatorID, role: "admin", caps: []string{ControlCapabilityPlatform}},
		} {
			status, _, payload := h.serve(call)
			if status != http.StatusForbidden {
				t.Fatalf("%s %s scoped status = %d, want 403", call.method, call.path, status)
			}
			if code := controlErrorCode(t, payload); code != string(contract.CodeForbidden) {
				t.Fatalf("scoped code = %q, want forbidden", code)
			}
		}
		// control:credits only: rewards search allowed (nil dep -> 501), gateway denied.
		status, _, _ = h.serve(controlCall{
			method: http.MethodPost, path: "/api/control/v1/rewards/codes/search", body: `{}`,
			userID: controlOperatorID, role: "admin", caps: []string{ControlCapabilityCredits},
		})
		if status != http.StatusNotImplemented {
			t.Fatalf("scoped rewards status = %d, want 501 (nil dep, guard passed)", status)
		}
	})
}

func TestControlExtendedNilDepsFailClosed(t *testing.T) {
	h := newControlHarness(t, nil)
	cases := []controlCall{
		{method: http.MethodGet, path: "/api/control/v1/platform/ai-gateway/detect-models"},
		{method: http.MethodGet, path: "/api/control/v1/platform/ai-gateway/models-reference?model_id=x"},
		{method: http.MethodGet, path: "/api/control/v1/platform/ai-gateway/brand-logo?provider=openai"},
		{method: http.MethodGet, path: "/api/control/v1/platform/ai-gateway/models-observed"},
		{method: http.MethodGet, path: "/api/control/v1/platform/brand-logo-overrides"},
		{method: http.MethodGet, path: "/api/control/v1/platform/monitor/stream"},
		{method: http.MethodGet, path: "/api/control/v1/rewards/codes/7/redemptions"},
	}
	for _, call := range cases {
		call.userID = controlOperatorID
		call.role = "admin"
		status, _, payload := h.serve(call)
		if status != http.StatusNotImplemented {
			t.Fatalf("%s %s status = %d, want 501", call.method, call.path, status)
		}
		if code := controlErrorCode(t, payload); code != string(contract.CodeContractNotImplemented) {
			t.Fatalf("code = %q, want contract_not_implemented", code)
		}
	}
	// Tasks + OAuth + upload have dedicated fail-closed codes (503).
	// (The harness always wires the Tasks fake, so nil-Tasks 503 is
	// covered by the adapter fail-closed test below, not here.)
	for _, call := range []controlCall{
		{method: http.MethodGet, path: "/api/control/v1/users/oauth-providers/google"},
	} {
		call.userID = controlOperatorID
		call.role = "admin"
		status, _, payload := h.serve(call)
		if status != http.StatusServiceUnavailable {
			t.Fatalf("%s %s status = %d, want 503", call.method, call.path, status)
		}
		if code := controlErrorCode(t, payload); code != string(contract.CodeServiceUnavailable) {
			t.Fatalf("code = %q, want service_unavailable", code)
		}
	}
}

func TestControlExtendedReasonAndIdempotency(t *testing.T) {
	h := newControlHarness(t, nil)
	admin := controlCall{userID: controlOperatorID, role: "admin"}

	t.Run("mutations require reason", func(t *testing.T) {
		cases := []controlCall{
			{method: http.MethodPost, path: "/api/control/v1/platform/ai-gateway/test-connection", body: `{}`,
				headers: map[string]string{contract.IdempotencyKeyHeader: "ext-reason-1"}},
			{method: http.MethodPut, path: "/api/control/v1/users/oauth-providers/google", body: `{"enabled":true}`,
				headers: map[string]string{contract.IdempotencyKeyHeader: "ext-reason-2"}},
			{method: http.MethodPost, path: "/api/control/v1/rewards/codes", body: `{}`,
				headers: map[string]string{contract.IdempotencyKeyHeader: "ext-reason-3"}},
			{method: http.MethodPatch, path: "/api/control/v1/rewards/codes/7", body: `{}`,
				headers: map[string]string{contract.IdempotencyKeyHeader: "ext-reason-4"}},
			{method: http.MethodDelete, path: "/api/control/v1/rewards/codes/7",
				headers: map[string]string{contract.IdempotencyKeyHeader: "ext-reason-5"}},
		}
		for _, call := range cases {
			call.userID = admin.userID
			call.role = admin.role
			status, _, payload := h.serve(call)
			if status != http.StatusBadRequest {
				t.Fatalf("%s %s status = %d, want 400", call.method, call.path, status)
			}
			if code := controlErrorCode(t, payload); code != string(contract.CodeControlReasonRequired) {
				t.Fatalf("code = %q, want control_reason_required", code)
			}
		}
	})

	t.Run("mutations require header idempotency key", func(t *testing.T) {
		call := controlCall{
			method: http.MethodPost, path: "/api/control/v1/rewards/codes", body: `{"reason":"x"}`,
			userID: controlOperatorID, role: "admin",
			headers: map[string]string{OperatorReasonHeader: "has reason, no key"},
		}
		status, _, payload := h.serve(call)
		if status != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", status)
		}
		if code := controlErrorCode(t, payload); code != string(contract.CodeIdempotencyKeyRequired) {
			t.Fatalf("code = %q, want idempotency_key_required", code)
		}
	})

	t.Run("cancel replays on key reuse and conflicts on divergent body", func(t *testing.T) {
		first := controlCall{
			method: http.MethodPost, path: "/api/control/v1/tasks/task_replay123/cancel", body: `{"reason":"first"}`,
			userID: controlOperatorID, role: "admin",
			headers: controlMutationHeaders("ext-replay-1", "first reason"),
		}
		status, _, _ := h.serve(first)
		if status != http.StatusOK {
			t.Fatalf("first cancel status = %d, want 200", status)
		}
		if h.tasks.cancelCalls != 1 {
			t.Fatalf("cancel calls = %d, want 1", h.tasks.cancelCalls)
		}
		second := first
		second.headers = controlMutationHeaders("ext-replay-1", "first reason")
		status, _, _ = h.serve(second)
		if status != http.StatusOK {
			t.Fatalf("replay cancel status = %d, want 200", status)
		}
		if h.tasks.cancelCalls != 1 {
			t.Fatalf("replay must not re-execute: calls = %d, want 1", h.tasks.cancelCalls)
		}
		records := h.sink.Records()
		if last := records[len(records)-1]; !last.Replayed || last.Outcome != ControlAuditOK {
			t.Fatalf("replay audit = %+v, want replayed success", last)
		}
		divergent := first
		divergent.body = `{"reason":"different"}`
		divergent.headers = controlMutationHeaders("ext-replay-1", "different reason")
		status, _, payload := h.serve(divergent)
		if status != http.StatusConflict {
			t.Fatalf("divergent status = %d, want 409", status)
		}
		if code := controlErrorCode(t, payload); code != string(contract.CodeIdempotencyConflict) {
			t.Fatalf("code = %q, want idempotency_conflict", code)
		}
	})

	t.Run("cancel carries the operator reason to the seam and audit", func(t *testing.T) {
		status, _, _ := h.serve(controlCall{
			method: http.MethodPost, path: "/api/control/v1/tasks/task_reason123/cancel",
			userID: controlOperatorID, role: "admin",
			headers: controlMutationHeaders("ext-reason-6", "runaway task"),
		})
		if status != http.StatusOK {
			t.Fatalf("cancel status = %d, want 200", status)
		}
		h.tasks.mu.Lock()
		reason := h.tasks.cancelReason
		h.tasks.mu.Unlock()
		if reason != "runaway task" {
			t.Fatalf("seam reason = %q, want operator reason", reason)
		}
		last := h.sink.Records()[len(h.sink.Records())-1]
		if last.Reason != "runaway task" || last.Operation != "tasks.cancel" || last.TargetID != "task_reason123" {
			t.Fatalf("audit = %+v, want reason + tasks.cancel + target", last)
		}
	})
}

func TestControlRewardsProxyHappyPath(t *testing.T) {
	cmds := &fakeRewardAdminCommands{}
	h := newControlHarness(t, func(deps *ContractControlDeps) {
		deps.RewardCommands = rewardhttp.NewAdminCommandHandler(cmds)
		deps.RewardReads = rewardhttp.NewAdminReadHandler(fakeRewardAdminReads{}, fakeRewardIdentityProjection{})
	})
	admin := controlCall{userID: controlOperatorID, role: "admin"}

	t.Run("create code", func(t *testing.T) {
		status, _, payload := h.serve(controlCall{
			method: http.MethodPost, path: "/api/control/v1/rewards/codes",
			body:   `{"display_name":"launch","code":"ABCDEFGH","credit_amount":10,"max_redemptions":100}`,
			userID: admin.userID, role: admin.role,
			headers: controlMutationHeaders("ext-reward-1", "launch promo"),
		})
		if status != http.StatusOK {
			t.Fatalf("create status = %d, body=%v", status, payload)
		}
		if cmds.creates != 1 {
			t.Fatalf("creates = %d, want 1", cmds.creates)
		}
		last := h.sink.Records()[len(h.sink.Records())-1]
		if last.Operation != "rewards.create_code" || last.Capability != ControlCapabilityCredits ||
			last.Reason != "launch promo" || last.Outcome != ControlAuditOK {
			t.Fatalf("audit = %+v", last)
		}
	})

	t.Run("search codes as POST read", func(t *testing.T) {
		status, _, _ := h.serve(controlCall{
			method: http.MethodPost, path: "/api/control/v1/rewards/codes/search",
			body:   `{"query":"launch"}`,
			userID: admin.userID, role: admin.role,
		})
		if status != http.StatusOK {
			t.Fatalf("search status = %d, want 200", status)
		}
	})

	t.Run("redemption read + update + toggle + delete", func(t *testing.T) {
		for _, call := range []controlCall{
			{method: http.MethodGet, path: "/api/control/v1/rewards/codes/7/redemptions"},
			{method: http.MethodGet, path: "/api/control/v1/rewards/codes/7/redemptions/3"},
		} {
			call.userID = admin.userID
			call.role = admin.role
			if status, _, payload := h.serve(call); status != http.StatusOK {
				t.Fatalf("%s %s status = %d (%v), want 200", call.method, call.path, status, payload)
			}
		}
		mutations := []controlCall{
			{method: http.MethodPatch, path: "/api/control/v1/rewards/codes/7",
				body:    `{"display_name":"y","credit_amount":5,"max_redemptions":10}`,
				headers: controlMutationHeaders("ext-reward-2", "rename")},
			{method: http.MethodPatch, path: "/api/control/v1/rewards/codes/7/enabled",
				body:    `{"enabled":false}`,
				headers: controlMutationHeaders("ext-reward-3", "pause")},
			{method: http.MethodDelete, path: "/api/control/v1/rewards/codes/7",
				headers: controlMutationHeaders("ext-reward-4", "cleanup")},
		}
		for _, call := range mutations {
			call.userID = admin.userID
			call.role = admin.role
			if status, _, payload := h.serve(call); status != http.StatusOK {
				t.Fatalf("%s %s status = %d (%v), want 200", call.method, call.path, status, payload)
			}
		}
		if cmds.updates != 1 || cmds.toggles != 1 || cmds.deletes != 1 {
			t.Fatalf("updates/toggles/deletes = %d/%d/%d, want 1/1/1", cmds.updates, cmds.toggles, cmds.deletes)
		}
	})
}

func TestControlOAuthHandlers(t *testing.T) {
	oauth := &fakeControlOAuth{}
	h := newControlHarness(t, func(deps *ContractControlDeps) {
		deps.OAuth = oauth
	})
	admin := controlCall{userID: controlOperatorID, role: "admin"}

	t.Run("get provider", func(t *testing.T) {
		status, _, payload := h.serve(controlCall{
			method: http.MethodGet, path: "/api/control/v1/users/oauth-providers/google",
			userID: admin.userID, role: admin.role,
		})
		if status != http.StatusOK {
			t.Fatalf("get status = %d (%v), want 200", status, payload)
		}
		data, _ := payload["data"].(map[string]any)
		if data["provider"] != "google" {
			t.Fatalf("provider = %v, want google", data["provider"])
		}
		last := h.sink.Records()[len(h.sink.Records())-1]
		if last.Operation != "users.get_oauth_provider" || last.Capability != ControlCapabilityUsers ||
			last.TargetType != "oauth_provider" || last.TargetID != "google" {
			t.Fatalf("audit = %+v", last)
		}
	})

	t.Run("update provider carries reason", func(t *testing.T) {
		status, _, _ := h.serve(controlCall{
			method: http.MethodPut, path: "/api/control/v1/users/oauth-providers/discord",
			body:   `{"enabled":true,"client_id":"cid","client_secret":"sek"}`,
			userID: admin.userID, role: admin.role,
			headers: controlMutationHeaders("ext-oauth-1", "enable discord"),
		})
		if status != http.StatusOK {
			t.Fatalf("update status = %d, want 200", status)
		}
		if oauth.lastProvide != "discord" || !oauth.lastUpdate.Enabled || oauth.lastUpdate.ClientID != "cid" {
			t.Fatalf("service input = %+v %+v", oauth.lastProvide, oauth.lastUpdate)
		}
		last := h.sink.Records()[len(h.sink.Records())-1]
		if last.Reason != "enable discord" || last.Outcome != ControlAuditOK {
			t.Fatalf("audit = %+v", last)
		}
	})

	t.Run("unknown provider 404", func(t *testing.T) {
		status, _, payload := h.serve(controlCall{
			method: http.MethodGet, path: "/api/control/v1/users/oauth-providers/github",
			userID: admin.userID, role: admin.role,
		})
		if status != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", status)
		}
		if code := controlErrorCode(t, payload); code != string(contract.CodeControlNotFound) {
			t.Fatalf("code = %q, want control_not_found", code)
		}
	})
}

func TestControlBrandingUploadGuards(t *testing.T) {
	newUploadRequest := func(t *testing.T, reason, key string, fileBytes []byte, filename string) *http.Request {
		t.Helper()
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		if reason != "" {
			_ = writer.WriteField("reason", reason)
		}
		if fileBytes != nil {
			part, err := writer.CreateFormFile("file", filename)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := part.Write(fileBytes); err != nil {
				t.Fatal(err)
			}
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(http.MethodPost, "/api/control/v1/platform/settings/branding/logo", &body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		req.Header.Set(contract.RequestIDHeader, "test-upload-req-1")
		req.Header.Set("X-Test-User", "11")
		req.Header.Set("X-Test-Role", "admin")
		if key != "" {
			req.Header.Set(contract.IdempotencyKeyHeader, key)
		}
		return req
	}
	serveUpload := func(engine interface {
		ServeHTTP(http.ResponseWriter, *http.Request)
	}, req *http.Request) (int, map[string]any) {
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, req)
		var payload map[string]any
		if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
			return recorder.Code, map[string]any{"_raw": recorder.Body.String()}
		}
		return recorder.Code, payload
	}

	h := newControlHarness(t, func(deps *ContractControlDeps) {
		deps.BrandingUpload = &fakeControlBrandingUpload{}
	})
	// Minimal valid 1x1 PNG: the upload validator inspects real image
	// bytes (dimensions), so a bare magic header is rejected.
	png, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("reason required", func(t *testing.T) {
		status, payload := serveUpload(h.engine, newUploadRequest(t, "", "ext-up-1", png, "logo.png"))
		if status != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", status)
		}
		if code := controlErrorCode(t, payload); code != string(contract.CodeControlReasonRequired) {
			t.Fatalf("code = %q, want control_reason_required", code)
		}
	})

	t.Run("idempotency key required", func(t *testing.T) {
		status, payload := serveUpload(h.engine, newUploadRequest(t, "brand refresh", "", png, "logo.png"))
		if status != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", status)
		}
		if code := controlErrorCode(t, payload); code != string(contract.CodeIdempotencyKeyRequired) {
			t.Fatalf("code = %q, want idempotency_key_required", code)
		}
	})

	t.Run("file required and validated", func(t *testing.T) {
		status, _ := serveUpload(h.engine, newUploadRequest(t, "brand refresh", "ext-up-2", nil, ""))
		if status != http.StatusBadRequest {
			t.Fatalf("missing file status = %d, want 400", status)
		}
		status, payload := serveUpload(h.engine, newUploadRequest(t, "brand refresh", "ext-up-3", []byte("not an image"), "logo.png"))
		if status != http.StatusBadRequest {
			t.Fatalf("bad content status = %d (%v), want 400", status, payload)
		}
	})

	t.Run("happy path replays on key reuse and conflicts on divergent content", func(t *testing.T) {
		status, payload := serveUpload(h.engine, newUploadRequest(t, "brand refresh", "ext-up-4", png, "logo.png"))
		if status != http.StatusOK {
			t.Fatalf("upload status = %d (%v), want 200", status, payload)
		}
		data, _ := payload["data"].(map[string]any)
		if data["logo_url"] != "/uploads/site/assets/logo.png" {
			t.Fatalf("logo_url = %v", data["logo_url"])
		}
		againStatus, againPayload := serveUpload(h.engine, newUploadRequest(t, "brand refresh", "ext-up-4", png, "logo.png"))
		if againStatus != http.StatusOK {
			t.Fatalf("replay status = %d, want 200", againStatus)
		}
		againData, _ := againPayload["data"].(map[string]any)
		if againData["logo_url"] != data["logo_url"] {
			t.Fatalf("replay logo_url = %v, want %v", againData["logo_url"], data["logo_url"])
		}
		divergent, divergentPayload := serveUpload(h.engine, newUploadRequest(t, "brand refresh", "ext-up-4", append(png, 0x01), "logo.png"))
		if divergent != http.StatusConflict {
			t.Fatalf("divergent status = %d (%v), want 409", divergent, divergentPayload)
		}
		if code := controlErrorCode(t, divergentPayload); code != string(contract.CodeIdempotencyConflict) {
			t.Fatalf("code = %q, want idempotency_conflict", code)
		}
		records := h.sink.Records()
		if len(records) == 0 || records[len(records)-1].Reason != "brand refresh" {
			t.Fatalf("branding upload audit reason = %+v, want multipart reason", records)
		}
	})
}

func TestWorkspaceControlTaskAdapterFailClosed(t *testing.T) {
	adapter := NewWorkspaceControlTasks(nil, nil, nil, nil)
	if adapter == nil {
		t.Fatal("nil db must still return an adapter")
	}
	ctx := context.Background()
	if _, err := adapter.GetTask(ctx, "task_missing123"); !isControlTaskNotFound(err) {
		t.Fatalf("GetTask err = %v, want not found", err)
	}
	if _, _, err := adapter.ListTasks(ctx, nil); !isControlTaskNotFound(err) {
		t.Fatalf("ListTasks err = %v, want not found", err)
	}
	if _, err := adapter.ResolveAdminTaskUserID(ctx, "usr_abc"); !isControlTaskNotFound(err) {
		t.Fatalf("ResolveAdminTaskUserID err = %v, want not found", err)
	}
	if err := adapter.CancelAdminTask(ctx, "task_missing123", controlOperatorID, "reason"); !isControlTaskNotFound(err) {
		t.Fatalf("CancelAdminTask err = %v, want not found", err)
	}
}

func TestControlBrandLogoOverrideAuditTarget(t *testing.T) {
	h := newControlHarness(t, nil)
	status, _, _ := h.serve(controlCall{
		method: http.MethodPut, path: "/api/control/v1/platform/brand-logo-overrides/oauth-provider/google",
		body:   `{"logo_key":"google-oauth"}`,
		userID: controlOperatorID, role: "admin",
		headers: controlMutationHeaders("ext-brand-1", "override logo"),
	})
	// Nil dep fails closed with 501, but the audit record still carries
	// the scope-qualified target, capability, reason, and key.
	if status != http.StatusNotImplemented {
		t.Fatalf("status = %d, want 501", status)
	}
	last := h.sink.Records()[len(h.sink.Records())-1]
	if last.TargetType != "brand_logo_override" || last.TargetID != "oauth-provider:google" {
		t.Fatalf("target = %q %q, want brand_logo_override oauth-provider:google", last.TargetType, last.TargetID)
	}
	if last.Capability != ControlCapabilityPlatform || last.Operation != "platform.upsert_brand_logo_override" ||
		last.Reason != "override logo" || last.IdempotencyKey != "ext-brand-1" {
		t.Fatalf("audit = %+v", last)
	}
}
