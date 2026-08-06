package auth

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ideaboard/api/internal/platform/config"
)

const contractInternalKey = "contract-key"

func TestServiceClientContractValidateSessionSuccess(t *testing.T) {
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
			writeContractJSON(w, http.StatusOK, map[string]any{"code": 0, "msg": "success", "data": map[string]any{"config_version": 3, "readiness": "not_tested"}})
		case r.Method == http.MethodPost && r.URL.Path == "/internal/mail/config/test":
			body := decodeContractBody(t, r)
			assertContractField(t, body, "actor_id", float64(42))
			assertContractField(t, body, "recipient", "ops@example.com")
			writeContractJSON(w, http.StatusOK, map[string]any{"code": 0, "msg": "success", "data": map[string]any{"sent": true}})
		case r.Method == http.MethodDelete && r.URL.Path == "/internal/mail/config":
			body := decodeContractBody(t, r)
			assertContractField(t, body, "actor_id", float64(42))
			writeContractJSON(w, http.StatusOK, map[string]any{"code": 0, "msg": "success", "data": map[string]any{"deleted": true}})
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

func TestServiceClientSocialProviderProxyUsesStableContract(t *testing.T) {
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
		case r.Method == http.MethodGet && r.URL.Path == "/api/auth/capabilities":
			if got := r.Header.Get("X-Internal-Key"); got != "" {
				t.Fatalf("public capabilities leaked X-Internal-Key = %q", got)
			}
			writeContractJSON(w, http.StatusOK, map[string]any{
				"code": 0, "msg": "success", "data": map[string]any{"social": map[string]bool{"google": true}},
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
	if err != nil || !capabilities.Social["google"] {
		t.Fatalf("GetSocialProviderCapabilities = %+v, err=%v", capabilities, err)
	}
}

func TestServiceClientSocialProviderProxyPreservesConflict(t *testing.T) {
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

func TestServiceClientContractSetUserRoleNotFound(t *testing.T) {
	server := newContractServer(t, func(w http.ResponseWriter, r *http.Request) {
		assertContractRequest(t, r, http.MethodPut, "/internal/users/auth-missing/role")
		assertJSONContentType(t, r)
		body := decodeContractBody(t, r)
		assertContractField(t, body, "role", "user")
		writeContractJSON(w, http.StatusNotFound, map[string]any{
			"code": http.StatusNotFound,
			"msg":  "auth user not found",
		})
	})
	defer server.Close()

	err := newContractClient(server).SetUserRole(context.Background(), "auth-missing", "user")
	if !errors.Is(err, ErrAuthUserNotFound) {
		t.Fatalf("SetUserRole err = %v, want ErrAuthUserNotFound", err)
	}
}

func TestServiceClientContractAdminMutationRejectsInvalidNotFoundEnvelope(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		body        string
	}{
		{name: "empty", contentType: "application/json", body: ""},
		{name: "malformed", contentType: "text/plain", body: "not-json"},
		{name: "missing code", contentType: "application/json", body: `{"msg":"auth user not found"}`},
		{name: "missing message", contentType: "application/json", body: `{"code":404}`},
		{name: "mismatched code", contentType: "application/json", body: `{"code":500,"msg":"auth user not found"}`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := newContractServer(t, func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", tc.contentType)
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(tc.body))
			})
			defer server.Close()

			err := newContractClient(server).DeleteUser(context.Background(), "auth-missing")
			if err == nil {
				t.Fatal("DeleteUser error = nil, want contract error")
			}
			if errors.Is(err, ErrAuthUserNotFound) {
				t.Fatalf("DeleteUser error = %v, malformed 404 must not map to ErrAuthUserNotFound", err)
			}
		})
	}
}

