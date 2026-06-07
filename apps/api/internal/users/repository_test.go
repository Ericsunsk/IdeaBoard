package users

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"ideaboard/api/internal/testdb"
)

func newUserRepoTestDB(t *testing.T) *sql.DB {
	t.Helper()
	return testdb.Open(t)
}

func TestCreateStorageUIDDoesNotReuseUsedUID(t *testing.T) {
	db := newUserRepoTestDB(t)
	repo := NewUserRepository(db)

	if _, err := db.ExecContext(context.Background(), `INSERT INTO users_used_uids (storage_uid) VALUES ($1)`, "ABC123"); err != nil {
		t.Fatalf("seed used uid failed: %v", err)
	}

	user := &User{
		Username:   "u1",
		Status:     1,
		StorageUID: "ABC123",
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
		{Username: "filter_full_banned_user_a", Role: "user", Status: 0},
		{Username: "filter_full_banned_user_b", Role: "user", Status: 0},
		{Username: "filter_full_active_user", Role: "user", Status: 1},
		{Username: "filter_full_banned_admin", Role: "admin", Status: 0},
		{Username: "outside_banned_user", Role: "user", Status: 0},
	}
	for _, user := range seedUsers {
		if err := repo.Create(ctx, user); err != nil {
			t.Fatalf("create user %q failed: %v", user.Username, err)
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
	if list[0].Role != "user" || list[0].Status != 0 || !strings.Contains(list[0].Username, "filter_full") {
		t.Fatalf("unexpected filtered user: %+v", list[0])
	}
}

func TestDeleteUserThenRecreateDoesNotReuseUID(t *testing.T) {
	db := newUserRepoTestDB(t)
	repo := NewUserRepository(db)

	originalUID := "ZZ9Y88"
	first := &User{
		Username:   "u_delete_1",
		Status:     1,
		StorageUID: originalUID,
	}
	if err := repo.Create(context.Background(), first); err != nil {
		t.Fatalf("create first user failed: %v", err)
	}

	ownedBoardID := "board-owned"
	if _, err := db.ExecContext(context.Background(), `INSERT INTO user_boards (board_id, user_id, name, state) VALUES ($1, $2, $3, $4)`, ownedBoardID, first.ID, "Owned", "{}"); err != nil {
		t.Fatalf("seed user_boards failed: %v", err)
	}
	if _, err := db.ExecContext(context.Background(), `INSERT INTO board_members (owner_user_id, board_id, user_id, role) VALUES ($1, $2, $3, $4)`, first.ID, ownedBoardID, first.ID, "viewer"); err != nil {
		t.Fatalf("seed board_members owned failed: %v", err)
	}
	if _, err := db.ExecContext(context.Background(), `INSERT INTO board_members (owner_user_id, board_id, user_id, role) VALUES ($1, $2, $3, $4)`, first.ID, "board-shared", first.ID, "viewer"); err != nil {
		t.Fatalf("seed board_members shared failed: %v", err)
	}
	if _, err := db.ExecContext(context.Background(), `INSERT INTO board_share_tokens (board_id, token, role, created_by) VALUES ($1, $2, $3, $4)`, ownedBoardID, "owned-token", "viewer", first.ID); err != nil {
		t.Fatalf("seed board_share_tokens owned failed: %v", err)
	}
	if _, err := db.ExecContext(context.Background(), `INSERT INTO board_share_tokens (board_id, token, role, created_by) VALUES ($1, $2, $3, $4)`, "board-shared", "shared-token", "viewer", first.ID); err != nil {
		t.Fatalf("seed board_share_tokens shared failed: %v", err)
	}
	if _, err := db.ExecContext(
		context.Background(),
		`INSERT INTO tasks (task_id, user_id, channel_id, prompt, user_prompt, params, status, error_msg, result) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		"delete-user-task", first.ID, 1, "", "", "", "pending", "", "",
	); err != nil {
		t.Fatalf("seed tasks failed: %v", err)
	}

	if err := repo.Delete(context.Background(), first.ID); err != nil {
		t.Fatalf("delete user failed: %v", err)
	}

	for name, query := range map[string]string{
		"user":              `SELECT COUNT(*) FROM users WHERE id = $1`,
		"owned board":       `SELECT COUNT(*) FROM user_boards WHERE user_id = $1`,
		"board member":      `SELECT COUNT(*) FROM board_members WHERE user_id = $1`,
		"board share token": `SELECT COUNT(*) FROM board_share_tokens WHERE created_by = $1`,
		"task":              `SELECT COUNT(*) FROM tasks WHERE user_id = $1`,
	} {
		var count int64
		if err := db.QueryRowContext(context.Background(), query, first.ID).Scan(&count); err != nil {
			t.Fatalf("count %s failed: %v", name, err)
		}
		if count != 0 {
			t.Fatalf("%s count = %d, want 0", name, count)
		}
	}

	var oldUsedUIDCount int64
	if err := db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM users_used_uids WHERE storage_uid = $1`, originalUID).Scan(&oldUsedUIDCount); err != nil {
		t.Fatalf("count old used uid failed: %v", err)
	}
	if oldUsedUIDCount != 1 {
		t.Fatalf("old used uid count = %d, want 1", oldUsedUIDCount)
	}

	second := &User{
		Username:   "u_delete_2",
		Status:     1,
		StorageUID: originalUID,
	}
	if err := repo.Create(context.Background(), second); err != nil {
		t.Fatalf("create second user failed: %v", err)
	}

	if second.StorageUID == originalUID {
		t.Fatalf("uid reused after delete: %s", second.StorageUID)
	}
	if !IsValidStorageUID(second.StorageUID) {
		t.Fatalf("new storage uid is invalid: %s", second.StorageUID)
	}
}
