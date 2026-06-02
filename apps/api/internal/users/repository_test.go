package users

import (
	"context"
	"database/sql"
	"testing"

	"api/internal/testdb"
)

func newUserRepoTestDB(t *testing.T) *sql.DB {
	t.Helper()
	return testdb.Open(t)
}

func TestCreateStorageUIDDoesNotReuseUsedUID(t *testing.T) {
	db := newUserRepoTestDB(t)
	repo := NewUserRepository(db)

	if _, err := db.ExecContext(context.Background(), `INSERT INTO users_used_uids (storage_uid) VALUES (?)`, "ABC123"); err != nil {
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
	if err := db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM users_used_uids WHERE storage_uid = ?`, user.StorageUID).Scan(&count); err != nil {
		t.Fatalf("count used uid failed: %v", err)
	}
	if count != 1 {
		t.Fatalf("used uid record count = %d, want 1", count)
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
	if _, err := db.ExecContext(context.Background(), `INSERT INTO user_boards (board_id, user_id, name, state) VALUES (?, ?, ?, ?)`, ownedBoardID, first.ID, "Owned", "{}"); err != nil {
		t.Fatalf("seed user_boards failed: %v", err)
	}
	if _, err := db.ExecContext(context.Background(), `INSERT INTO board_members (board_id, user_id, role) VALUES (?, ?, ?)`, ownedBoardID, first.ID, "viewer"); err != nil {
		t.Fatalf("seed board_members owned failed: %v", err)
	}
	if _, err := db.ExecContext(context.Background(), `INSERT INTO board_members (board_id, user_id, role) VALUES (?, ?, ?)`, "board-shared", first.ID, "viewer"); err != nil {
		t.Fatalf("seed board_members shared failed: %v", err)
	}
	if _, err := db.ExecContext(context.Background(), `INSERT INTO board_share_tokens (board_id, token, role, created_by) VALUES (?, ?, ?, ?)`, ownedBoardID, "owned-token", "viewer", first.ID); err != nil {
		t.Fatalf("seed board_share_tokens owned failed: %v", err)
	}
	if _, err := db.ExecContext(context.Background(), `INSERT INTO board_share_tokens (board_id, token, role, created_by) VALUES (?, ?, ?, ?)`, "board-shared", "shared-token", "viewer", first.ID); err != nil {
		t.Fatalf("seed board_share_tokens shared failed: %v", err)
	}
	if _, err := db.ExecContext(
		context.Background(),
		`INSERT INTO tasks (task_id, user_id, channel_id, prompt, user_prompt, params, status, error_msg, result) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"delete-user-task", first.ID, 1, "", "", "", "pending", "", "",
	); err != nil {
		t.Fatalf("seed tasks failed: %v", err)
	}

	if err := repo.Delete(context.Background(), first.ID); err != nil {
		t.Fatalf("delete user failed: %v", err)
	}

	for name, query := range map[string]string{
		"user":              `SELECT COUNT(*) FROM users WHERE id = ?`,
		"owned board":       `SELECT COUNT(*) FROM user_boards WHERE user_id = ?`,
		"board member":      `SELECT COUNT(*) FROM board_members WHERE user_id = ?`,
		"board share token": `SELECT COUNT(*) FROM board_share_tokens WHERE created_by = ?`,
		"task":              `SELECT COUNT(*) FROM tasks WHERE user_id = ?`,
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
	if err := db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM users_used_uids WHERE storage_uid = ?`, originalUID).Scan(&oldUsedUIDCount); err != nil {
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