func TestServiceClientContractAdminMutationsSuccess(t *testing.T) {
	tests := []struct {
		name         string
		method       string
		path         string
		escapedPath  string
		wantFields   map[string]any
		responseData map[string]any
		call         func(context.Context, *ServiceClient) error
	}{
		{
			name:        "delete user",
			method:      http.MethodDelete,
			path:        "/internal/users/auth-123/child",
			escapedPath: "/internal/users/auth-123%2Fchild",
			wantFields:  nil,
			responseData: map[string]any{
				"auth_user_id": "auth-123/child",
				"deleted":      true,
			},
			call: func(ctx context.Context, client *ServiceClient) error {
				return client.DeleteUser(ctx, " auth-123/child ")
			},
		},
		{
			name:   "set user role",
			method: http.MethodPut,
			path:   "/internal/users/auth-123/role",
			wantFields: map[string]any{
				"role": "admin",
			},
			responseData: map[string]any{
				"auth_user_id": "auth-123",
				"role":         "admin",
			},
			call: func(ctx context.Context, client *ServiceClient) error {
				return client.SetUserRole(ctx, " auth-123 ", "admin")
			},
		},
		{
			name:   "ban user",
			method: http.MethodPost,
			path:   "/internal/users/ban",
			wantFields: map[string]any{
				"userId":       "auth-123",
				"banReason":    "policy violation",
				"banExpiresIn": float64(3600),
			},
			responseData: map[string]any{
				"auth_user_id": "auth-123",
				"banned":       true,
			},
			call: func(ctx context.Context, client *ServiceClient) error {
				return client.BanUser(ctx, " auth-123 ", " policy violation ", 3600)
			},
		},
		{
			name:   "unban user",
			method: http.MethodPost,
			path:   "/internal/users/unban",
			wantFields: map[string]any{
				"userId": "auth-123",
			},
			responseData: map[string]any{
				"auth_user_id": "auth-123",
				"banned":       false,
			},
			call: func(ctx context.Context, client *ServiceClient) error {
				return client.UnbanUser(ctx, " auth-123 ")
			},
		},
		{
			name:   "revoke user sessions",
			method: http.MethodPost,
			path:   "/internal/users/revoke-sessions",
			wantFields: map[string]any{
				"userId": "auth-123",
			},
			responseData: map[string]any{
				"auth_user_id": "auth-123",
				"revoked":      true,
			},
			call: func(ctx context.Context, client *ServiceClient) error {
				return client.RevokeUserSessions(ctx, " auth-123 ")
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := newContractServer(t, func(w http.ResponseWriter, r *http.Request) {
				assertContractRequest(t, r, tc.method, tc.path)
				if tc.escapedPath != "" && r.URL.EscapedPath() != tc.escapedPath {
					t.Fatalf("escaped path = %s, want %s", r.URL.EscapedPath(), tc.escapedPath)
				}
				if len(tc.wantFields) > 0 {
					assertJSONContentType(t, r)
					body := decodeContractBody(t, r)
					for key, want := range tc.wantFields {
						assertContractField(t, body, key, want)
					}
				}
				payload := map[string]any{
					"code": 0,
					"msg":  "success",
				}
				if tc.responseData != nil {
					payload["data"] = tc.responseData
				}
				writeContractJSON(w, http.StatusOK, payload)
			})
			defer server.Close()

			if err := tc.call(context.Background(), newContractClient(server)); err != nil {
				t.Fatalf("%s returned error: %v", tc.name, err)
			}
		})
	}
}

