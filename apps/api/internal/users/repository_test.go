package users

import (
	"fmt"
	"strings"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newUserRepoTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", sanitizeSQLiteMemoryName(t.Name()))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite db failed: %v", err)
	}

	if err := db.AutoMigrate(&User{}, &UsedUID{}); err != nil {
		t.Fatalf("auto migrate users tables failed: %v", err)
	}

	for _, sql := range []string{
		`CREATE TABLE IF NOT EXISTS user_boards (id INTEGER PRIMARY KEY AUTOINCREMENT, board_id TEXT, user_id INTEGER)`,
		`CREATE TABLE IF NOT EXISTS board_members (id INTEGER PRIMARY KEY AUTOINCREMENT, board_id TEXT, user_id INTEGER)`,
		`CREATE TABLE IF NOT EXISTS board_share_tokens (id INTEGER PRIMARY KEY AUTOINCREMENT, board_id TEXT, created_by INTEGER)`,
		`CREATE TABLE IF NOT EXISTS tasks (id INTEGER PRIMARY KEY AUTOINCREMENT, user_id INTEGER)`,
	} {
		if err := db.Exec(sql).Error; err != nil {
			t.Fatalf("create cleanup table failed: %v", err)
		}
	}

	return db
}

func sanitizeSQLiteMemoryName(name string) string {
	replacer := strings.NewReplacer("/", "_", " ", "_", ":", "_")
	return replacer.Replace(strings.TrimSpace(name))
}

func TestCreateStorageUIDDoesNotReuseUsedUID(t *testing.T) {
	db := newUserRepoTestDB(t)
	repo := NewUserRepository(db)

	if err := db.Create(&UsedUID{StorageUID: "ABC123"}).Error; err != nil {
		t.Fatalf("seed used uid failed: %v", err)
	}

	user := &User{
		Username:   "u1",
		Status:     1,
		StorageUID: "ABC123",
	}
	if err := repo.Create(user); err != nil {
		t.Fatalf("create user failed: %v", err)
	}

	if user.StorageUID == "ABC123" {
		t.Fatalf("storage uid reused: %s", user.StorageUID)
	}
	if !IsValidStorageUID(user.StorageUID) {
		t.Fatalf("storage uid is invalid: %s", user.StorageUID)
	}

	var count int64
	if err := db.Model(&UsedUID{}).Where("storage_uid = ?", user.StorageUID).Count(&count).Error; err != nil {
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
	if err := repo.Create(first); err != nil {
		t.Fatalf("create first user failed: %v", err)
	}

	ownedBoardID := "board-owned"
	if err := db.Exec(`INSERT INTO user_boards (board_id, user_id) VALUES (?, ?)`, ownedBoardID, first.ID).Error; err != nil {
		t.Fatalf("seed user_boards failed: %v", err)
	}
	if err := db.Exec(`INSERT INTO board_members (board_id, user_id) VALUES (?, ?)`, ownedBoardID, first.ID).Error; err != nil {
		t.Fatalf("seed board_members owned failed: %v", err)
	}
	if err := db.Exec(`INSERT INTO board_members (board_id, user_id) VALUES (?, ?)`, "board-shared", first.ID).Error; err != nil {
		t.Fatalf("seed board_members shared failed: %v", err)
	}
	if err := db.Exec(`INSERT INTO board_share_tokens (board_id, created_by) VALUES (?, ?)`, ownedBoardID, first.ID).Error; err != nil {
		t.Fatalf("seed board_share_tokens owned failed: %v", err)
	}
	if err := db.Exec(`INSERT INTO board_share_tokens (board_id, created_by) VALUES (?, ?)`, "board-shared", first.ID).Error; err != nil {
		t.Fatalf("seed board_share_tokens shared failed: %v", err)
	}
	if err := db.Exec(`INSERT INTO tasks (user_id) VALUES (?)`, first.ID).Error; err != nil {
		t.Fatalf("seed tasks failed: %v", err)
	}

	if err := repo.Delete(first.ID); err != nil {
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
		if err := db.Raw(query, first.ID).Scan(&count).Error; err != nil {
			t.Fatalf("count %s failed: %v", name, err)
		}
		if count != 0 {
			t.Fatalf("%s count = %d, want 0", name, count)
		}
	}

	var oldUsedUIDCount int64
	if err := db.Model(&UsedUID{}).Where("storage_uid = ?", originalUID).Count(&oldUsedUIDCount).Error; err != nil {
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
	if err := repo.Create(second); err != nil {
		t.Fatalf("create second user failed: %v", err)
	}

	if second.StorageUID == originalUID {
		t.Fatalf("uid reused after delete: %s", second.StorageUID)
	}
	if !IsValidStorageUID(second.StorageUID) {
		t.Fatalf("new storage uid is invalid: %s", second.StorageUID)
	}
}
