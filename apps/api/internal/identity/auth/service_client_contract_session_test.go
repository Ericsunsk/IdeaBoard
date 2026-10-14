package auth

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestServiceClientContractValidateSessionSuccess(t *testing.T) {
	t.Parallel()
	server := newContractServer(t, func(w http.ResponseWriter, r *http.Request) {
		assertContractRequest(t, r, http.MethodGet, "/internal/session")
		if got := r.Header.Get("Cookie"); got != "better-auth.session_token=session-123" {
			t.Fatalf("Cookie = %q, want session cookie", got)
		}
		writeContractJSON(w, http.StatusOK, map[string]any{
			"code": 0,
			"msg":  "success",
			"data": map[string]any{
				"session": map[string]any{
					"auth_user_id":            "auth-123",
					"email":                   "alice@example.com",
					"display_name":            "Alice 设计师",
					"avatar":                  "https://lh3.googleusercontent.com/avatar",
					"claim_identity":          "alice@example.com",
					"claim_identity_kind":     "canonical_email",
					"claim_identity_verified": true,
				},
			},
		})
	})
	defer server.Close()

	session, err := newContractClient(server).ValidateSession(context.Background(), "better-auth.session_token=session-123")
	if err != nil {
		t.Fatalf("ValidateSession returned error: %v", err)
	}
	if session.AuthUserID != "auth-123" || session.DisplayName != "Alice 设计师" || session.Avatar != "https://lh3.googleusercontent.com/avatar" || session.ClaimIdentity != "alice@example.com" || !session.ClaimIdentityVerified {
		t.Fatalf("session = %+v, want trusted claim identity from envelope", session)
	}
}

func TestServiceClientMailConfigProxyUsesInternalContractAndActor(t *testing.T) {
	t.Parallel()
	server := newContractServer(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Internal-Key"); got != contractInternalKey {
			t.Fatalf("X-Internal-Key = %q, want %q", got, contractInternalKey)
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/internal/mail/config":
			writeContractJSON(w, http.StatusOK, map[string]any{
				"code": 0, "msg": "success", "data": map[string]any{
					"configured": true, "api_key_configured": true, "from_name": "IdeaBoard",
					"from_email": "noreply@example.com", "reply_to": "support@example.com",
					"config_version": 2, "readiness": "ready",
				},
			})
		case r.Method == http.MethodPut && r.URL.Path == "/internal/mail/config":
			body := decodeContractBody(t, r)
			assertContractField(t, body, "actor_id", float64(42))
			assertContractField(t, body, "api_key", "re_secret")
			if _, exists := body["enabled"]; exists {
				t.Fatal("omitted enabled must remain absent from the Auth request")
			}
			writeContractJSON(w, http.StatusOK, map[string]any{"code": 0, "msg": "success", "data": map[string]any{"config_version": 3, "readiness": "not_tested"}})
		case r.Method == http.MethodPost && r.URL.Path == "/internal/mail/config/test":
			body := decodeContractBody(t, r)
			assertContractField(t, body, "actor_id", float64(42))
			assertContractField(t, body, "recipient", "ops@example.com")
			writeContractJSON(w, http.StatusOK, map[string]any{"code": 0, "msg": "success", "data": map[string]any{"ok": true}})
		case r.Method == http.MethodDelete && r.URL.Path == "/internal/mail/config":
			body := decodeContractBody(t, r)
			assertContractField(t, body, "actor_id", float64(42))
			writeContractJSON(w, http.StatusOK, map[string]any{"code": 0, "msg": "success", "data": map[string]any{"ok": true}})
		default:
			http.NotFound(w, r)
		}
	})
	defer server.Close()

	client := newContractClient(server)
	summary, err := client.GetMailConfig(context.Background())
	if err != nil || summary.ConfigVersion != 2 || summary.Readiness != "ready" {
		t.Fatalf("GetMailConfig = %+v, err=%v", summary, err)
	}
	if _, err := client.UpdateMailConfig(context.Background(), MailConfigUpdate{APIKey: "re_secret", FromName: "IdeaBoard", FromEmail: "noreply@example.com"}, 42); err != nil {
		t.Fatalf("UpdateMailConfig: %v", err)
	}
	if err := client.SendMailConfigTest(context.Background(), "ops@example.com", 42); err != nil {
		t.Fatalf("SendMailConfigTest: %v", err)
	}
	if err := client.ClearMailConfig(context.Background(), 42); err != nil {
		t.Fatalf("ClearMailConfig: %v", err)
	}
}

