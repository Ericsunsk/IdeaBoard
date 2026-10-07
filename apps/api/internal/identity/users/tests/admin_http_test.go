package tests

import (
	"context"
	"encoding/json"
	"errors"
	"ideaboard/api/internal/credits/domain"
	"ideaboard/api/internal/credits/ports"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"ideaboard/api/internal/credits"
	"ideaboard/api/internal/identity/autherrors"
	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/identity/users/application/authsync"
	"ideaboard/api/internal/platform/settings"
	"ideaboard/api/internal/testdb"
	"ideaboard/api/internal/testsupport"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

type fakeAdminAuthManager struct {
	deleteErr          error
	deletedAuthUserID  string
	banErr             error
	unbanErr           error
	revokeErr          error
	bannedAuthUserID   string
	unbannedAuthUserID string
	revokedAuthUserID  string
}

type fakeAdminUserImpersonator struct {
	cookieHeader string
	authUserID   string
	setCookies   []string
	err          error
}

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

type adminImpersonationStatusError int

func (e adminImpersonationStatusError) Error() string {
	return "impersonation failed"
}

func (e adminImpersonationStatusError) StatusCode() int {
	return int(e)
}

func (f *fakeAdminUserImpersonator) ImpersonateUser(_ context.Context, cookieHeader string, authUserID string) ([]string, error) {
	f.cookieHeader = cookieHeader
	f.authUserID = authUserID
	return f.setCookies, f.err
}

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

func (f *fakeAdminAuthManager) DeleteUser(_ context.Context, authUserID string) error {
	f.deletedAuthUserID = authUserID
	return f.deleteErr
}

func (f *fakeAdminAuthManager) SetUserRole(_ context.Context, _ string, _ string) error {
	return nil
}

func (f *fakeAdminAuthManager) BanUser(_ context.Context, authUserID string, _ string, _ int) error {
	f.bannedAuthUserID = authUserID
	return f.banErr
}

func (f *fakeAdminAuthManager) UnbanUser(_ context.Context, authUserID string) error {
	f.unbannedAuthUserID = authUserID
	return f.unbanErr
}

func (f *fakeAdminAuthManager) RevokeUserSessions(_ context.Context, authUserID string) error {
	f.revokedAuthUserID = authUserID
	return f.revokeErr
}

func newAdminHandlerForRuntimeSettings(db *pgxpool.Pool) *AdminHandler {
	return NewAdminHandler(nil, settings.NewRuntimeSettingsService(settings.NewSystemConfigRepository(db)), nil)
}

func TestAdminHighRiskHandlersRejectUserPrincipal(t *testing.T) {
	gin.SetMode(gin.TestMode)
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
	gin.SetMode(gin.TestMode)
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
	gin.SetMode(gin.TestMode)
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
	gin.SetMode(gin.TestMode)
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

func TestAdminListUsersRejectsInvalidFilters(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := newUserRepoTestDB(t)
	repo := NewUserRepository(db)
	handler := NewAdminHandler(NewUserService(repo, nil, nil), nil, nil)
	router := gin.New()
	router.GET("/admin/users", handler.ListUsers)

	req := httptest.NewRequest(http.MethodGet, "/admin/users?role=owner", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "role 仅支持 admin 或 user") {
		t.Fatalf("body = %s, want role validation message", rec.Body.String())
	}
}

func TestAdminListUsersIncludesAvailableCreditBalance(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db, pool := testdb.OpenWithPool(t)
	repo := NewUserRepository(db)
	user := &User{
		DisplayName: "admin_balance_user",
		Role:        "user",
		Status:      1,
	}
	if err := repo.Create(t.Context(), user); err != nil {
		t.Fatalf("create user failed: %v", err)
	}

	creditService := credits.NewService(pool)
	tx, err := pool.Begin(t.Context())
	if err != nil {
		t.Fatalf("begin wallet transaction failed: %v", err)
	}
	if err := creditService.LifecycleInTx(tx).ProvisionWallet(t.Context(), user.ID); err != nil {
		_ = tx.Rollback(t.Context())
		t.Fatalf("provision wallet failed: %v", err)
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatalf("commit wallet transaction failed: %v", err)
	}
	amount, err := domain.NewCreditAmount(125)
	if err != nil {
		t.Fatalf("build credit amount failed: %v", err)
	}
	if _, err := creditService.Grant(t.Context(), ports.GrantCommand{
		UserID: user.ID,
		Amount: amount,
		Metadata: domain.CommandMetadata{
			IdempotencyKey: "admin-list-balance-user",
			Actor: domain.Actor{
				Type: domain.ActorAdmin,
				ID:   "admin-list-test",
			},
			Source: "admin_grant",
			Reason: "seed admin user balance",
			BusinessReference: domain.BusinessReference{
				Type: "test",
				ID:   "admin-list-balance-user",
			},
		},
	}); err != nil {
		t.Fatalf("grant credits failed: %v", err)
	}

	handler := NewAdminHandler(NewUserService(repo, nil, creditService), nil, nil)
	router := gin.New()
	router.GET("/admin/users", handler.ListUsers)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(
		recorder,
		httptest.NewRequest(http.MethodGet, "/admin/users?keyword=admin_balance_user", nil),
	)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	var response struct {
		Data struct {
			List []struct {
				PublicID         string          `json:"public_id"`
				DisplayName      string          `json:"display_name"`
				InternalID       json.RawMessage `json:"id"`
				AvailableCredits int64           `json:"available_credits"`
			} `json:"list"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	if len(response.Data.List) != 1 {
		t.Fatalf("list length = %d, want 1; body=%s", len(response.Data.List), recorder.Body.String())
	}
	if response.Data.List[0].PublicID != user.PublicID.String() || response.Data.List[0].DisplayName != user.DisplayName {
		t.Fatalf("public identity = (%q, %q), want (%q, %q)", response.Data.List[0].PublicID, response.Data.List[0].DisplayName, user.PublicID, user.DisplayName)
	}
	if response.Data.List[0].InternalID != nil {
		t.Fatalf("response exposed internal user id: %s", recorder.Body.String())
	}
	if response.Data.List[0].AvailableCredits != 125 {
		t.Fatalf(
			"available credits = %d, want 125; body=%s",
			response.Data.List[0].AvailableCredits,
			recorder.Body.String(),
		)
	}
}

func TestAdminListUsersHTTPContractComposesPageBalancesWithFiltersAndCounts(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db, pool := testdb.OpenWithPool(t)
	repo := NewUserRepository(db)
	creditService := credits.NewService(pool)
	users := []*User{
		createAdminListUserWithBalance(t, repo, pool, creditService, "contract_admin_active", "admin", 1, 11),
		createAdminListUserWithBalance(t, repo, pool, creditService, "contract_user_active", "user", 1, 22),
		createAdminListUserWithBalance(t, repo, pool, creditService, "contract_user_banned", "user", 0, 33),
		createAdminListUserWithBalance(t, repo, pool, creditService, "contract_user_active_second", "user", 1, 44),
	}

	handler := NewAdminHandler(NewUserService(repo, nil, creditService), nil, nil)
	router := gin.New()
	router.GET("/admin/users", handler.ListUsers)

	assertAdminListUsersResponse(t, router, "/admin/users?keyword=contract&page=1&page_size=2", adminUserListAssertion{
		Total: 3 + 1, ActiveCount: 3, BannedCount: 1, AdminCount: 1,
		Balances: map[string]int64{users[3].PublicID.String(): 44, users[2].PublicID.String(): 33},
	})
	assertAdminListUsersResponse(t, router, "/admin/users?keyword=contract&role=user&status=1&page=2&page_size=1", adminUserListAssertion{
		Total: 2, ActiveCount: 2, BannedCount: 0, AdminCount: 0,
		Balances: map[string]int64{users[1].PublicID.String(): 22},
	})
}

func TestAdminListUsersFailsWhenCreditWalletBalanceIsIncomplete(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db, pool := testdb.OpenWithPool(t)
	repo := NewUserRepository(db)
	creditService := credits.NewService(pool)

	tests := []struct {
		name           string
		usernameSuffix string
		setup          func(t *testing.T, user *User)
	}{
		{
			name:           "wallet is missing",
			usernameSuffix: "wallet_missing",
			setup: func(t *testing.T, user *User) {
				if _, err := pool.Exec(t.Context(), `
DELETE FROM credit_accounts
WHERE wallet_user_id = $1`, user.ID); err != nil {
					t.Fatalf("delete wallet accounts failed: %v", err)
				}
				if _, err := pool.Exec(t.Context(), `
DELETE FROM credit_wallets
WHERE user_id = $1`, user.ID); err != nil {
					t.Fatalf("delete wallet failed: %v", err)
				}
			},
		},
		{
			name:           "available balance projection is missing",
			usernameSuffix: "projection_missing",
			setup: func(t *testing.T, user *User) {
				tx, err := pool.Begin(t.Context())
				if err != nil {
					t.Fatalf("begin wallet transaction failed: %v", err)
				}
				if err := creditService.LifecycleInTx(tx).ProvisionWallet(t.Context(), user.ID); err != nil {
					_ = tx.Rollback(t.Context())
					t.Fatalf("provision wallet failed: %v", err)
				}
				if err := tx.Commit(t.Context()); err != nil {
					t.Fatalf("commit wallet transaction failed: %v", err)
				}
				if _, err := pool.Exec(t.Context(), `
DELETE FROM credit_accounts
WHERE wallet_user_id = $1
  AND account_role = 'user_available'`, user.ID); err != nil {
					t.Fatalf("delete available balance projection failed: %v", err)
				}
			},
		},
		{
			name:           "reserved balance projection is missing",
			usernameSuffix: "reserved_projection_missing",
			setup: func(t *testing.T, user *User) {
				provisionAdminListWallet(t, pool, creditService, user.ID)
				if _, err := pool.Exec(t.Context(), `
DELETE FROM credit_accounts
WHERE wallet_user_id = $1
  AND account_role = 'user_reserved'`, user.ID); err != nil {
					t.Fatalf("delete reserved balance projection failed: %v", err)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user := &User{
				DisplayName: "incomplete_wallet_" + tt.usernameSuffix,
				Role:        "user",
				Status:      1,
			}
			if err := repo.Create(t.Context(), user); err != nil {
				t.Fatalf("create user failed: %v", err)
			}
			tt.setup(t, user)

			handler := NewAdminHandler(NewUserService(repo, nil, creditService), nil, nil)
			router := gin.New()
			router.GET("/admin/users", handler.ListUsers)
			recorder := httptest.NewRecorder()
			router.ServeHTTP(
				recorder,
				httptest.NewRequest(http.MethodGet, "/admin/users?keyword="+user.DisplayName, nil),
			)

			if recorder.Code != http.StatusInternalServerError {
				t.Fatalf(
					"status = %d, want %d; incomplete wallet must not be represented as zero, body=%s",
					recorder.Code,
					http.StatusInternalServerError,
					recorder.Body.String(),
				)
			}
		})
	}
}

type adminUserListAssertion struct {
	Total       int64
	ActiveCount int64
	BannedCount int64
	AdminCount  int64
	Balances    map[string]int64
}

func assertAdminListUsersResponse(t *testing.T, router *gin.Engine, path string, want adminUserListAssertion) {
	t.Helper()
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("GET %s status = %d, want %d, body=%s", path, recorder.Code, http.StatusOK, recorder.Body.String())
	}
	var response struct {
		Data struct {
			Total       int64 `json:"total"`
			ActiveCount int64 `json:"active_count"`
			BannedCount int64 `json:"banned_count"`
			AdminCount  int64 `json:"admin_count"`
			List        []struct {
				PublicID         string          `json:"public_id"`
				InternalID       json.RawMessage `json:"id"`
				AvailableCredits int64           `json:"available_credits"`
			} `json:"list"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Data.Total != want.Total || response.Data.ActiveCount != want.ActiveCount || response.Data.BannedCount != want.BannedCount || response.Data.AdminCount != want.AdminCount {
		t.Fatalf("counts = %+v, want total=%d active=%d banned=%d admin=%d", response.Data, want.Total, want.ActiveCount, want.BannedCount, want.AdminCount)
	}
	if len(response.Data.List) != len(want.Balances) {
		t.Fatalf("list length = %d, want %d; body=%s", len(response.Data.List), len(want.Balances), recorder.Body.String())
	}
	for _, item := range response.Data.List {
		if item.InternalID != nil {
			t.Fatalf("response exposed internal user id: %s", recorder.Body.String())
		}
		wantBalance, ok := want.Balances[item.PublicID]
		if !ok || item.AvailableCredits != wantBalance {
			t.Fatalf("row public_id=%q balance=%d, want balances=%v", item.PublicID, item.AvailableCredits, want.Balances)
		}
	}
}

func createAdminListUserWithBalance(t *testing.T, repo *UserRepository, pool *pgxpool.Pool, creditService *credits.Service, displayName, role string, status int16, balance int64) *User {
	t.Helper()
	user := &User{DisplayName: displayName, Role: role, Status: status}
	if err := repo.Create(t.Context(), user); err != nil {
		t.Fatalf("create user %q failed: %v", displayName, err)
	}
	provisionAdminListWallet(t, pool, creditService, user.ID)
	if balance == 0 {
		return user
	}
	amount, err := domain.NewCreditAmount(balance)
	if err != nil {
		t.Fatalf("build credit amount failed: %v", err)
	}
	if _, err := creditService.Grant(t.Context(), ports.GrantCommand{
		UserID: user.ID,
		Amount: amount,
		Metadata: domain.CommandMetadata{
			IdempotencyKey:    "admin-list-contract-" + strconv.FormatInt(user.ID, 10),
			Actor:             domain.Actor{Type: domain.ActorAdmin, ID: "admin-list-test"},
			Source:            "admin_grant",
			Reason:            "seed admin user balance",
			BusinessReference: domain.BusinessReference{Type: "test", ID: strconv.FormatInt(user.ID, 10)},
		},
	}); err != nil {
		t.Fatalf("grant credits for %q failed: %v", displayName, err)
	}
	return user
}

func provisionAdminListWallet(t *testing.T, pool *pgxpool.Pool, creditService *credits.Service, userID int64) {
	t.Helper()
	tx, err := pool.Begin(t.Context())
	if err != nil {
		t.Fatalf("begin wallet transaction failed: %v", err)
	}
	if err := creditService.LifecycleInTx(tx).ProvisionWallet(t.Context(), userID); err != nil {
		_ = tx.Rollback(t.Context())
		t.Fatalf("provision wallet failed: %v", err)
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatalf("commit wallet transaction failed: %v", err)
	}
}

func TestAdminRuntimeSettingsGetFailsWhenRepositoryUnavailable(t *testing.T) {
	gin.SetMode(gin.TestMode)

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
	gin.SetMode(gin.TestMode)
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
	gin.SetMode(gin.TestMode)

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
	gin.SetMode(gin.TestMode)

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
	gin.SetMode(gin.TestMode)

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
	gin.SetMode(gin.TestMode)

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
	gin.SetMode(gin.TestMode)

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
	gin.SetMode(gin.TestMode)

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

func TestAdminDeleteUserContinuesLocalCleanupWhenAuthUserAlreadyMissing(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := newUserRepoTestDB(t)
	repo := NewUserRepository(db)
	authUserID := "missing-auth-user"
	targetUser := &User{
		DisplayName: "stale_user",
		AuthUserID:  &authUserID,
		Role:        "user",
		Status:      1,
	}
	if err := repo.Create(context.Background(), targetUser); err != nil {
		t.Fatalf("create target user failed: %v", err)
	}
	authManager := &fakeAdminAuthManager{deleteErr: autherrors.ErrAuthUserNotFound}
	lifecycle := &recordingUserLifecycle{}
	lifecycle.deleteFn = func(ctx context.Context, userID int64, authDeleter AuthUserDeleter, uploadCleaner UserUploadCleaner) error {
		if err := authDeleter.DeleteUser(ctx, authUserID); err != nil && !errors.Is(err, autherrors.ErrAuthUserNotFound) {
			return err
		}
		lifecycle.deleteErr = nil
		return uploadCleaner.DeleteUserUploads(ctx, userID, targetUser.StorageUID)
	}
	userService := NewUserService(repo, lifecycle, nil, &recordingUserUploadCleaner{})
	handler := NewAdminHandler(userService, nil, NewIdentityAdminCommands(userService, authManager, nil))
	router := gin.New()
	router.DELETE("/admin/users/:public_id", func(c *gin.Context) {
		testsupport.SetPrincipal(t, c, 999, principal.RoleAdmin)
		handler.DeleteUser(c)
	})

	req := httptest.NewRequest(http.MethodDelete, "/admin/users/"+targetUser.PublicID.String(), nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if authManager.deletedAuthUserID != authUserID {
		t.Fatalf("deleted auth user id = %q, want %q", authManager.deletedAuthUserID, authUserID)
	}

	if len(lifecycle.deleteCalls) != 1 || lifecycle.deleteCalls[0] != targetUser.ID {
		t.Fatalf("lifecycle delete calls = %v, want one call for user %d", lifecycle.deleteCalls, targetUser.ID)
	}
}

func TestAdminDeleteUserReportsLocalCleanupFailureAfterAuthDelete(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := newUserRepoTestDB(t)
	repo := NewUserRepository(db)
	authUserID := "auth-user-kept"
	targetUser := &User{
		DisplayName: "cleanup_fail_user",
		AuthUserID:  &authUserID,
		StorageUID:  "FAIL42",
		Role:        "user",
		Status:      1,
	}
	if err := repo.Create(context.Background(), targetUser); err != nil {
		t.Fatalf("create target user failed: %v", err)
	}

	authManager := &fakeAdminAuthManager{}
	lifecycle := &recordingUserLifecycle{}
	lifecycle.deleteFn = func(ctx context.Context, userID int64, authDeleter AuthUserDeleter, uploadCleaner UserUploadCleaner) error {
		if err := authDeleter.DeleteUser(ctx, authUserID); err != nil && !errors.Is(err, autherrors.ErrAuthUserNotFound) {
			return err
		}
		return uploadCleaner.DeleteUserUploads(ctx, userID, targetUser.StorageUID)
	}
	userService := NewUserService(
		repo,
		lifecycle,
		nil,
		&recordingUserUploadCleaner{err: errors.New("storage down")},
	)
	handler := NewAdminHandler(userService, nil, NewIdentityAdminCommands(userService, authManager, nil))
	router := gin.New()
	router.DELETE("/admin/users/:public_id", func(c *gin.Context) {
		testsupport.SetPrincipal(t, c, 999, principal.RoleAdmin)
		handler.DeleteUser(c)
	})

	req := httptest.NewRequest(http.MethodDelete, "/admin/users/"+targetUser.PublicID.String(), nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusInternalServerError, rec.Body.String())
	}
	if authManager.deletedAuthUserID != authUserID {
		t.Fatalf("auth delete was called for %q, want %q", authManager.deletedAuthUserID, authUserID)
	}
	if _, err := repo.GetByID(context.Background(), targetUser.ID); err != nil {
		t.Fatalf("target user should remain after cleanup failure: %v", err)
	}
}

func TestAdminDeleteUserPreservesLocalUserWhenAuthDeleteFails(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := newUserRepoTestDB(t)
	repo := NewUserRepository(db)
	authUserID := "auth-delete-fails"
	targetUser := &User{
		DisplayName: "auth_delete_fail_user",
		AuthUserID:  &authUserID,
		Role:        "user",
		Status:      1,
	}
	if err := repo.Create(context.Background(), targetUser); err != nil {
		t.Fatalf("create target user failed: %v", err)
	}

	authManager := &fakeAdminAuthManager{deleteErr: errors.New("auth unavailable")}
	lifecycle := &recordingUserLifecycle{}
	lifecycle.deleteFn = func(ctx context.Context, _ int64, authDeleter AuthUserDeleter, _ UserUploadCleaner) error {
		return authDeleter.DeleteUser(ctx, authUserID)
	}
	userService := NewUserService(repo, lifecycle, nil, &recordingUserUploadCleaner{})
	handler := NewAdminHandler(userService, nil, NewIdentityAdminCommands(userService, authManager, nil))
	router := gin.New()
	router.DELETE("/admin/users/:public_id", func(c *gin.Context) {
		testsupport.SetPrincipal(t, c, 999, principal.RoleAdmin)
		handler.DeleteUser(c)
	})

	req := httptest.NewRequest(http.MethodDelete, "/admin/users/"+targetUser.PublicID.String(), nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusInternalServerError, rec.Body.String())
	}
	if authManager.deletedAuthUserID != authUserID {
		t.Fatalf("deleted auth user id = %q, want %q", authManager.deletedAuthUserID, authUserID)
	}
	if _, err := repo.GetByID(context.Background(), targetUser.ID); err != nil {
		t.Fatalf("local user changed after Auth delete failure: %v", err)
	}
}

func TestAdminBanUserCommitsBusinessFirstWithSyncPending(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := newUserRepoTestDB(t)
	repo := NewUserRepository(db)
	authUserID := "auth-ban-user"
	targetUser := &User{
		DisplayName: "target_user",
		AuthUserID:  &authUserID,
		Role:        "user",
		Status:      1,
	}
	if err := repo.Create(context.Background(), targetUser); err != nil {
		t.Fatalf("create target user failed: %v", err)
	}

	authManager := &fakeAdminAuthManager{}
	lifecycle := &recordingUserLifecycle{}
	userService := NewUserService(repo, lifecycle, nil)
	handler := NewAdminHandler(userService, nil, NewIdentityAdminCommands(userService, authManager, authsync.NewStore(db)))
	router := gin.New()
	router.POST("/admin/users/:public_id/ban", func(c *gin.Context) {
		testsupport.SetPrincipal(t, c, 999, principal.RoleAdmin)
		handler.BanUser(c)
	})

	req := httptest.NewRequest(http.MethodPost, "/admin/users/"+targetUser.PublicID.String()+"/ban", strings.NewReader(`{"reason":"abuse","expires_in_days":7}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if body := rec.Body.String(); !strings.Contains(body, `"auth_sync_pending":true`) {
		t.Fatalf("response body = %s, want auth_sync_pending:true (business committed, Auth converging)", body)
	}
	// Business-first: no Auth I/O on the request path, so the fake Auth
	// client must stay untouched even though it is healthy.
	if authManager.bannedAuthUserID != "" {
		t.Fatalf("banned auth user id = %q, want no request-path Auth call", authManager.bannedAuthUserID)
	}
	if len(lifecycle.statusCalls) != 1 || lifecycle.statusCalls[0] != 0 {
		t.Fatalf("status lifecycle calls = %v, want [0]", lifecycle.statusCalls)
	}
	var pending int
	if err := db.QueryRow(context.Background(), `SELECT count(*) FROM identity_auth_sync_intents WHERE user_id = $1 AND status = 'pending'`, targetUser.ID).Scan(&pending); err != nil {
		t.Fatalf("count pending intents: %v", err)
	}
	if pending != 1 {
		t.Fatalf("pending intents = %d, want exactly 1 durable retry", pending)
	}
}

func TestAdminRevokeUserSessionsUsesAuthService(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := newUserRepoTestDB(t)
	repo := NewUserRepository(db)
	authUserID := "auth-revoke-user"
	targetUser := &User{
		DisplayName: "target_user",
		AuthUserID:  &authUserID,
		Role:        "user",
		Status:      1,
	}
	if err := repo.Create(context.Background(), targetUser); err != nil {
		t.Fatalf("create target user failed: %v", err)
	}

	authManager := &fakeAdminAuthManager{}
	userService := NewUserService(repo, nil, nil)
	handler := NewAdminHandler(userService, nil, NewIdentityAdminCommands(userService, authManager, nil))
	router := gin.New()
	router.POST("/admin/users/:public_id/revoke-sessions", func(c *gin.Context) {
		testsupport.SetPrincipal(t, c, 999, principal.RoleAdmin)
		handler.RevokeUserSessions(c)
	})

	req := httptest.NewRequest(http.MethodPost, "/admin/users/"+targetUser.PublicID.String()+"/revoke-sessions", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if authManager.revokedAuthUserID != authUserID {
		t.Fatalf("revoked auth user id = %q, want %q", authManager.revokedAuthUserID, authUserID)
	}
}

func TestAdminImpersonateUserUsesPublicIDAndForwardsSession(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := newUserRepoTestDB(t)
	repo := NewUserRepository(db)
	authUserID := "auth-impersonated"
	targetUser := &User{DisplayName: "impersonated user", AuthUserID: &authUserID, Role: "user", Status: 1}
	if err := repo.Create(context.Background(), targetUser); err != nil {
		t.Fatalf("create target user failed: %v", err)
	}
	impersonator := &fakeAdminUserImpersonator{setCookies: []string{"session=impersonated; Path=/; HttpOnly"}}
	userService := NewUserService(repo, nil, nil)
	handler := NewAdminHandler(userService, nil, nil, impersonator)
	router := gin.New()
	router.POST("/admin/users/:public_id/impersonate", func(c *gin.Context) {
		testsupport.SetPrincipal(t, c, 999, principal.RoleAdmin)
		handler.ImpersonateUser(c)
	})
	req := httptest.NewRequest(http.MethodPost, "/admin/users/"+targetUser.PublicID.String()+"/impersonate", nil)
	req.Header.Set("Cookie", "session=admin")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	if impersonator.authUserID != authUserID || impersonator.cookieHeader != "session=admin" {
		t.Fatalf("bridge args = auth=%q cookie=%q", impersonator.authUserID, impersonator.cookieHeader)
	}
	if got := rec.Header().Values("Set-Cookie"); len(got) != 1 || got[0] != impersonator.setCookies[0] {
		t.Fatalf("Set-Cookie = %#v, want %#v", got, impersonator.setCookies)
	}
}
