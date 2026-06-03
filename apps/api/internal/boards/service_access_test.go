package boards

import (
	"context"
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