func TestServiceClientMailConfigUpdatePreservesOptionalEnabled(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"omitted", "disabled", "enabled"} {
		t.Run(name, func(t *testing.T) {
			var enabled *bool
			if name != "omitted" {
				value := name == "enabled"
				enabled = &value
			}
			server := newContractServer(t, func(w http.ResponseWriter, r *http.Request) {
				assertContractRequest(t, r, http.MethodPut, "/internal/mail/config")
				body := decodeContractBody(t, r)
				value, exists := body["enabled"]
				if enabled == nil {
					if exists {
						t.Error("omitted enabled must remain absent")
					}
				} else if !exists || value != *enabled {
					t.Errorf("enabled = %v, present = %v, want %v", value, exists, *enabled)
				}
				writeContractJSON(w, http.StatusOK, map[string]any{
					"code": 0, "msg": "success", "data": map[string]any{"enabled": false},
				})
			})
			defer server.Close()
			summary, err := newContractClient(server).UpdateMailConfig(context.Background(), MailConfigUpdate{
				FromName: "IdeaBoard", FromEmail: "auth@example.com", Enabled: enabled,
			}, 42)
			if err != nil {
				t.Fatalf("UpdateMailConfig: %v", err)
			}
			if summary.Enabled {
				t.Fatal("disabled Auth summary must remain disabled")
			}
		})
	}
}

func TestServiceClientSocialProviderProxyUsesStableContract(t *testing.T) {
	t.Parallel()
	var updateCalls int
	server := newContractServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/internal/social-providers/google":
			assertContractInternalKey(t, r)
			writeContractJSON(w, http.StatusOK, socialProviderContractEnvelope(3))
		case r.Method == http.MethodPut && r.URL.Path == "/internal/social-providers/google":
			assertContractInternalKey(t, r)
			updateCalls++
			body := decodeContractBody(t, r)
			assertContractField(t, body, "enabled", true)
			assertContractField(t, body, "client_id", "google-client.apps.googleusercontent.com")
			_, hasSecret := body["client_secret"]
			if updateCalls == 1 && hasSecret {
				t.Fatalf("blank client_secret must be omitted: %#v", body)
			}
			if updateCalls == 2 {
				assertContractField(t, body, "client_secret", "google-secret")
			}
			writeContractJSON(w, http.StatusOK, socialProviderContractEnvelope(3+updateCalls))
		case r.Method == http.MethodGet && r.URL.Path == "/internal/social-providers/capabilities":
			assertContractInternalKey(t, r)
			writeContractJSON(w, http.StatusOK, map[string]any{
				"code": 0, "msg": "success", "data": map[string]any{"social": map[string]bool{"google": true, "discord": true}},
			})
		default:
			http.NotFound(w, r)
		}
	})
	defer server.Close()

	client := newContractClient(server)
	summary, err := client.GetSocialProviderSettings(context.Background(), "google")
	if err != nil || summary.Provider != "google" || summary.ConfigVersion != 3 || !summary.ClientSecretConfigured {
		t.Fatalf("GetSocialProviderSettings = %+v, err=%v", summary, err)
	}
	input := SocialProviderSettingsUpdate{Enabled: true, ClientID: "google-client.apps.googleusercontent.com"}
	if _, err := client.UpdateSocialProviderSettings(context.Background(), "google", input); err != nil {
		t.Fatalf("UpdateSocialProviderSettings blank secret: %v", err)
	}
	input.ClientSecret = "google-secret"
	if _, err := client.UpdateSocialProviderSettings(context.Background(), "google", input); err != nil {
		t.Fatalf("UpdateSocialProviderSettings with secret: %v", err)
	}
	capabilities, err := client.GetSocialProviderCapabilities(context.Background())
	if err != nil || !capabilities.Social["google"] || !capabilities.Social["discord"] {
		t.Fatalf("GetSocialProviderCapabilities = %+v, err=%v", capabilities, err)
	}
}

