package boards

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"api/internal/platform/database"
	"api/internal/testdb"
	"api/internal/users"
)

func newBoardRepoTestDB(t *testing.T) *sql.DB {
	t.Helper()
	return testdb.Open(t)
}

func TestBoardRepositoryOwnerAndAccessibleLookup(t *testing.T) {
	db := newBoardRepoTestDB(t)
	repo := NewBoardRepository(db)
	userRepo := users.NewUserRepository(db)

	owner := &users.User{Username: "owner", Status: 1, Role: "user"}
	if err := userRepo.Create(context.Background(), owner); err != nil {
		t.Fatalf("seed owner failed: %v", err)
	}
	member := &users.User{Username: "member", Status: 1, Role: "user"}
	if err := userRepo.Create(context.Background(), member); err != nil {
		t.Fatalf("seed member failed: %v", err)
	}

	t.Run("missing owner board returns record not found", func(t *testing.T) {
		board, err := repo.GetByOwnerAndBoardID(context.Background(), owner.ID, "missing-board")
		if board != nil {
			t.Fatalf("board = %#v, want nil", board)
		}
		if !database.IsNotFound(err) {
			t.Fatalf("err = %v, want not found", err)
		}
	})

	t.Run("existing board returns owner board", func(t *testing.T) {
		const boardID = "board-1"
		seed := &UserBoard{
			BoardID: boardID,
			UserID:  owner.ID,
			Name:    "Board 1",
			State:   "{}",
		}
		seed.CreatedAt = time.Now()
		seed.UpdatedAt = seed.CreatedAt
		if err := repo.Upsert(context.Background(), seed); err != nil {
			t.Fatalf("seed board failed: %v", err)
		}

		board, err := repo.GetByOwnerAndBoardID(context.Background(), owner.ID, boardID)
		if err != nil {
			t.Fatalf("GetByOwnerAndBoardID returned error: %v", err)
		}
		if board == nil {
			t.Fatalf("board is nil")
		}
		if board.BoardID != boardID {
			t.Fatalf("board.BoardID = %q, want %q", board.BoardID, boardID)
		}

		if err := repo.UpsertMember(context.Background(), owner.ID, boardID, member.ID, BoardAccessRoleViewer); err != nil {
			t.Fatalf("seed board member failed: %v", err)
		}
		accessible, err := repo.GetAccessibleByBoardID(context.Background(), member.ID, boardID)
		if err != nil {
			t.Fatalf("GetAccessibleByBoardID returned error: %v", err)
		}
		if accessible == nil {
			t.Fatal("accessible board is nil")
		}
		if accessible.Board.UserID != owner.ID {
			t.Fatalf("accessible owner id = %d, want %d", accessible.Board.UserID, owner.ID)
		}
		if accessible.Role != BoardAccessRoleViewer {
			t.Fatalf("accessible role = %s, want viewer", accessible.Role)
		}
	})
}