func TestServiceClientContractAdminMutationsRejectMissingSuccessEnvelopeFields(t *testing.T) {
	operations := []struct {
		name         string
		responseData map[string]any
		call         func(context.Context, *ServiceClient) error
	}{
		{
			name:         "delete user",
			responseData: map[string]any{"auth_user_id": "auth-123", "deleted": true},
			call: func(ctx context.Context, client *ServiceClient) error {
				return client.DeleteUser(ctx, "auth-123")
			},
		},
		{
			name:         "set user role",
			responseData: map[string]any{"auth_user_id": "auth-123", "role": "admin"},
			call: func(ctx context.Context, client *ServiceClient) error {
				return client.SetUserRole(ctx, "auth-123", "admin")
			},
		},
		{
			name:         "ban user",
			responseData: map[string]any{"auth_user_id": "auth-123", "banned": true},
			call: func(ctx context.Context, client *ServiceClient) error {
				return client.BanUser(ctx, "auth-123", "", 0)
			},
		},
		{
			name:         "unban user",
			responseData: map[string]any{"auth_user_id": "auth-123", "banned": false},
			call: func(ctx context.Context, client *ServiceClient) error {
				return client.UnbanUser(ctx, "auth-123")
			},
		},
		{
			name:         "revoke user sessions",
			responseData: map[string]any{"auth_user_id": "auth-123", "revoked": true},
			call: func(ctx context.Context, client *ServiceClient) error {
				return client.RevokeUserSessions(ctx, "auth-123")
			},
		},
	}

	for _, operation := range operations {
		for _, missingField := range []string{"code", "msg", "data"} {
			t.Run(operation.name+" missing "+missingField, func(t *testing.T) {
				server := newContractServer(t, func(w http.ResponseWriter, _ *http.Request) {
					payload := map[string]any{
						"code": 0,
						"msg":  "success",
						"data": operation.responseData,
					}
					delete(payload, missingField)
					writeContractJSON(w, http.StatusOK, payload)
				})
				defer server.Close()

				err := operation.call(context.Background(), newContractClient(server))
				if err == nil || !strings.Contains(err.Error(), "invalid success envelope") {
					t.Fatalf("mutation error = %v, want invalid success envelope", err)
				}
			})
		}
	}
}

func TestServiceClientContractAdminMutationsRejectInvalidSuccessData(t *testing.T) {
	tests := []struct {
		name         string
		responseData map[string]any
		omitData     bool
		call         func(context.Context, *ServiceClient) error
	}{
		{
			name:     "delete missing data",
			omitData: true,
			call: func(ctx context.Context, client *ServiceClient) error {
				return client.DeleteUser(ctx, "auth-123")
			},
		},
		{
			name:         "delete wrong user id",
			responseData: map[string]any{"auth_user_id": "auth-other", "deleted": true},
			call: func(ctx context.Context, client *ServiceClient) error {
				return client.DeleteUser(ctx, "auth-123")
			},
		},
		{
			name:         "delete false literal",
			responseData: map[string]any{"auth_user_id": "auth-123", "deleted": false},
			call: func(ctx context.Context, client *ServiceClient) error {
				return client.DeleteUser(ctx, "auth-123")
			},
		},
		{
			name:         "set role wrong user id",
			responseData: map[string]any{"auth_user_id": "auth-other", "role": "admin"},
			call: func(ctx context.Context, client *ServiceClient) error {
				return client.SetUserRole(ctx, "auth-123", "admin")
			},
		},
		{
			name:         "set role wrong role",
			responseData: map[string]any{"auth_user_id": "auth-123", "role": "user"},
			call: func(ctx context.Context, client *ServiceClient) error {
				return client.SetUserRole(ctx, "auth-123", "admin")
			},
		},
		{
			name:         "ban wrong user id",
			responseData: map[string]any{"auth_user_id": "auth-other", "banned": true},
			call: func(ctx context.Context, client *ServiceClient) error {
				return client.BanUser(ctx, "auth-123", "", 0)
			},
		},
		{
			name:         "ban false literal",
			responseData: map[string]any{"auth_user_id": "auth-123", "banned": false},
			call: func(ctx context.Context, client *ServiceClient) error {
				return client.BanUser(ctx, "auth-123", "", 0)
			},
		},
		{
			name:         "unban wrong user id",
			responseData: map[string]any{"auth_user_id": "auth-other", "banned": false},
			call: func(ctx context.Context, client *ServiceClient) error {
				return client.UnbanUser(ctx, "auth-123")
			},
		},
		{
			name:         "unban true literal",
			responseData: map[string]any{"auth_user_id": "auth-123", "banned": true},
			call: func(ctx context.Context, client *ServiceClient) error {
				return client.UnbanUser(ctx, "auth-123")
			},
		},
		{
			name:         "unban missing literal",
			responseData: map[string]any{"auth_user_id": "auth-123"},
			call: func(ctx context.Context, client *ServiceClient) error {
				return client.UnbanUser(ctx, "auth-123")
			},
		},
		{
			name:         "revoke sessions wrong user id",
			responseData: map[string]any{"auth_user_id": "auth-other", "revoked": true},
			call: func(ctx context.Context, client *ServiceClient) error {
				return client.RevokeUserSessions(ctx, "auth-123")
			},
		},
		{
			name:         "revoke sessions false literal",
			responseData: map[string]any{"auth_user_id": "auth-123", "revoked": false},
			call: func(ctx context.Context, client *ServiceClient) error {
				return client.RevokeUserSessions(ctx, "auth-123")
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := newContractServer(t, func(w http.ResponseWriter, _ *http.Request) {
				payload := map[string]any{"code": 0, "msg": "success"}
				if !tc.omitData {
					payload["data"] = tc.responseData
				}
				writeContractJSON(w, http.StatusOK, payload)
			})
			defer server.Close()

			err := tc.call(context.Background(), newContractClient(server))
			if err == nil || !strings.Contains(err.Error(), "invalid success envelope") {
				t.Fatalf("mutation error = %v, want invalid success envelope", err)
			}
		})
	}
}