func TestServiceClientSocialProviderProxyPreservesConflict(t *testing.T) {
	t.Parallel()
	server := newContractServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeContractJSON(w, http.StatusConflict, map[string]any{
			"code": http.StatusConflict,
			"msg":  "请先停用 Google 登录再修改 Client ID",
		})
	})
	defer server.Close()

	_, err := newContractClient(server).UpdateSocialProviderSettings(
		context.Background(),
		"google",
		SocialProviderSettingsUpdate{Enabled: true, ClientID: "changed.apps.googleusercontent.com"},
	)
	var providerErr *SocialProviderSettingsError
	if !errors.As(err, &providerErr) || providerErr.Status != http.StatusConflict || !strings.Contains(providerErr.Message, "先停用") {
		t.Fatalf("UpdateSocialProviderSettings error = %#v, want preserved conflict", err)
	}
}

func TestServiceClientContractValidateSessionRejectsMissingSuccessEnvelopeFields(t *testing.T) {
	t.Parallel()
	validData := map[string]any{
		"session": map[string]any{
			"auth_user_id":            "auth-123",
			"email":                   "alice@example.com",
			"display_name":            "alice",
			"claim_identity":          "alice@example.com",
			"claim_identity_kind":     "canonical_email",
			"claim_identity_verified": true,
		},
	}
	tests := []struct {
		name    string
		payload map[string]any
	}{
		{name: "missing code", payload: map[string]any{"msg": "success", "data": validData}},
		{name: "missing message", payload: map[string]any{"code": 0, "data": validData}},
		{name: "missing data", payload: map[string]any{"code": 0, "msg": "success"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := newContractServer(t, func(w http.ResponseWriter, _ *http.Request) {
				writeContractJSON(w, http.StatusOK, tc.payload)
			})
			defer server.Close()

			_, err := newContractClient(server).ValidateSession(context.Background(), "session=valid")
			if err == nil || !strings.Contains(err.Error(), "invalid success envelope") {
				t.Fatalf("ValidateSession error = %v, want invalid success envelope", err)
			}
		})
	}
}

func TestServiceClientContractValidateSessionUnauthorized(t *testing.T) {
	t.Parallel()
	server := newContractServer(t, func(w http.ResponseWriter, r *http.Request) {
		assertContractRequest(t, r, http.MethodGet, "/internal/session")
		writeContractJSON(w, http.StatusUnauthorized, map[string]any{
			"code": http.StatusUnauthorized,
			"msg":  "unauthorized",
		})
	})
	defer server.Close()

	_, err := newContractClient(server).ValidateSession(context.Background(), "better-auth.session_token=expired")
	if !errors.Is(err, ErrSessionUnauthorized) {
		t.Fatalf("ValidateSession err = %v, want ErrSessionUnauthorized", err)
	}
}

func TestServiceClientContractValidateSessionRejectsUntrustedClaimIdentity(t *testing.T) {
	t.Parallel()
	server := newContractServer(t, func(w http.ResponseWriter, r *http.Request) {
		assertContractRequest(t, r, http.MethodGet, "/internal/session")
		writeContractJSON(w, http.StatusOK, map[string]any{
			"code": 0,
			"msg":  "success",
			"data": map[string]any{"session": map[string]any{
				"auth_user_id": "auth-untrusted", "display_name": "alice",
				"claim_identity": "attacker@example.com", "claim_identity_kind": "untrusted",
				"claim_identity_verified": false,
			}},
		})
	})
	defer server.Close()

	if _, err := newContractClient(server).ValidateSession(context.Background(), "session=untrusted"); err == nil || !strings.Contains(err.Error(), "untrusted") {
		t.Fatalf("ValidateSession error = %v, want untrusted identity rejection", err)
	}
}

func TestServiceClientContractEnsureUserRequestAndSuccess(t *testing.T) {
	t.Parallel()
	server := newContractServer(t, func(w http.ResponseWriter, r *http.Request) {
		assertContractRequest(t, r, http.MethodPost, "/internal/users/ensure")
		assertJSONContentType(t, r)
		body := decodeContractBody(t, r)
		assertContractField(t, body, "email", "alice@example.com")
		assertContractField(t, body, "display_name", "alice")
		assertContractField(t, body, "password", "secret1234")
		assertContractField(t, body, "allow_existing", true)
		assertContractField(t, body, "role", "admin")

		writeContractJSON(w, http.StatusOK, map[string]any{
			"code": 0,
			"msg":  "success",
			"data": map[string]any{
				"user": map[string]any{
					"auth_user_id":            "auth-alice",
					"display_name":            "alice",
					"email":                   "alice@example.com",
					"claim_identity":          "alice@example.com",
					"claim_identity_kind":     "canonical_email",
					"claim_identity_verified": true,
					"role":                    "admin",
					"existing":                true,
				},
			},
		})
	})
	defer server.Close()

	user, err := newContractClient(server).EnsureUser(context.Background(), " Alice@Example.com ", "alice", "secret1234", true, "admin")
	if err != nil {
		t.Fatalf("EnsureUser returned error: %v", err)
	}
	if user.AuthUserID != "auth-alice" || user.DisplayName != "alice" || user.ClaimIdentity != "alice@example.com" || !user.ClaimIdentityVerified || user.Role != "admin" || !user.Existing {
		t.Fatalf("user = %+v, want provisioned auth user from envelope", user)
	}
}

