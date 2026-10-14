package tests

import (
	"context"
	"errors"
	"ideaboard/api/internal/identity/autherrors"
	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/identity/users/application/authsync"
	"ideaboard/api/internal/testsupport"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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

func (f *fakeAdminAuthManager) BanUser(_ context.Context, authUserID string, _ int) error {
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

func TestAdminDeleteUserContinuesLocalCleanupWhenAuthUserAlreadyMissing(t *testing.T) {

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

	req := httptest.NewRequest(http.MethodPost, "/admin/users/"+targetUser.PublicID.String()+"/ban", strings.NewReader(`{"expires_in_days":7}`))
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
