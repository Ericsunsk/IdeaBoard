package boards

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	identitydomain "ideaboard/api/internal/identity/users/domain"
	identitypostgres "ideaboard/api/internal/identity/users/infrastructure/postgres"
	"ideaboard/api/internal/testdb"
)

func TestBoardServiceTenantIsolationAndMemberAccess(t *testing.T) {
	db := testdb.Open(t)
	userRepo := identitypostgres.NewRepository(db)
	repo := NewRepository(db)
	service := NewService(repo, userRepo)
	ctx := context.Background()

	ownerA := createBoardTestUser(t, userRepo, "owner-a")
	ownerB := createBoardTestUser(t, userRepo, "owner-b")
	memberC := createBoardTestUser(t, userRepo, "member-c")

	boardID := "shared-board"
	boardA, err := service.UpsertBoard(ctx, ownerA.ID, boardID, &UpsertRequest{Name: "Alpha Board", State: validBoardState(CurrentBoardStateSchemaVersion, 101)})
	if err != nil {
		t.Fatalf("create board A failed: %v", err)
	}
	if _, err := service.UpsertBoard(ctx, ownerB.ID, boardID, &UpsertRequest{Name: "Beta Board", State: validBoardState(CurrentBoardStateSchemaVersion, 102)}); !errors.Is(err, ErrIDConflict) {
		t.Fatalf("duplicate board id err = %v, want ErrIDConflict", err)
	}

	boardBID := "owner-b-board"
	boardB, err := service.UpsertBoard(ctx, ownerB.ID, boardBID, &UpsertRequest{Name: "Beta Board", State: validBoardState(CurrentBoardStateSchemaVersion, 102)})
	if err != nil {
		t.Fatalf("create board B failed: %v", err)
	}
	if boardA.Board.ID == boardB.Board.ID {
		t.Fatal("boards should be distinct rows")
	}
	if boardA.Board.OwnerUserID != ownerA.ID || boardB.Board.OwnerUserID != ownerB.ID {
		t.Fatalf("unexpected board owners: %#v %#v", boardA.Board.OwnerUserID, boardB.Board.OwnerUserID)
	}

	accessA, roleA, err := service.ResolveBoardAccess(ctx, ownerA.ID, boardID)
	if err != nil {
		t.Fatalf("resolve owner A access failed: %v", err)
	}
	if accessA.OwnerUserID != ownerA.ID || roleA != RoleOwner {
		t.Fatalf("owner A access = (%d, %s), want (%d, owner)", accessA.OwnerUserID, roleA, ownerA.ID)
	}

	accessB, roleB, err := service.ResolveBoardAccess(ctx, ownerB.ID, boardBID)
	if err != nil {
		t.Fatalf("resolve owner B access failed: %v", err)
	}
	if accessB.OwnerUserID != ownerB.ID || roleB != RoleOwner {
		t.Fatalf("owner B access = (%d, %s), want (%d, owner)", accessB.OwnerUserID, roleB, ownerB.ID)
	}

	if _, _, err := service.ResolveBoardAccess(ctx, memberC.ID, boardID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("uninvited access err = %v, want ErrNotFound", err)
	}

	invited, err := service.InviteMember(ctx, ownerA.ID, boardID, memberC.PublicID.String(), string(RoleViewer))
	if err != nil {
		t.Fatalf("invite member failed: %v", err)
	}
	if invited.PublicID != memberC.PublicID || invited.DisplayName != memberC.DisplayName || invited.Role != RoleViewer {
		t.Fatalf("invited member = %+v, want public identity for %s", invited, memberC.PublicID)
	}

	memberAccess, memberRole, err := service.ResolveBoardAccess(ctx, memberC.ID, boardID)
	if err != nil {
		t.Fatalf("resolve member access failed: %v", err)
	}
	if memberAccess.OwnerUserID != ownerA.ID {
		t.Fatalf("member access owner id = %d, want %d", memberAccess.OwnerUserID, ownerA.ID)
	}
	if memberRole != RoleViewer {
		t.Fatalf("member role = %s, want viewer", memberRole)
	}

	members, err := service.ListMembers(ctx, ownerA.ID, boardID)
	if err != nil {
		t.Fatalf("list members failed: %v", err)
	}
	if len(members) != 2 {
		t.Fatalf("members len = %d, want 2", len(members))
	}
	encodedMembers, err := json.Marshal(members)
	if err != nil {
		t.Fatalf("marshal members failed: %v", err)
	}
	for _, privateField := range []string{"user_id", "member_uid", "storage_uid"} {
		if strings.Contains(string(encodedMembers), `"`+privateField+`"`) {
			t.Fatalf("members expose private field %q: %s", privateField, encodedMembers)
		}
	}
}