func TestServiceClientContractEnsureUserRejectsMissingSuccessEnvelopeFields(t *testing.T) {
	t.Parallel()
	validData := map[string]any{
		"user": map[string]any{
			"auth_user_id":            "auth-alice",
			"display_name":            "alice",
			"email":                   "alice@example.com",
			"claim_identity":          "alice@example.com",
			"claim_identity_kind":     "canonical_email",
			"claim_identity_verified": true,
			"role":                    "admin",
			"existing":                true,
		},
	}
	tests := []struct {
		name    string
		payload map[string]any
	}{
		{name: "missing code", payload: map[string]any{"msg": "success", "data": validData}},
		{name: "missing message", payload: map[string]any{"code": 0, "data": validData}},
		{name: "missing data", payload: map[string]any{"code": 0, "msg": "success"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := newContractServer(t, func(w http.ResponseWriter, _ *http.Request) {
				writeContractJSON(w, http.StatusOK, tc.payload)
			})
			defer server.Close()

			_, err := newContractClient(server).EnsureUser(context.Background(), "alice@example.com", "alice", "secret1234", true, "admin")
			if err == nil || !strings.Contains(err.Error(), "invalid success envelope") {
				t.Fatalf("EnsureUser error = %v, want invalid success envelope", err)
			}
		})
	}
}

func TestServiceClientContractRejectsOversizedResponse(t *testing.T) {
	t.Parallel()
	server := newContractServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(strings.Repeat("x", int(maxAuthServiceResponseBytes+1))))
	})
	defer server.Close()

	_, err := newContractClient(server).ValidateSession(context.Background(), "session=oversized")
	if !errors.Is(err, errAuthServiceResponseTooLarge) {
		t.Fatalf("ValidateSession err = %v, want errAuthServiceResponseTooLarge", err)
	}
}

func TestBoundedResponseBodyAllowsExactLimit(t *testing.T) {
	t.Parallel()
	want := "exact"
	body := &boundedResponseBody{
		ReadCloser: io.NopCloser(strings.NewReader(want)),
		remaining:  int64(len(want)),
	}

	got, err := io.ReadAll(body)
	if err != nil {
		t.Fatalf("ReadAll returned error: %v", err)
	}
	if string(got) != want {
		t.Fatalf("body = %q, want %q", got, want)
	}
}

func TestServiceClientContractRejectsInvalidErrorEnvelope(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		body string
	}{
		{name: "malformed JSON", body: "not-json"},
		{name: "missing code", body: `{"msg":"failed"}`},
		{name: "missing message", body: `{"code":500}`},
		{name: "null code", body: `{"code":null,"msg":"failed"}`},
		{name: "null message", body: `{"code":500,"msg":null}`},
		{name: "string code", body: `{"code":"500","msg":"failed"}`},
		{name: "numeric message", body: `{"code":500,"msg":123}`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := newContractServer(t, func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "text/plain")
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(tc.body))
			})
			defer server.Close()

			_, err := newContractClient(server).EnsureUser(context.Background(), "alice@example.com", "alice", "secret1234", false)
			if err == nil || !strings.Contains(err.Error(), "decode error envelope") {
				t.Fatalf("EnsureUser err = %v, want error envelope decode failure", err)
			}
		})
	}
}

