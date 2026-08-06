package users

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"ideaboard/api/internal/testdb"

	"github.com/google/uuid"
)

func newUserRepoTestDB(t *testing.T) *sql.DB {
	t.Helper()
	return testdb.Open(t)
}

func TestCreateAssignsPublicIdentityAndPersistsDisplayName(t *testing.T) {
	db := newUserRepoTestDB(t)
	repo := NewUserRepository(db)
	user := &User{
		DisplayName: "画布创作者",
		Status:      1,
	}

	if err := repo.Create(context.Background(), user); err != nil {
		t.Fatalf("create user failed: %v", err)
	}
	if user.PublicID == uuid.Nil {
		t.Fatal("public id is nil")
	}

	stored, err := repo.GetByID(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("get user failed: %v", err)
	}
	if stored.PublicID != user.PublicID || stored.DisplayName != "画布创作者" {
		t.Fatalf("stored public identity = %+v, want public_id=%s display_name=%q", stored, user.PublicID, "画布创作者")
	}
	publicUser, err := repo.GetByPublicID(context.Background(), user.PublicID.String())
	if err != nil {
		t.Fatalf("get user by public id failed: %v", err)
	}
	if publicUser.ID != user.ID {
		t.Fatalf("public identity resolved internal id = %d, want %d", publicUser.ID, user.ID)
	}
}

func TestCreateStorageUIDDoesNotReuseUsedUID(t *testing.T) {
	db := newUserRepoTestDB(t)
	repo := NewUserRepository(db)

	if _, err := db.ExecContext(context.Background(), `INSERT INTO users_used_uids (storage_uid) VALUES ($1)`, "ABC123"); err != nil {
		t.Fatalf("seed used uid failed: %v", err)
	}

	user := &User{
		DisplayName: "u1",
		Status:      1,
		StorageUID:  "ABC123",
	}
	if err := repo.Create(context.Background(), user); err != nil {
		t.Fatalf("create user failed: %v", err)
	}

	if user.StorageUID == "ABC123" {
		t.Fatalf("storage uid reused: %s", user.StorageUID)
	}
	if !IsValidStorageUID(user.StorageUID) {
		t.Fatalf("storage uid is invalid: %s", user.StorageUID)
	}

	var count int64
	if err := db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM users_used_uids WHERE storage_uid = $1`, user.StorageUID).Scan(&count); err != nil {
		t.Fatalf("count used uid failed: %v", err)
	}
	if count != 1 {
		t.Fatalf("used uid record count = %d, want 1", count)
	}
}

func TestListUsersFiltersRoleAndStatusAcrossTotal(t *testing.T) {
	ctx := context.Background()
	db := newUserRepoTestDB(t)
	repo := NewUserRepository(db)

	seedUsers := []*User{
		{DisplayName: "filter_full_banned_user_a", Role: "user", Status: 0},
		{DisplayName: "filter_full_banned_user_b", Role: "user", Status: 0},
		{DisplayName: "filter_full_active_user", Role: "user", Status: 1},
		{DisplayName: "filter_full_banned_admin", Role: "admin", Status: 0},
		{DisplayName: "outside_banned_user", Role: "user", Status: 0},
	}
	for _, user := range seedUsers {
		if err := repo.Create(ctx, user); err != nil {
			t.Fatalf("create user %q failed: %v", user.DisplayName, err)
		}
	}

	list, counts, err := repo.List(ctx, "filter_full", "user", 0, true, 1, 1)
	if err != nil {
		t.Fatalf("list users failed: %v", err)
	}
	if counts.Total != 2 {
		t.Fatalf("total = %d, want 2", counts.Total)
	}
	if counts.ActiveCount != 0 || counts.BannedCount != 2 || counts.AdminCount != 0 {
		t.Fatalf("counts = %+v, want active=0 banned=2 admin=0", counts)
	}
	if len(list) != 1 {
		t.Fatalf("page list length = %d, want 1", len(list))
	}
	if list[0].Role != "user" || list[0].Status != 0 || !strings.Contains(list[0].DisplayName, "filter_full") {
		t.Fatalf("unexpected filtered user: %+v", list[0])
	}
}
