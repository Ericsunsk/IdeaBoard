package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"ideaboard/api/internal/platform/config"
	"ideaboard/api/internal/platform/database"
	"ideaboard/api/internal/testdb"
	"ideaboard/api/internal/users"

	"github.com/gin-gonic/gin"
)

func TestResolveLocalUserByAuthIdentityRequiresExistingLocalUser(t *testing.T) {
	db := testdb.Open(t)
	userRepo := users.NewUserRepository(db)
	if err := userRepo.Create(context.Background(), &users.User{Username: "alice", Status: 1, Role: "user"}); err != nil {
		t.Fatalf("seed existing user failed: %v", err)
	}

	resolvedUser := &users.User{Username: "alice_auth", AuthUserID: func() *string { v := "auth-1234567890"; return &v }(), Status: 1, Role: "user"}
	if err := userRepo.Create(context.Background(), resolvedUser); err != nil {
		t.Fatalf("seed auth-bound user failed: %v", err)
	}

	resolved, err := resolveLocalUserByAuthIdentity(context.Background(), userRepo, &SessionIdentity{
		AuthUserID: "auth-1234567890",
		Username:   "alice",
	})
	if err != nil {
		t.Fatalf("resolve local user failed: %v", err)
	}
	if resolved.ID != resolvedUser.ID {
		t.Fatalf("resolved user id = %d, want %d", resolved.ID, resolvedUser.ID)
	}
	if resolved.AuthUserID == nil || *resolved.AuthUserID != "auth-1234567890" {
		t.Fatalf("resolved auth_user_id = %#v, want auth-1234567890", resolved.AuthUserID)
	}

	_, err = resolveLocalUserByAuthIdentity(context.Background(), userRepo, &SessionIdentity{
		AuthUserID: "auth-missing-123",
		Username:   "ghost",
	})
	if err == nil {
		t.Fatal("expected missing local user to be rejected")
	}
	if !errors.Is(err, database.ErrNotFound) {
		t.Fatalf("err = %v, want database not found", err)
	}

	if _, err := userRepo.GetByAuthUserID(context.Background(), "auth-missing-123"); !errors.Is(err, database.ErrNotFound) {
		t.Fatalf("missing auth user should not be auto-created, got err=%v", err)
	}
}

func TestAuthMiddlewareReturnsServiceUnavailableWhenLocalLookupFails(t *testing.T) {
	gin.SetMode(gin.TestMode)

	authServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"msg":"success","data":{"session":{"auth_user_id":"auth-db-error","username":"alice"}}}`))
	}))
	defer authServer.Close()

	db := testdb.Open(t)
	userRepo := users.NewUserRepository(db)
	if err := db.Close(); err != nil {
		t.Fatalf("close test db failed: %v", err)
	}

	router := gin.New()
	router.Use(AuthMiddleware(NewServiceClient(config.AuthServiceConfig{
		BaseURL:        authServer.URL,
		TimeoutSeconds: 1,
	}), userRepo))
	router.GET("/api/user/info", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/user/info", nil)
	req.Header.Set("Cookie", "better-auth.session_token=test")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusServiceUnavailable, rec.Body.String())
	}
}