func TestServiceClientImpersonationBridgeForwardsSessionCookieWithoutInternalKey(t *testing.T) {
	t.Parallel()
	server := newContractServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/auth/admin/impersonate-user" {
			t.Fatalf("request = %s %s, want POST /api/auth/admin/impersonate-user", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Cookie"); got != "session=admin-session" {
			t.Fatalf("Cookie = %q, want forwarded admin session", got)
		}
		if got := r.Header.Get("X-Internal-Key"); got != "" {
			t.Fatalf("X-Internal-Key = %q, want absent", got)
		}
		body := decodeContractBody(t, r)
		assertContractField(t, body, "userId", "auth-target")
		w.Header().Add("Set-Cookie", "better-auth.session_token=impersonated; Path=/; HttpOnly")
		writeContractJSON(w, http.StatusOK, map[string]any{"code": 0, "msg": "success", "data": map[string]any{}})
	})
	defer server.Close()

	cookies, err := newContractClient(server).ImpersonateUser(context.Background(), "session=admin-session", " auth-target ")
	if err != nil {
		t.Fatalf("ImpersonateUser error: %v", err)
	}
	if len(cookies) != 1 || !strings.HasPrefix(cookies[0], "better-auth.session_token=impersonated") {
		t.Fatalf("Set-Cookie = %#v, want impersonation cookie", cookies)
	}
}

func TestServiceClientImpersonationBridgeRejectsSuccessWithoutSessionCookie(t *testing.T) {
	t.Parallel()
	server := newContractServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeContractJSON(w, http.StatusOK, map[string]any{"code": 0, "msg": "success", "data": map[string]any{}})
	})
	defer server.Close()

	_, err := newContractClient(server).ImpersonateUser(context.Background(), "session=admin-session", "auth-target")
	var impersonationErr *ImpersonationError
	if !errors.As(err, &impersonationErr) || impersonationErr.Status != http.StatusBadGateway {
		t.Fatalf("ImpersonateUser error = %#v, want 502 contract failure", err)
	}
}

func TestServiceClientGetUserStatesUsesStableContract(t *testing.T) {
	t.Parallel()
	server := newContractServer(t, func(w http.ResponseWriter, r *http.Request) {
		assertContractRequest(t, r, http.MethodPost, "/internal/users/states")
		assertJSONContentType(t, r)
		body := decodeContractBody(t, r)
		ids, ok := body["user_ids"].([]any)
		if !ok || len(ids) != 3 {
			t.Fatalf("body[user_ids] = %#v, want 3 requested ids", body["user_ids"])
		}
		writeContractJSON(w, http.StatusOK, map[string]any{
			"code": 0,
			"msg":  "success",
			"data": map[string]any{
				"users": []any{
					map[string]any{
						"auth_user_id": "auth-admin",
						"role":         "admin",
						"banned":       false,
						"ban_expires":  nil,
					},
					map[string]any{
						"auth_user_id": "auth-banned",
						"role":         "user",
						"banned":       true,
						"ban_expires":  "2026-10-11T03:00:00Z",
					},
				},
			},
		})
	})
	defer server.Close()

	// Unknown Auth IDs are omitted by the read-only contract, never fabricated.
	states, err := newContractClient(server).GetUserStates(context.Background(), []string{" auth-admin ", "auth-banned", "auth-admin", "auth-missing", " "})
	if err != nil {
		t.Fatalf("GetUserStates returned error: %v", err)
	}
	if len(states) != 2 {
		t.Fatalf("states = %#v, want 2 known identities (missing omitted)", states)
	}
	admin := states["auth-admin"]
	if admin.Role != "admin" || admin.Banned || admin.BanExpiresAt != nil {
		t.Fatalf("admin state = %+v, want unbanned admin without ban metadata", admin)
	}
	banned := states["auth-banned"]
	if banned.Role != "user" || !banned.Banned {
		t.Fatalf("banned state = %+v, want banned user", banned)
	}
	if banned.BanExpiresAt == nil || banned.BanExpiresAt.UTC().Format("2006-01-02") != "2026-10-11" {
		t.Fatalf("banned ban expiry = %+v, want 2026-10-11", banned.BanExpiresAt)
	}
	if _, ok := states["auth-missing"]; ok {
		t.Fatalf("states contains auth-missing = %#v, unknown IDs must be omitted", states)
	}
}

func TestServiceClientGetUserStatesRejectsInvalidEnvelope(t *testing.T) {
	t.Parallel()
	server := newContractServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeContractJSON(w, http.StatusOK, map[string]any{
			"code": 0,
			"msg":  "success",
			"data": map[string]any{
				"users": []any{
					map[string]any{"auth_user_id": "", "role": "user", "banned": false},
				},
			},
		})
	})
	defer server.Close()

	if _, err := newContractClient(server).GetUserStates(context.Background(), []string{"auth-user"}); err == nil {
		t.Fatal("GetUserStates with empty auth_user_id succeeded, want invalid envelope error")
	}
}
