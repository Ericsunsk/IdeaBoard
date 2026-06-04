package auth

import (
	"context"
	"errors"
	"testing"

	"ideaboard/api/internal/platform/database"
	"ideaboard/api/internal/testdb"
	"ideaboard/api/internal/users"
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
