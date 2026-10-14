package auth

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
)

func TestServiceClientContractSetUserRoleNotFound(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
				"banExpiresIn": float64(3600),
			},
			responseData: map[string]any{
				"auth_user_id": "auth-123",
				"banned":       true,
			},
			call: func(ctx context.Context, client *ServiceClient) error {
				return client.BanUser(ctx, " auth-123 ", 3600)
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
	t.Parallel()
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
				return client.BanUser(ctx, "auth-123", 0)
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
	t.Parallel()
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
				return client.BanUser(ctx, "auth-123", 0)
			},
		},
		{
			name:         "ban false literal",
			responseData: map[string]any{"auth_user_id": "auth-123", "banned": false},
			call: func(ctx context.Context, client *ServiceClient) error {
				return client.BanUser(ctx, "auth-123", 0)
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
