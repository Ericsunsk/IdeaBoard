package users

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"ideaboard/api/internal/autherrors"
	"ideaboard/api/internal/platform/database"

	"github.com/gin-gonic/gin"
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

func TestAdminListUsersRejectsInvalidFilters(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := newUserRepoTestDB(t)
	repo := NewUserRepository(db)
	handler := NewAdminHandler(NewUserService(repo), nil, nil)
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
	if _, err := db.ExecContext(
		context.Background(),
		`INSERT INTO tasks (task_id, user_id, prompt, user_prompt, params, status, error_msg, result) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		"admin-delete-task", targetUser.ID, "", "", "", "pending", "", "",
	); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}

	authManager := &fakeAdminAuthManager{deleteErr: autherrors.ErrAuthUserNotFound}
	handler := NewAdminHandler(NewUserService(repo), nil, authManager)
	router := gin.New()
	router.DELETE("/admin/users/:id", func(c *gin.Context) {
		c.Set("user_id", int64(999))
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

	if _, err := repo.GetByID(context.Background(), targetUser.ID); !database.IsNotFound(err) {
		t.Fatalf("target user lookup err = %v, want record not found", err)
	}
	var taskCount int64
	if err := db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM tasks WHERE user_id = $1`, targetUser.ID).Scan(&taskCount); err != nil {
		t.Fatalf("count tasks failed: %v", err)
	}
	if taskCount != 0 {
		t.Fatalf("task count = %d, want 0", taskCount)
	}
}

func TestAdminDeleteUserSkipsAuthDeleteWhenLocalCleanupFails(t *testing.T) {
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
	userService := NewUserService(repo, &recordingUserUploadCleaner{err: errors.New("storage down")})
	handler := NewAdminHandler(userService, nil, authManager)
	router := gin.New()
	router.DELETE("/admin/users/:id", func(c *gin.Context) {
		c.Set("user_id", int64(999))
		handler.DeleteUser(c)
	})

	req := httptest.NewRequest(http.MethodDelete, "/admin/users/"+strconv.FormatInt(targetUser.ID, 10), nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusInternalServerError, rec.Body.String())
	}
	if authManager.deletedAuthUserID != "" {
		t.Fatalf("auth delete was called for %q, want skipped", authManager.deletedAuthUserID)
	}
	if _, err := repo.GetByID(context.Background(), targetUser.ID); err != nil {
		t.Fatalf("target user should remain after cleanup failure: %v", err)
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
	handler := NewAdminHandler(NewUserService(repo), nil, authManager)
	router := gin.New()
	router.POST("/admin/users/:id/ban", func(c *gin.Context) {
		c.Set("user_id", int64(999))
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

	updated, err := repo.GetByID(context.Background(), targetUser.ID)
	if err != nil {
		t.Fatalf("reload target user failed: %v", err)
	}
	if updated.Status != 0 {
		t.Fatalf("updated status = %d, want 0", updated.Status)
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
	handler := NewAdminHandler(NewUserService(repo), nil, authManager)
	router := gin.New()
	router.POST("/admin/users/:id/revoke-sessions", func(c *gin.Context) {
		c.Set("user_id", int64(999))
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
