package tests

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/gin-gonic/gin"
	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/testsupport"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type fakeAdminUserService struct {
	user *User
	err  error
}

func (f *fakeAdminUserService) ListUsers(context.Context, *UserListRequest) (*UserListResponse, error) {
	return nil, f.err
}

func (f *fakeAdminUserService) GetByPublicID(context.Context, string) (*User, error) {
	return f.user, f.err
}

func TestAdminHighRiskHandlersRejectUserPrincipal(t *testing.T) {
	t.Parallel()
	handler := NewAdminHandler(nil, nil, nil)

	for _, tt := range []struct {
		name   string
		method string
		path   string
		route  string
		run    gin.HandlerFunc
	}{
		{name: "outbound policy", method: http.MethodPut, path: "/settings/outbound-policy", route: "/settings/outbound-policy", run: handler.UpdateOutboundPolicy},
		{name: "role", method: http.MethodPut, path: "/users/00000000-0000-0000-0000-000000000001/role", route: "/users/:public_id/role", run: handler.UpdateUserRole},
		{name: "delete", method: http.MethodDelete, path: "/users/00000000-0000-0000-0000-000000000001", route: "/users/:public_id", run: handler.DeleteUser},
		{name: "ban", method: http.MethodPost, path: "/users/00000000-0000-0000-0000-000000000001/ban", route: "/users/:public_id/ban", run: handler.BanUser},
		{name: "unban", method: http.MethodPost, path: "/users/00000000-0000-0000-0000-000000000001/unban", route: "/users/:public_id/unban", run: handler.UnbanUser},
		{name: "revoke sessions", method: http.MethodPost, path: "/users/00000000-0000-0000-0000-000000000001/revoke", route: "/users/:public_id/revoke", run: handler.RevokeUserSessions},
	} {
		t.Run(tt.name, func(t *testing.T) {
			router := gin.New()
			router.Use(func(c *gin.Context) {
				testsupport.SetPrincipal(t, c, 7, principal.RoleUser)
				c.Next()
			})
			router.Handle(tt.method, tt.route, tt.run)
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(tt.method, tt.path, nil))
			if recorder.Code != http.StatusForbidden {
				t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestAdminIdentityCommandHandlersReturn500WhenCommandsAreMissing(t *testing.T) {
	t.Parallel()
	handler := NewAdminHandler(nil, nil, nil)

	for _, tt := range []struct {
		name   string
		method string
		path   string
		route  string
		run    gin.HandlerFunc
	}{
		{name: "update role", method: http.MethodPut, path: "/users/00000000-0000-0000-0000-000000000001/role", route: "/users/:public_id/role", run: handler.UpdateUserRole},
		{name: "ban", method: http.MethodPost, path: "/users/00000000-0000-0000-0000-000000000001/ban", route: "/users/:public_id/ban", run: handler.BanUser},
		{name: "unban", method: http.MethodPost, path: "/users/00000000-0000-0000-0000-000000000001/unban", route: "/users/:public_id/unban", run: handler.UnbanUser},
		{name: "revoke", method: http.MethodPost, path: "/users/00000000-0000-0000-0000-000000000001/revoke", route: "/users/:public_id/revoke", run: handler.RevokeUserSessions},
		{name: "delete", method: http.MethodDelete, path: "/users/00000000-0000-0000-0000-000000000001", route: "/users/:public_id", run: handler.DeleteUser},
	} {
		t.Run(tt.name, func(t *testing.T) {
			router := gin.New()
			router.Use(func(c *gin.Context) {
				testsupport.SetPrincipal(t, c, 7, principal.RoleAdmin)
				c.Next()
			})
			router.Handle(tt.method, tt.route, tt.run)

			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(tt.method, tt.path, nil))
			if recorder.Code != http.StatusInternalServerError {
				t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusInternalServerError, recorder.Body.String())
			}
		})
	}
}

func TestAdminImpersonateUserHTTPContractWithoutDatabase(t *testing.T) {
	t.Parallel()
	const targetPublicID = "00000000-0000-0000-0000-000000000001"
	authUserID := "auth-impersonated"
	userService := &fakeAdminUserService{user: &User{ID: 8, AuthUserID: &authUserID, Role: RoleUser}}

	t.Run("missing dependencies return 500", func(t *testing.T) {
		cases := []struct {
			name string
			run  gin.HandlerFunc
		}{
			{name: "nil handler", run: func(c *gin.Context) { var handler *AdminHandler; handler.ImpersonateUser(c) }},
			{name: "nil user service", run: NewAdminHandler(nil, nil, nil, &fakeAdminUserImpersonator{}).ImpersonateUser},
			{name: "nil impersonator", run: NewAdminHandler(userService, nil, nil).ImpersonateUser},
		}
		for _, tt := range cases {
			t.Run(tt.name, func(t *testing.T) {
				recorder := httptest.NewRecorder()
				context, _ := gin.CreateTestContext(recorder)
				context.Request = httptest.NewRequest(http.MethodPost, "/users/"+targetPublicID+"/impersonate", nil)
				context.Params = gin.Params{{Key: "public_id", Value: targetPublicID}}
				testsupport.SetPrincipal(t, context, 7, principal.RoleAdmin)
				tt.run(context)
				if recorder.Code != http.StatusInternalServerError {
					t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusInternalServerError, recorder.Body.String())
				}
			})
		}
	})

	for _, tt := range []struct {
		name       string
		err        error
		wantStatus int
	}{
		{name: "unauthorized", err: adminImpersonationStatusError(http.StatusUnauthorized), wantStatus: http.StatusUnauthorized},
		{name: "forbidden", err: adminImpersonationStatusError(http.StatusForbidden), wantStatus: http.StatusForbidden},
		{name: "bad request", err: adminImpersonationStatusError(http.StatusBadRequest), wantStatus: http.StatusBadRequest},
		{name: "not found", err: adminImpersonationStatusError(http.StatusNotFound), wantStatus: http.StatusBadRequest},
		{name: "upstream failure", err: adminImpersonationStatusError(http.StatusInternalServerError), wantStatus: http.StatusBadGateway},
		{name: "untyped failure", err: errors.New("upstream failure"), wantStatus: http.StatusBadGateway},
	} {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(recorder)
			context.Request = httptest.NewRequest(http.MethodPost, "/users/"+targetPublicID+"/impersonate", nil)
			context.Params = gin.Params{{Key: "public_id", Value: targetPublicID}}
			testsupport.SetPrincipal(t, context, 7, principal.RoleAdmin)

			handler := NewAdminHandler(userService, nil, nil, &fakeAdminUserImpersonator{err: tt.err})
			handler.ImpersonateUser(context)

			if recorder.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", recorder.Code, tt.wantStatus, recorder.Body.String())
			}
		})
	}

	t.Run("success has no target data", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		context, _ := gin.CreateTestContext(recorder)
		context.Request = httptest.NewRequest(http.MethodPost, "/users/"+targetPublicID+"/impersonate", nil)
		context.Params = gin.Params{{Key: "public_id", Value: targetPublicID}}
		testsupport.SetPrincipal(t, context, 7, principal.RoleAdmin)

		handler := NewAdminHandler(userService, nil, nil, &fakeAdminUserImpersonator{setCookies: []string{"session=impersonated; Path=/; HttpOnly"}})
		handler.ImpersonateUser(context)

		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
		}
		var payload map[string]json.RawMessage
		if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if _, ok := payload["data"]; ok {
			t.Fatalf("response leaked target data: %s", recorder.Body.String())
		}
	})
}

