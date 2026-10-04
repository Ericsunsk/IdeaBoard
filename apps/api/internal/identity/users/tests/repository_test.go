package tests

import (
	"context"
	"strings"
	"testing"

	"ideaboard/api/internal/db/dbutil"

	"github.com/google/uuid"
)

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

	if _, err := db.Exec(context.Background(), `INSERT INTO users_used_uids (storage_uid) VALUES ($1)`, "ABC123"); err != nil {
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
	if err := db.QueryRow(context.Background(), `SELECT COUNT(*) FROM users_used_uids WHERE storage_uid = $1`, user.StorageUID).Scan(&count); err != nil {
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

func TestAmbientTransactionCarriesAdminUserMutation(t *testing.T) {
	ctx := context.Background()
	db := newUserRepoTestDB(t)
	repo := NewUserRepository(db)
	observer, err := db.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire observer connection: %v", err)
	}
	defer observer.Release()

	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("begin transaction: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	ambient := dbutil.ContextWithAmbientTx(ctx, tx)
	if err := repo.UpdateRoleByID(ambient, 42, "admin"); err != nil {
		t.Fatalf("update role in ambient transaction: %v", err)
	}

	var role string
	if err := observer.QueryRow(ctx, `SELECT role FROM users WHERE id = $1`, 42).Scan(&role); err != nil {
		t.Fatalf("read role before commit: %v", err)
	}
	if role == "admin" {
		t.Fatal("ambient role update became visible before commit")
	}

	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit ambient transaction: %v", err)
	}
	if err := observer.QueryRow(ctx, `SELECT role FROM users WHERE id = $1`, 42).Scan(&role); err != nil {
		t.Fatalf("read role after commit: %v", err)
	}
	if role != "admin" {
		t.Fatalf("role after commit = %q, want admin", role)
	}
}

func TestAmbientTransactionRollbackDiscardsAdminUserMutation(t *testing.T) {
	ctx := context.Background()
	db := newUserRepoTestDB(t)
	repo := NewUserRepository(db)
	observer, err := db.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire observer connection: %v", err)
	}
	defer observer.Release()

	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("begin transaction: %v", err)
	}
	ambient := dbutil.ContextWithAmbientTx(ctx, tx)
	if err := repo.UpdateStatus(ambient, 42, StatusBanned); err != nil {
		t.Fatalf("update status in ambient transaction: %v", err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("rollback ambient transaction: %v", err)
	}

	var status int16
	if err := observer.QueryRow(ctx, `SELECT status FROM users WHERE id = $1`, 42).Scan(&status); err != nil {
		t.Fatalf("read status after rollback: %v", err)
	}
	if status == StatusBanned {
		t.Fatal("ambient status update survived rollback")
	}
}
