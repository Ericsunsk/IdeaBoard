package auth

import (
	"context"
	"testing"

	"api/internal/users"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestPtrStringAndDerefString(t *testing.T) {
	if got := ptrString("  "); got != nil {
		t.Fatal("expected blank string to map to nil pointer")
	}
	v := ptrString("  alice  ")
	if v == nil || *v != "alice" {
		t.Fatalf("trimmed pointer = %#v, want alice", v)
	}
	if got := derefString(nil); got != "" {
		t.Fatalf("deref nil = %q, want empty string", got)
	}
	if got := derefString(v); got != "alice" {
		t.Fatalf("deref value = %q, want alice", got)
	}
}

func TestResolveLocalUserByAuthIdentityDoesNotBindExistingUsername(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite db failed: %v", err)
	}
	if err := db.AutoMigrate(&users.User{}, &users.UsedUID{}); err != nil {
		t.Fatalf("auto migrate users failed: %v", err)
	}

	userRepo := users.NewUserRepository(db)
	existing := &users.User{Username: "alice", Status: 1, Role: "user"}
	if err := userRepo.Create(context.Background(), existing); err != nil {
		t.Fatalf("seed existing user failed: %v", err)
	}

	resolved, err := resolveLocalUserByAuthIdentity(context.Background(), userRepo, &SessionIdentity{
		AuthUserID: "auth-1234567890",
		Username:   "alice",
	})
	if err != nil {
		t.Fatalf("resolve local user failed: %v", err)
	}
	if resolved.ID == existing.ID {
		t.Fatalf("resolved existing username user; want a new auth-bound user")
	}
	if resolved.Username != "alice_01" {
		t.Fatalf("resolved username = %q, want alice_01", resolved.Username)
	}
	if resolved.AuthUserID == nil || *resolved.AuthUserID != "auth-1234567890" {
		t.Fatalf("resolved auth_user_id = %#v, want auth-1234567890", resolved.AuthUserID)
	}

	unchanged, err := userRepo.GetByID(context.Background(), existing.ID)
	if err != nil {
		t.Fatalf("reload existing user failed: %v", err)
	}
	if unchanged.AuthUserID != nil {
		t.Fatalf("existing username user auth_user_id = %#v, want nil", unchanged.AuthUserID)
	}
}
