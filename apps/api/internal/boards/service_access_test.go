package boards

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"api/internal/testdb"
	"api/internal/users"
)

func TestBoardServiceTenantIsolationAndMemberAccess(t *testing.T) {
	db := testdb.Open(t)
	userRepo := users.NewUserRepository(db)
	repo := NewBoardRepository(db)
	service := NewBoardService(repo, userRepo)
	ctx := context.Background()

	ownerA := createBoardTestUser(t, userRepo, "owner-a")
	ownerB := createBoardTestUser(t, userRepo, "owner-b")
	memberC := createBoardTestUser(t, userRepo, "member-c")

	boardID := "shared-board"
	boardA, err := service.UpsertBoard(ctx, ownerA.ID, boardID, &UpsertBoardRequest{Name: "Alpha Board", State: map[string]interface{}{"v": 1}})
	if err != nil {
		t.Fatalf("create board A failed: %v", err)
	}
	boardB, err := service.UpsertBoard(ctx, ownerB.ID, boardID, &UpsertBoardRequest{Name: "Beta Board", State: map[string]interface{}{"v": 2}})
	if err != nil {
		t.Fatalf("create board B failed: %v", err)
	}
	if boardA.Board.ID == boardB.Board.ID {
		t.Fatal("boards should be distinct rows")
	}
	if boardA.Board.UserID != ownerA.ID || boardB.Board.UserID != ownerB.ID {
		t.Fatalf("unexpected board owners: %#v %#v", boardA.Board.UserID, boardB.Board.UserID)
	}

	accessA, roleA, err := service.ResolveBoardAccess(ctx, ownerA.ID, boardID)
	if err != nil {
		t.Fatalf("resolve owner A access failed: %v", err)
	}
	if accessA.UserID != ownerA.ID || roleA != BoardAccessRoleOwner {
		t.Fatalf("owner A access = (%d, %s), want (%d, owner)", accessA.UserID, roleA, ownerA.ID)
	}

	accessB, roleB, err := service.ResolveBoardAccess(ctx, ownerB.ID, boardID)
	if err != nil {
		t.Fatalf("resolve owner B access failed: %v", err)
	}
	if accessB.UserID != ownerB.ID || roleB != BoardAccessRoleOwner {
		t.Fatalf("owner B access = (%d, %s), want (%d, owner)", accessB.UserID, roleB, ownerB.ID)
	}

	if _, _, err := service.ResolveBoardAccess(ctx, memberC.ID, boardID); !errors.Is(err, ErrBoardNotFound) {
		t.Fatalf("uninvited access err = %v, want ErrBoardNotFound", err)
	}

	if _, err := service.InviteMember(ctx, ownerA.ID, boardID, memberC.StorageUID, string(BoardAccessRoleViewer)); err != nil {
		t.Fatalf("invite member failed: %v", err)
	}

	memberAccess, memberRole, err := service.ResolveBoardAccess(ctx, memberC.ID, boardID)
	if err != nil {
		t.Fatalf("resolve member access failed: %v", err)
	}
	if memberAccess.UserID != ownerA.ID {
		t.Fatalf("member access owner id = %d, want %d", memberAccess.UserID, ownerA.ID)
	}
	if memberRole != BoardAccessRoleViewer {
		t.Fatalf("member role = %s, want viewer", memberRole)
	}

	members, err := service.ListMembers(ctx, ownerA.ID, boardID)
	if err != nil {
		t.Fatalf("list members failed: %v", err)
	}
	if len(members) != 2 {
		t.Fatalf("members len = %d, want 2", len(members))
	}
}

func TestBoardServiceEditorCanUpdateStateButNotRename(t *testing.T) {
	db := testdb.Open(t)
	userRepo := users.NewUserRepository(db)
	repo := NewBoardRepository(db)
	service := NewBoardService(repo, userRepo)
	ctx := context.Background()

	owner := createBoardTestUser(t, userRepo, "editor-owner")
	editor := createBoardTestUser(t, userRepo, "editor-member")

	boardID := "editable-board"
	if _, err := service.UpsertBoard(ctx, owner.ID, boardID, &UpsertBoardRequest{Name: "Design Board", State: map[string]interface{}{"v": 1}}); err != nil {
		t.Fatalf("create board failed: %v", err)
	}
	if _, err := service.InviteMember(ctx, owner.ID, boardID, editor.StorageUID, string(BoardAccessRoleEditor)); err != nil {
		t.Fatalf("invite editor failed: %v", err)
	}

	updated, err := service.UpsertBoard(ctx, editor.ID, boardID, &UpsertBoardRequest{Name: "Design Board", State: map[string]interface{}{"v": 2}})
	if err != nil {
		t.Fatalf("editor update failed: %v", err)
	}
	if updated.Role != BoardAccessRoleEditor {
		t.Fatalf("updated role = %s, want editor", updated.Role)
	}
	if updated.Board.UserID != owner.ID {
		t.Fatalf("updated owner id = %d, want %d", updated.Board.UserID, owner.ID)
	}
	var state map[string]int
	if err := json.Unmarshal([]byte(updated.Board.State), &state); err != nil {
		t.Fatalf("decode updated state failed: %v", err)
	}
	if state["v"] != 2 {
		t.Fatalf("updated state v = %d, want 2", state["v"])
	}

	if _, err := service.UpsertBoard(ctx, editor.ID, boardID, &UpsertBoardRequest{Name: "Renamed Board", State: map[string]interface{}{"v": 3}}); !errors.Is(err, ErrBoardPermissionDenied) {
		t.Fatalf("editor rename err = %v, want ErrBoardPermissionDenied", err)
	}
}

func createBoardTestUser(t *testing.T, repo *users.UserRepository, username string) *users.User {
	t.Helper()
	user := &users.User{
		Username: username,
		Status:   1,
		Role:     "user",
	}
	if err := repo.Create(context.Background(), user); err != nil {
		t.Fatalf("create test user %s failed: %v", username, err)
	}
	return user
}