func TestServiceClientContractRejectsOversizedResponse(t *testing.T) {
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

func newContractServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(handler))
}

func newContractClient(server *httptest.Server) *ServiceClient {
	return NewServiceClient(config.AuthServiceConfig{
		BaseURL:        server.URL,
		InternalAPIKey: contractInternalKey,
		TimeoutSeconds: 1,
	})
}

func assertContractRequest(t *testing.T, r *http.Request, method, path string) {
	t.Helper()
	if r.Method != method {
		t.Fatalf("method = %s, want %s", r.Method, method)
	}
	if r.URL.Path != path {
		t.Fatalf("path = %s, want %s", r.URL.Path, path)
	}
	if got := r.Header.Get("X-Internal-Key"); got != contractInternalKey {
		t.Fatalf("X-Internal-Key = %q, want %q", got, contractInternalKey)
	}
}

func assertContractInternalKey(t *testing.T, r *http.Request) {
	t.Helper()
	if got := r.Header.Get("X-Internal-Key"); got != contractInternalKey {
		t.Fatalf("X-Internal-Key = %q, want %q", got, contractInternalKey)
	}
}

func assertJSONContentType(t *testing.T, r *http.Request) {
	t.Helper()
	if got := r.Header.Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}
}

func decodeContractBody(t *testing.T, r *http.Request) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		t.Fatalf("decode request body failed: %v", err)
	}
	return body
}

func assertContractField(t *testing.T, body map[string]any, key string, want any) {
	t.Helper()
	got, ok := body[key]
	if !ok {
		t.Fatalf("body missing %q field: %#v", key, body)
	}
	if got != want {
		t.Fatalf("body[%q] = %#v, want %#v", key, got, want)
	}
}

func writeContractJSON(w http.ResponseWriter, status int, payload map[string]any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func socialProviderContractEnvelope(version int) map[string]any {
	return map[string]any{
		"code": 0,
		"msg":  "success",
		"data": map[string]any{
			"provider": "google", "enabled": true, "configured": true, "available": true,
			"client_id": "google-client.apps.googleusercontent.com", "client_secret_configured": true,
			"scopes": []string{"openid", "email", "profile"}, "config_version": version,
			"callback_url": "https://example.com/api/auth/callback/google", "updated_at": "2026-08-06T00:00:00Z",
		},
	}
}
