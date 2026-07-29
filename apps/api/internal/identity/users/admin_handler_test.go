package users

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"ideaboard/api/internal/credits"
	"ideaboard/api/internal/identity/autherrors"
	"ideaboard/api/internal/identity/principal"
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

type adminBrandingSettingsResponse struct {
	Code int                      `json:"code"`
	Msg  string                   `json:"msg"`
	Data settings.BrandingSetting `json:"data"`
}

type adminTaskMonitorSettingsResponse struct {
	Code int                         `json:"code"`
	Msg  string                      `json:"msg"`
	Data settings.TaskMonitorSetting `json:"data"`
}

type adminPublicURLSettingsResponse struct {
	Code int                       `json:"code"`
	Msg  string                    `json:"msg"`
	Data settings.PublicURLSetting `json:"data"`
}

type adminMediaUploadSettingsResponse struct {
	Code int                         `json:"code"`
	Msg  string                      `json:"msg"`
	Data settings.MediaUploadSetting `json:"data"`
}

type adminOperationalSettingsResponse struct {
	Code int                               `json:"code"`
	Msg  string                            `json:"msg"`
	Data settings.OperationalSettingsState `json:"data"`
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

func newAdminHandlerForRuntimeSettings(db *sql.DB) *AdminHandler {
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
		{name: "operational settings", method: http.MethodPut, path: "/settings/operational", route: "/settings/operational", run: handler.UpdateOperationalSettings},
		{name: "role", method: http.MethodPut, path: "/users/1/role", route: "/users/:id/role", run: handler.UpdateUserRole},
		{name: "delete", method: http.MethodDelete, path: "/users/1", route: "/users/:id", run: handler.DeleteUser},
		{name: "ban", method: http.MethodPost, path: "/users/1/ban", route: "/users/:id/ban", run: handler.BanUser},
		{name: "unban", method: http.MethodPost, path: "/users/1/unban", route: "/users/:id/unban", run: handler.UnbanUser},
		{name: "revoke sessions", method: http.MethodPost, path: "/users/1/revoke", route: "/users/:id/revoke", run: handler.RevokeUserSessions},
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
		Username: "admin_balance_user",
		Role:     "user",
		Status:   1,
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
	amount, err := credits.NewCreditAmount(125)
	if err != nil {
		t.Fatalf("build credit amount failed: %v", err)
	}
	if _, err := creditService.Grant(t.Context(), credits.GrantCommand{
		UserID: user.ID,
		Amount: amount,
		Metadata: credits.CommandMetadata{
			IdempotencyKey: "admin-list-balance-user",
			Actor: credits.Actor{
				Type: credits.ActorAdmin,
				ID:   "admin-list-test",
			},
			Source: "admin_grant",
			Reason: "seed admin user balance",
			BusinessReference: credits.BusinessReference{
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
				ID               int64 `json:"id"`
				AvailableCredits int64 `json:"available_credits"`
			} `json:"list"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	if len(response.Data.List) != 1 {
		t.Fatalf("list length = %d, want 1; body=%s", len(response.Data.List), recorder.Body.String())
	}
	if response.Data.List[0].ID != user.ID {
		t.Fatalf("user id = %d, want %d", response.Data.List[0].ID, user.ID)
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
		Balances: map[int64]int64{users[3].ID: 44, users[2].ID: 33},
	})
	assertAdminListUsersResponse(t, router, "/admin/users?keyword=contract&role=user&status=1&page=2&page_size=1", adminUserListAssertion{
		Total: 2, ActiveCount: 2, BannedCount: 0, AdminCount: 0,
		Balances: map[int64]int64{users[1].ID: 22},
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
			setup: func(_ *testing.T, _ *User) {
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
				Username: "incomplete_wallet_" + tt.usernameSuffix,
				Role:     "user",
				Status:   1,
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
				httptest.NewRequest(http.MethodGet, "/admin/users?keyword="+user.Username, nil),
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
	Balances    map[int64]int64
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
				ID               int64 `json:"id"`
				AvailableCredits int64 `json:"available_credits"`
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
		wantBalance, ok := want.Balances[item.ID]
		if !ok || item.AvailableCredits != wantBalance {
			t.Fatalf("row id=%d balance=%d, want balances=%v", item.ID, item.AvailableCredits, want.Balances)
		}
	}
}

func createAdminListUserWithBalance(t *testing.T, repo *UserRepository, pool *pgxpool.Pool, creditService *credits.Service, username, role string, status int16, balance int64) *User {
	t.Helper()
	user := &User{Username: username, Role: role, Status: status}
	if err := repo.Create(t.Context(), user); err != nil {
		t.Fatalf("create user %q failed: %v", username, err)
	}
	provisionAdminListWallet(t, pool, creditService, user.ID)
	if balance == 0 {
		return user
	}
	amount, err := credits.NewCreditAmount(balance)
	if err != nil {
		t.Fatalf("build credit amount failed: %v", err)
	}
	if _, err := creditService.Grant(t.Context(), credits.GrantCommand{
		UserID: user.ID,
		Amount: amount,
		Metadata: credits.CommandMetadata{
			IdempotencyKey:    "admin-list-contract-" + strconv.FormatInt(user.ID, 10),
			Actor:             credits.Actor{Type: credits.ActorAdmin, ID: "admin-list-test"},
			Source:            "admin_grant",
			Reason:            "seed admin user balance",
			BusinessReference: credits.BusinessReference{Type: "test", ID: strconv.FormatInt(user.ID, 10)},
		},
	}); err != nil {
		t.Fatalf("grant credits for %q failed: %v", username, err)
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
			name: "public URLs",
			path: "/admin/settings/public-urls",
			register: func(router *gin.Engine) {
				router.GET("/admin/settings/public-urls", handler.GetPublicURLSettings)
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
			name: "task monitor",
			path: "/admin/settings/task-monitor",
			register: func(router *gin.Engine) {
				router.GET("/admin/settings/task-monitor", handler.GetTaskMonitorSettings)
			},
		},
		{
			name: "operational",
			path: "/admin/settings/operational",
			register: func(router *gin.Engine) {
				router.GET("/admin/settings/operational", handler.GetOperationalSettings)
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

func TestAdminOperationalSettingsUpdateRequiresActorReasonAndReturnsEffectiveState(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := newUserRepoTestDB(t)
	service := settings.NewRuntimeSettingsService(settings.NewSystemConfigRepository(db))
	service.SetEffectiveOperationalSettings(settings.DefaultTaskRuntimeSetting(), settings.DefaultEventRetentionSetting())
	handler := NewAdminHandler(nil, service, nil)

	validBody := `{
  "task_runtime":{"worker_concurrency":12,"task_timeout_seconds":240,"no_capacity_retry_seconds":9,"max_retry":31},
  "outbound_policy":{"allow_hosts":["internal.example.com:8443"]},
  "event_retention":{"nats_retention_days":30},
  "reason":"capacity tuning"
}`

	t.Run("missing actor", func(t *testing.T) {
		router := gin.New()
		router.PUT("/admin/settings/operational", handler.UpdateOperationalSettings)
		req := httptest.NewRequest(http.MethodPut, "/admin/settings/operational", strings.NewReader(validBody))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusUnauthorized, rec.Body.String())
		}
	})

	t.Run("missing reason", func(t *testing.T) {
		router := gin.New()
		router.Use(func(c *gin.Context) { testsupport.SetPrincipal(t, c, 42, principal.RoleAdmin); c.Next() })
		router.PUT("/admin/settings/operational", handler.UpdateOperationalSettings)
		body := strings.Replace(validBody, `"reason":"capacity tuning"`, `"reason":""`, 1)
		req := httptest.NewRequest(http.MethodPut, "/admin/settings/operational", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "reason") {
			t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("valid update", func(t *testing.T) {
		router := gin.New()
		router.Use(func(c *gin.Context) { testsupport.SetPrincipal(t, c, 42, principal.RoleAdmin); c.Next() })
		router.PUT("/admin/settings/operational", handler.UpdateOperationalSettings)
		req := httptest.NewRequest(http.MethodPut, "/admin/settings/operational", strings.NewReader(validBody))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
		}
		var payload adminOperationalSettingsResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if !payload.Data.TaskRuntime.RestartRequired || payload.Data.OutboundPolicy.RestartRequired || !payload.Data.EventRetention.RestartRequired {
			t.Fatalf("operational state = %+v", payload.Data)
		}
		if payload.Data.TaskRuntime.Effective != settings.DefaultTaskRuntimeSetting() {
			t.Fatalf("effective task runtime = %+v", payload.Data.TaskRuntime.Effective)
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

func TestAdminTaskMonitorSettingsDefaultsWhenConfigMissing(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := newUserRepoTestDB(t)
	handler := newAdminHandlerForRuntimeSettings(db)
	router := gin.New()
	router.GET("/admin/settings/task-monitor", handler.GetTaskMonitorSettings)

	req := httptest.NewRequest(http.MethodGet, "/admin/settings/task-monitor", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got adminTaskMonitorSettingsResponse
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	if got.Data != settings.DefaultTaskMonitorSetting() {
		t.Fatalf("task monitor settings = %+v, want defaults", got.Data)
	}
}

func TestAdminTaskMonitorSettingsUpdatePersists(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := newUserRepoTestDB(t)
	handler := newAdminHandlerForRuntimeSettings(db)
	router := gin.New()
	router.GET("/admin/settings/task-monitor", handler.GetTaskMonitorSettings)
	router.PUT("/admin/settings/task-monitor", handler.UpdateTaskMonitorSettings)

	req := httptest.NewRequest(http.MethodPut, "/admin/settings/task-monitor", strings.NewReader(`{"list_page_size":24,"auto_refresh_enabled":false,"refresh_interval_seconds":10,"result_preview_enabled":false}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var updated adminTaskMonitorSettingsResponse
	if err := json.NewDecoder(rec.Body).Decode(&updated); err != nil {
		t.Fatalf("decode update response failed: %v", err)
	}
	want := settings.TaskMonitorSetting{ListPageSize: 24, AutoRefreshEnabled: false, RefreshIntervalSeconds: 10, ResultPreviewEnabled: false}
	if updated.Data != want {
		t.Fatalf("updated task monitor settings = %+v, want %+v", updated.Data, want)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/admin/settings/task-monitor", nil)
	getRec := httptest.NewRecorder()
	router.ServeHTTP(getRec, getReq)

	var got adminTaskMonitorSettingsResponse
	if err := json.NewDecoder(getRec.Body).Decode(&got); err != nil {
		t.Fatalf("decode get response failed: %v", err)
	}
	if got.Data != want {
		t.Fatalf("persisted task monitor settings = %+v, want %+v", got.Data, want)
	}
}

func TestAdminTaskMonitorSettingsRejectsInvalidPayload(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := newUserRepoTestDB(t)
	handler := newAdminHandlerForRuntimeSettings(db)
	router := gin.New()
	router.PUT("/admin/settings/task-monitor", handler.UpdateTaskMonitorSettings)

	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "page size too small",
			body: `{"list_page_size":4,"auto_refresh_enabled":true,"refresh_interval_seconds":4,"result_preview_enabled":true}`,
			want: "list_page_size 必须在 5 到 50 之间",
		},
		{
			name: "refresh interval too small",
			body: `{"list_page_size":12,"auto_refresh_enabled":true,"refresh_interval_seconds":1,"result_preview_enabled":true}`,
			want: "refresh_interval_seconds 必须在 2 到 60 秒之间",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPut, "/admin/settings/task-monitor", strings.NewReader(tt.body))
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

func TestAdminPublicURLSettingsDefaultsWhenConfigMissing(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := newUserRepoTestDB(t)
	handler := newAdminHandlerForRuntimeSettings(db)
	router := gin.New()
	router.GET("/admin/settings/public-urls", handler.GetPublicURLSettings)

	req := httptest.NewRequest(http.MethodGet, "/admin/settings/public-urls", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got adminPublicURLSettingsResponse
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	want := settings.PublicURLSetting{}
	if got.Data != want {
		t.Fatalf("public URL settings = %+v, want %+v", got.Data, want)
	}
}

func TestAdminPublicURLSettingsUpdateNormalizesAndPersists(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := newUserRepoTestDB(t)
	handler := newAdminHandlerForRuntimeSettings(db)
	router := gin.New()
	router.GET("/admin/settings/public-urls", handler.GetPublicURLSettings)
	router.PUT("/admin/settings/public-urls", handler.UpdatePublicURLSettings)

	req := httptest.NewRequest(http.MethodPut, "/admin/settings/public-urls", strings.NewReader(`{"public_base_url":"  https://app.example.com/  ","public_asset_base_url":""}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var updated adminPublicURLSettingsResponse
	if err := json.NewDecoder(rec.Body).Decode(&updated); err != nil {
		t.Fatalf("decode update response failed: %v", err)
	}
	want := settings.PublicURLSetting{PublicBaseURL: "https://app.example.com", PublicAssetBaseURL: ""}
	if updated.Data != want {
		t.Fatalf("updated public URL settings = %+v, want %+v", updated.Data, want)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/admin/settings/public-urls", nil)
	getRec := httptest.NewRecorder()
	router.ServeHTTP(getRec, getReq)

	var got adminPublicURLSettingsResponse
	if err := json.NewDecoder(getRec.Body).Decode(&got); err != nil {
		t.Fatalf("decode get response failed: %v", err)
	}
	if got.Data != want {
		t.Fatalf("persisted public URL settings = %+v, want %+v", got.Data, want)
	}
}

func TestAdminPublicURLSettingsRejectsInvalidPayload(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := newUserRepoTestDB(t)
	handler := newAdminHandlerForRuntimeSettings(db)
	router := gin.New()
	router.PUT("/admin/settings/public-urls", handler.UpdatePublicURLSettings)

	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "invalid public base URL",
			body: `{"public_base_url":"javascript:alert(1)","public_asset_base_url":""}`,
			want: "public_base_url 必须为空或 http(s) URL",
		},
		{
			name: "invalid public asset base URL",
			body: `{"public_base_url":"","public_asset_base_url":"ftp://assets.example.com"}`,
			want: "public_asset_base_url 必须为空或 http(s) URL",
		},
		{
			name: "public base URL with credentials",
			body: `{"public_base_url":"https://user:pass@app.example.com","public_asset_base_url":""}`,
			want: "public_base_url 必须为空或 http(s) URL",
		},
		{
			name: "public base URL with query",
			body: `{"public_base_url":"https://app.example.com?token=1","public_asset_base_url":""}`,
			want: "public_base_url 必须为空或 http(s) URL",
		},
		{
			name: "public asset URL with fragment",
			body: `{"public_base_url":"","public_asset_base_url":"https://assets.example.com#uploads"}`,
			want: "public_asset_base_url 必须为空或 http(s) URL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPut, "/admin/settings/public-urls", strings.NewReader(tt.body))
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
		Username:   "stale_user",
		AuthUserID: &authUserID,
		Role:       "user",
		Status:     1,
	}
	if err := repo.Create(context.Background(), targetUser); err != nil {
		t.Fatalf("create target user failed: %v", err)
	}
	authManager := &fakeAdminAuthManager{deleteErr: autherrors.ErrAuthUserNotFound}
	lifecycle := &recordingUserLifecycle{progress: DeletionProgress{UserID: targetUser.ID, AuthUserID: authUserID, StorageUID: targetUser.StorageUID, Phase: DeletionPendingAuth}}
	userService := NewUserService(repo, lifecycle, nil, &recordingUserUploadCleaner{})
	handler := NewAdminHandler(userService, nil, NewIdentityAdminCommands(userService, authManager))
	router := gin.New()
	router.DELETE("/admin/users/:id", func(c *gin.Context) {
		testsupport.SetPrincipal(t, c, 999, principal.RoleAdmin)
		handler.DeleteUser(c)
	})

	req := httptest.NewRequest(http.MethodDelete, "/admin/users/"+strconv.FormatInt(targetUser.ID, 10), nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if authManager.deletedAuthUserID != authUserID {
		t.Fatalf("deleted auth user id = %q, want %q", authManager.deletedAuthUserID, authUserID)
	}

	if lifecycle.tombstoneCalls != 1 {
		t.Fatalf("tombstone calls = %d, want 1", lifecycle.tombstoneCalls)
	}
}

func TestAdminDeleteUserReportsLocalCleanupFailureAfterAuthDelete(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := newUserRepoTestDB(t)
	repo := NewUserRepository(db)
	authUserID := "auth-user-kept"
	targetUser := &User{
		Username:   "cleanup_fail_user",
		AuthUserID: &authUserID,
		StorageUID: "FAIL42",
		Role:       "user",
		Status:     1,
	}
	if err := repo.Create(context.Background(), targetUser); err != nil {
		t.Fatalf("create target user failed: %v", err)
	}

	authManager := &fakeAdminAuthManager{}
	lifecycle := &recordingUserLifecycle{progress: DeletionProgress{UserID: targetUser.ID, AuthUserID: authUserID, StorageUID: targetUser.StorageUID, Phase: DeletionPendingAuth}}
	userService := NewUserService(
		repo,
		lifecycle,
		nil,
		&recordingUserUploadCleaner{err: errors.New("storage down")},
	)
	handler := NewAdminHandler(userService, nil, NewIdentityAdminCommands(userService, authManager))
	router := gin.New()
	router.DELETE("/admin/users/:id", func(c *gin.Context) {
		testsupport.SetPrincipal(t, c, 999, principal.RoleAdmin)
		handler.DeleteUser(c)
	})

	req := httptest.NewRequest(http.MethodDelete, "/admin/users/"+strconv.FormatInt(targetUser.ID, 10), nil)
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
		Username:   "auth_delete_fail_user",
		AuthUserID: &authUserID,
		Role:       "user",
		Status:     1,
	}
	if err := repo.Create(context.Background(), targetUser); err != nil {
		t.Fatalf("create target user failed: %v", err)
	}

	authManager := &fakeAdminAuthManager{deleteErr: errors.New("auth unavailable")}
	lifecycle := &recordingUserLifecycle{progress: DeletionProgress{UserID: targetUser.ID, AuthUserID: authUserID, StorageUID: targetUser.StorageUID, Phase: DeletionPendingAuth}}
	userService := NewUserService(repo, lifecycle, nil, &recordingUserUploadCleaner{})
	handler := NewAdminHandler(userService, nil, NewIdentityAdminCommands(userService, authManager))
	router := gin.New()
	router.DELETE("/admin/users/:id", func(c *gin.Context) {
		testsupport.SetPrincipal(t, c, 999, principal.RoleAdmin)
		handler.DeleteUser(c)
	})

	req := httptest.NewRequest(http.MethodDelete, "/admin/users/"+strconv.FormatInt(targetUser.ID, 10), nil)
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

func TestAdminBanUserOrchestratesAuthBeforeLocalStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := newUserRepoTestDB(t)
	repo := NewUserRepository(db)
	authUserID := "auth-ban-user"
	targetUser := &User{
		Username:   "target_user",
		AuthUserID: &authUserID,
		Role:       "user",
		Status:     1,
	}
	if err := repo.Create(context.Background(), targetUser); err != nil {
		t.Fatalf("create target user failed: %v", err)
	}

	authManager := &fakeAdminAuthManager{}
	lifecycle := &recordingUserLifecycle{}
	userService := NewUserService(repo, lifecycle, nil)
	handler := NewAdminHandler(userService, nil, NewIdentityAdminCommands(userService, authManager))
	router := gin.New()
	router.POST("/admin/users/:id/ban", func(c *gin.Context) {
		testsupport.SetPrincipal(t, c, 999, principal.RoleAdmin)
		handler.BanUser(c)
	})

	req := httptest.NewRequest(http.MethodPost, "/admin/users/"+strconv.FormatInt(targetUser.ID, 10)+"/ban", strings.NewReader(`{"reason":"abuse","expires_in_days":7}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if authManager.bannedAuthUserID != authUserID {
		t.Fatalf("banned auth user id = %q, want %q", authManager.bannedAuthUserID, authUserID)
	}

	if len(lifecycle.statusCalls) != 1 || lifecycle.statusCalls[0] != 0 {
		t.Fatalf("status lifecycle calls = %v, want [0]", lifecycle.statusCalls)
	}
}

func TestAdminRevokeUserSessionsUsesAuthService(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := newUserRepoTestDB(t)
	repo := NewUserRepository(db)
	authUserID := "auth-revoke-user"
	targetUser := &User{
		Username:   "target_user",
		AuthUserID: &authUserID,
		Role:       "user",
		Status:     1,
	}
	if err := repo.Create(context.Background(), targetUser); err != nil {
		t.Fatalf("create target user failed: %v", err)
	}

	authManager := &fakeAdminAuthManager{}
	userService := NewUserService(repo, nil, nil)
	handler := NewAdminHandler(userService, nil, NewIdentityAdminCommands(userService, authManager))
	router := gin.New()
	router.POST("/admin/users/:id/revoke-sessions", func(c *gin.Context) {
		testsupport.SetPrincipal(t, c, 999, principal.RoleAdmin)
		handler.RevokeUserSessions(c)
	})

	req := httptest.NewRequest(http.MethodPost, "/admin/users/"+strconv.FormatInt(targetUser.ID, 10)+"/revoke-sessions", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if authManager.revokedAuthUserID != authUserID {
		t.Fatalf("revoked auth user id = %q, want %q", authManager.revokedAuthUserID, authUserID)
	}
}
