package boards

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"api/internal/platform/database"
	"api/internal/testdb"
)

func newBoardRepoTestDB(t *testing.T) *sql.DB {
	t.Helper()
	return testdb.Open(t)
}

func TestBoardRepositoryGetByBoardID(t *testing.T) {
	db := newBoardRepoTestDB(t)
	repo := NewBoardRepository(db)

	t.Run("missing board returns record not found", func(t *testing.T) {
		board, err := repo.GetByBoardID(context.Background(), "missing-board")
		if board != nil {
			t.Fatalf("board = %#v, want nil", board)
		}
		if !database.IsNotFound(err) {
			t.Fatalf("err = %v, want not found", err)
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
		seed.CreatedAt = time.Now()
		seed.UpdatedAt = seed.CreatedAt
		if err := repo.Upsert(context.Background(), seed); err != nil {
			t.Fatalf("seed board failed: %v", err)
		}

		board, err := repo.GetByBoardID(context.Background(), boardID)
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
