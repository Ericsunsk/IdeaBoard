package tests

import (
	"encoding/json"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"ideaboard/api/internal/credits"
	"ideaboard/api/internal/credits/domain"
	"ideaboard/api/internal/credits/ports"
	"ideaboard/api/internal/testdb"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestAdminListUsersRejectsInvalidFilters(t *testing.T) {

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
