package auth

import (
	"context"
	"encoding/json"
	"errors"
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
					"username":                "alice",
					"claim_identity":          "alice@users.ideaboard.local",
					"claim_identity_kind":     "trusted_canonical_registration",
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
	if session.AuthUserID != "auth-123" || session.Username != "alice" || session.ClaimIdentity != "alice@users.ideaboard.local" || !session.ClaimIdentityVerified {
		t.Fatalf("session = %+v, want trusted claim identity from envelope", session)
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
			"data": map[string]any{"session": map[string]any{
				"auth_user_id": "auth-untrusted", "username": "alice",
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
		assertContractField(t, body, "username", "alice")
		assertContractField(t, body, "password", "secret123")
		assertContractField(t, body, "allow_existing", true)
		assertContractField(t, body, "role", "admin")

		writeContractJSON(w, http.StatusOK, map[string]any{
			"code": 0,
			"msg":  "success",
			"data": map[string]any{
				"user": map[string]any{
					"auth_user_id":            "auth-alice",
					"username":                "alice",
					"claim_identity":          "alice@users.ideaboard.local",
					"claim_identity_kind":     "trusted_canonical_registration",
					"claim_identity_verified": true,
					"role":                    "admin",
					"existing":                true,
				},
			},
		})
	})
	defer server.Close()

	user, err := newContractClient(server).EnsureUser(context.Background(), " alice ", "secret123", true, "admin")
	if err != nil {
		t.Fatalf("EnsureUser returned error: %v", err)
	}
	if user.AuthUserID != "auth-alice" || user.Username != "alice" || user.ClaimIdentity != "alice@users.ideaboard.local" || !user.ClaimIdentityVerified || user.Role != "admin" || !user.Existing {
		t.Fatalf("user = %+v, want provisioned auth user from envelope", user)
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
			call: func(ctx context.Context, client *ServiceClient) error {
				return client.DeleteUser(ctx, " auth-123/child ")
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
