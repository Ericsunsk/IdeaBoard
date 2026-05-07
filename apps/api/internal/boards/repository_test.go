package boards

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newBoardRepoTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", sanitizeBoardSQLiteMemoryName(t.Name()))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite db failed: %v", err)
	}

	if err := db.AutoMigrate(&UserBoard{}); err != nil {
		t.Fatalf("auto migrate user_boards failed: %v", err)
	}

	return db
}

func sanitizeBoardSQLiteMemoryName(name string) string {
	replacer := strings.NewReplacer("/", "_", " ", "_", ":", "_")
	return replacer.Replace(strings.TrimSpace(name))
}

func TestBoardRepositoryGetByBoardID(t *testing.T) {
	db := newBoardRepoTestDB(t)
	repo := NewBoardRepository(db)

	t.Run("missing board returns record not found", func(t *testing.T) {
		board, err := repo.GetByBoardID("missing-board")
		if board != nil {
			t.Fatalf("board = %#v, want nil", board)
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			t.Fatalf("err = %v, want gorm.ErrRecordNotFound", err)
		}
	})

	t.Run("existing board returns board", func(t *testing.T) {
		const boardID = "board-1"
		seed := &UserBoard{
			BoardID: boardID,
			UserID:  1,
			Name:    "Board 1",
			State:   "{}",
		}
		if err := db.Create(seed).Error; err != nil {
			t.Fatalf("seed board failed: %v", err)
		}

		board, err := repo.GetByBoardID(boardID)
		if err != nil {
			t.Fatalf("GetByBoardID returned error: %v", err)
		}
		if board == nil {
			t.Fatalf("board is nil")
		}
		if board.BoardID != boardID {
			t.Fatalf("board.BoardID = %q, want %q", board.BoardID, boardID)
		}
	})
}