func TestIdentityAdminCommandErrorsMapToHTTP(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantBody   string
	}{
		{name: "invalid", err: identityAdminError(IdentityAdminInvalid, "invalid", nil), wantStatus: 400, wantBody: "invalid"},
		{name: "target not found", err: identityAdminError(IdentityAdminTargetNotFound, "target missing", nil), wantStatus: 400, wantBody: "target missing"},
		{name: "Auth identity not found", err: identityAdminError(IdentityAdminAuthIdentityNotFound, "Auth missing", nil), wantStatus: 400, wantBody: "Auth missing"},
		{name: "forbidden", err: identityAdminError(IdentityAdminForbidden, "forbidden", nil), wantStatus: 403, wantBody: "forbidden"},
		{name: "conflict", err: identityAdminError(IdentityAdminConflict, "payment state pending", nil), wantStatus: 409, wantBody: "payment state pending"},
		{name: "Auth failure", err: identityAdminError(IdentityAdminAuthFailure, "Auth failed", errors.New("dependency failed")), wantStatus: 502, wantBody: "Auth failed"},
		{name: "business failure", err: identityAdminError(IdentityAdminBusinessFailure, "business failed", errors.New("database failed")), wantStatus: 500, wantBody: "business failed"},
		{name: "reconciliation required", err: identityAdminError(IdentityAdminReconciliationRequired, "reconcile", errors.New("rollback failed")), wantStatus: 500, wantBody: "reconcile"},
		{name: "unavailable", err: identityAdminError(IdentityAdminUnavailable, "unavailable", nil), wantStatus: 500, wantBody: "unavailable"},
		{name: "unknown typed kind", err: identityAdminError(IdentityAdminErrorKind("future"), "future failure", nil), wantStatus: 500, wantBody: "管理员操作失败"},
		{name: "untyped error", err: errors.New("unexpected"), wantStatus: 500, wantBody: "管理员操作失败"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(recorder)

			respondIdentityAdminCommandError(context, tt.err)

			if recorder.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", recorder.Code, tt.wantStatus, recorder.Body.String())
			}
			if !strings.Contains(recorder.Body.String(), tt.wantBody) {
				t.Fatalf("body = %s, want %q", recorder.Body.String(), tt.wantBody)
			}
		})
	}
}