func TestBoardServiceEditorCanUpdateStateButNotRename(t *testing.T) {
	db := testdb.Open(t)
	userRepo := identitypostgres.NewRepository(db)
	repo := NewRepository(db)
	service := NewService(repo, userRepo)
	ctx := context.Background()

	owner := createBoardTestUser(t, userRepo, "editor-owner")
	editor := createBoardTestUser(t, userRepo, "editor-member")

	boardID := "editable-board"
	if _, err := service.UpsertBoard(ctx, owner.ID, boardID, &UpsertRequest{Name: "Design Board", State: validBoardState(CurrentBoardStateSchemaVersion, 100)}); err != nil {
		t.Fatalf("create board failed: %v", err)
	}
	if _, err := service.InviteMember(ctx, owner.ID, boardID, editor.PublicID.String(), string(RoleEditor)); err != nil {
		t.Fatalf("invite editor failed: %v", err)
	}

	updated, err := service.UpsertBoard(ctx, editor.ID, boardID, &UpsertRequest{
		Name:         "Design Board",
		State:        validBoardState(CurrentBoardStateSchemaVersion, 250),
		BaseRevision: 1,
	})
	if err != nil {
		t.Fatalf("editor update failed: %v", err)
	}
	if updated.Role != RoleEditor {
		t.Fatalf("updated role = %s, want editor", updated.Role)
	}
	if updated.Board.OwnerUserID != owner.ID {
		t.Fatalf("updated owner id = %d, want %d", updated.Board.OwnerUserID, owner.ID)
	}
	if _, err := service.UpsertBoard(ctx, editor.ID, boardID, &UpsertRequest{
		Name: "Design Board", State: validBoardState(CurrentBoardStateSchemaVersion, 251), BaseRevision: 1,
	}); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("stale revision err = %v, want ErrRevisionConflict", err)
	}
	var state struct {
		SchemaVersion int `json:"schemaVersion"`
		PromptNodes   []struct {
			Marker int `json:"marker"`
		} `json:"promptNodes"`
	}
	if err := json.Unmarshal([]byte(updated.Board.State), &state); err != nil {
		t.Fatalf("decode updated state failed: %v", err)
	}
	if state.SchemaVersion != CurrentBoardStateSchemaVersion {
		t.Fatalf(
			"updated schemaVersion = %d, want %d",
			state.SchemaVersion,
			CurrentBoardStateSchemaVersion,
		)
	}
	if len(state.PromptNodes) != 1 || state.PromptNodes[0].Marker != 250 {
		t.Fatalf("updated prompt marker = %+v, want 250", state.PromptNodes)
	}

	if _, err := service.UpsertBoard(ctx, editor.ID, boardID, &UpsertRequest{Name: "Renamed Board", State: validBoardState(CurrentBoardStateSchemaVersion, 300)}); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("editor rename err = %v, want ErrPermissionDenied", err)
	}
}

func TestBoardServiceShareTokenUsesBoardIdentityForPermissions(t *testing.T) {
	db := testdb.Open(t)
	userRepo := identitypostgres.NewRepository(db)
	repo := NewRepository(db)
	service := NewService(repo, userRepo)
	ctx := context.Background()

	owner := createBoardTestUser(t, userRepo, "share-owner")
	joiner := createBoardTestUser(t, userRepo, "share-joiner")
	outsider := createBoardTestUser(t, userRepo, "share-outsider")

	const boardID = "share-permission-board"
	created, err := service.UpsertBoard(ctx, owner.ID, boardID, &UpsertRequest{Name: "Share Permission Board", State: validBoardState(CurrentBoardStateSchemaVersion, 100)})
	if err != nil {
		t.Fatalf("create board failed: %v", err)
	}
	if created.Board.OwnerUserID != owner.ID || created.Board.CreatedBy != owner.ID || created.Board.LastSavedBy == nil || *created.Board.LastSavedBy != owner.ID {
		t.Fatalf("board identity = owner:%d created:%d saved:%v, want owner/created/saved %d", created.Board.OwnerUserID, created.Board.CreatedBy, created.Board.LastSavedBy, owner.ID)
	}

	token, err := service.CreateShareToken(ctx, owner.ID, boardID, string(RoleEditor), nil)
	if err != nil {
		t.Fatalf("create share token failed: %v", err)
	}
	if token.BoardID != boardID || token.CreatedBy != owner.ID {
		t.Fatalf("token identity = board:%q created:%d, want board:%q created:%d", token.BoardID, token.CreatedBy, boardID, owner.ID)
	}
	ownerTokens, err := service.ListShareTokens(ctx, owner.ID, boardID)
	if err != nil {
		t.Fatalf("owner list share tokens failed: %v", err)
	}
	if len(ownerTokens) != 1 || ownerTokens[0].ID != token.ID {
		t.Fatalf("owner share tokens = %+v, want token %d", ownerTokens, token.ID)
	}
	if _, err := service.ListShareTokens(ctx, outsider.ID, boardID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("outsider list share tokens err = %v, want ErrNotFound", err)
	}
	if err := service.RevokeShareToken(ctx, outsider.ID, boardID, token.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("outsider revoke share token err = %v, want ErrNotFound", err)
	}

	joined, err := service.JoinBoardByShareToken(ctx, joiner.ID, token.Token)
	if err != nil {
		t.Fatalf("join board failed: %v", err)
	}
	if joined.Board.OwnerUserID != owner.ID || joined.Role != RoleEditor {
		t.Fatalf("joined access = owner:%d role:%s, want owner:%d role:editor", joined.Board.OwnerUserID, joined.Role, owner.ID)
	}
	if _, err := service.ListShareTokens(ctx, joiner.ID, boardID); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("member list share tokens err = %v, want ErrPermissionDenied", err)
	}
	if err := service.RevokeShareToken(ctx, joiner.ID, boardID, token.ID); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("member revoke share token err = %v, want ErrPermissionDenied", err)
	}
	if err := service.RevokeShareToken(ctx, owner.ID, boardID, token.ID); err != nil {
		t.Fatalf("owner revoke share token failed: %v", err)
	}
}

func createBoardTestUser(t *testing.T, repo *identitypostgres.Repository, displayName string) *identitydomain.User {
	t.Helper()
	user := &identitydomain.User{
		DisplayName: displayName + " display",
		Status:      1,
		Role:        "user",
	}
	if err := repo.Create(context.Background(), user); err != nil {
		t.Fatalf("create test user %s failed: %v", displayName, err)
	}
	return user
}
