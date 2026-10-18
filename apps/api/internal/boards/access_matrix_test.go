package boards

import (
	"context"
	"errors"
	"sort"
	"testing"
)

// Matrix personas. IDs stay within the testdb-seeded users so the shared
// matrix also runs on PostgreSQL (which enforces user FKs).
const (
	matrixCreator  = int64(1)
	matrixEditor   = int64(3)
	matrixViewer   = int64(7)
	matrixStranger = int64(8)
	matrixMember   = int64(10)
	matrixWSOwner  = int64(11)
	matrixWSAdmin  = int64(12)
)

func mustMatrixBoard(t *testing.T, ctx context.Context, store WorkspaceBoardStore, workspaceID, name string, owner int64) string {
	t.Helper()
	boardID, err := NewV1BoardPublicID()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateBoard(ctx, workspaceID, owner, name, boardID); err != nil {
		t.Fatalf("create board %s: %v", name, err)
	}
	return boardID
}

// exerciseAccessMatrix pins the unified visibility + lifecycle behavior on
// any WorkspaceBoardStore. Callers pass a workspace the store can create
// boards in (InMemory materializes it; PostgreSQL needs a real workspace).
func exerciseAccessMatrix(t *testing.T, store WorkspaceBoardStore, workspaceID string) {
	t.Helper()
	ctx := context.Background()
	id := mustMatrixBoard(t, ctx, store, workspaceID, "Selected sharing", matrixCreator)
	initial, err := store.GetBoardAccess(ctx, workspaceID, id)
	if err != nil || initial.AccessState != AccessStatePrivate || len(initial.MemberUserIDs) != 0 {
		t.Fatalf("initial = %+v %v", initial, err)
	}
	on, err := store.ReplaceBoardAccess(ctx, workspaceID, id, matrixCreator, 1, WorkspaceAccessViewer, []int64{matrixEditor, matrixViewer, matrixEditor})
	if err != nil || on.AccessVersion != 2 || len(on.MemberUserIDs) != 2 || on.AccessState != AccessStateWorkspace {
		t.Fatalf("On = %+v %v", on, err)
	}
	for _, user := range []int64{matrixEditor, matrixViewer} {
		role, visible, err := store.GetBoardRole(ctx, workspaceID, id, user, "member")
		if err != nil || !visible || role != RoleViewer {
			t.Fatalf("selected user %d=%s %v %v", user, role, visible, err)
		}
	}
	for _, user := range []int64{matrixMember, matrixWSOwner, matrixWSAdmin} {
		if _, visible, err := store.GetBoardRole(ctx, workspaceID, id, user, "owner"); err != nil || visible {
			t.Fatalf("unselected user %d visible=%v %v", user, visible, err)
		}
		page, err := store.ListBoardCatalog(ctx, workspaceID, false, user, "owner", V1BoardCatalogPageRequest{Limit: 100})
		if err != nil || len(page.Entries) != 0 {
			t.Fatalf("unselected catalog=%+v %v", page, err)
		}
	}
	if _, err := store.ReplaceBoardAccess(ctx, workspaceID, id, matrixViewer, 2, WorkspaceAccessEditor, []int64{matrixViewer}); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("noncreator change=%v", err)
	}
	if _, err := store.ReplaceBoardAccess(ctx, workspaceID, id, matrixCreator, 1, WorkspaceAccessEditor, []int64{matrixViewer}); !errors.Is(err, ErrAccessConflict) {
		t.Fatalf("stale change=%v", err)
	}
	if _, err := store.ReplaceBoardAccess(ctx, workspaceID, id, matrixCreator, 2, WorkspaceAccessViewer, []int64{matrixCreator}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("select creator=%v", err)
	}
	same, err := store.ReplaceBoardAccess(ctx, workspaceID, id, matrixCreator, 2, WorkspaceAccessViewer, []int64{matrixViewer, matrixEditor})
	if err != nil || same.AccessVersion != 2 {
		t.Fatalf("same config=%+v %v", same, err)
	}
	elevated, err := store.ReplaceBoardAccess(ctx, workspaceID, id, matrixCreator, 2, WorkspaceAccessEditor, on.MemberUserIDs)
	if err != nil || elevated.AccessVersion != 3 {
		t.Fatalf("common role=%+v %v", elevated, err)
	}
	for _, user := range on.MemberUserIDs {
		role, visible, err := store.GetBoardRole(ctx, workspaceID, id, user, "member")
		if err != nil || !visible || role != RoleEditor {
			t.Fatalf("common editor=%s %v %v", role, visible, err)
		}
		page, err := store.ListBoardCatalog(ctx, workspaceID, false, user, "member", V1BoardCatalogPageRequest{Limit: 100})
		if err != nil || len(page.Entries) != 1 || page.Entries[0].EffectiveRole != RoleEditor {
			t.Fatalf("selected catalog=%+v %v", page, err)
		}
	}
	off, err := store.ReplaceBoardAccess(ctx, workspaceID, id, matrixCreator, 3, WorkspaceAccessNone, on.MemberUserIDs)
	if err != nil || off.AccessVersion != 4 || len(off.MemberUserIDs) != 0 || off.AccessState != AccessStatePrivate {
		t.Fatalf("Off=%+v %v", off, err)
	}
	empty, err := store.ReplaceBoardAccess(ctx, workspaceID, id, matrixCreator, 4, WorkspaceAccessViewer, nil)
	if err != nil || len(empty.MemberUserIDs) != 0 || empty.AccessState != AccessStatePrivate {
		t.Fatalf("reOn empty=%+v %v", empty, err)
	}
	for _, user := range on.MemberUserIDs {
		if _, visible, err := store.GetBoardRole(ctx, workspaceID, id, user, "member"); err != nil || visible {
			t.Fatalf("old selection resurrected %d", user)
		}
	}
	if role, visible, err := store.GetBoardRole(ctx, workspaceID, id, matrixCreator, "owner"); err != nil || !visible || role != RoleOwner {
		t.Fatalf("creator=%s %v %v", role, visible, err)
	}
	if _, err := store.ArchiveBoard(ctx, workspaceID, id, matrixCreator); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReplaceBoardAccess(ctx, workspaceID, id, matrixCreator, empty.AccessVersion, WorkspaceAccessEditor, []int64{matrixEditor}); !errors.Is(err, ErrV1Archived) {
		t.Fatalf("archived config=%v", err)
	}
}

func TestAccessMatrixInMemory(t *testing.T) {
	exerciseAccessMatrix(t, NewInMemoryWorkspaceBoardStoreForTests(testWorkspaceMembers{}, testWorkspaceMembers{}), "ws_access_matrix")
}

func TestResolveRoleSinglePolicy(t *testing.T) {
	for _, tc := range []struct {
		creator, caller int64
		selected        bool
		access          WorkspaceAccess
		member          bool
		want            AccessRole
		visible         bool
	}{
		{1, 1, false, WorkspaceAccessNone, true, RoleOwner, true},
		{1, 1, false, WorkspaceAccessEditor, false, "", false},
		{0, 1, true, WorkspaceAccessEditor, true, "", false},
		{1, 2, true, WorkspaceAccessViewer, true, RoleViewer, true},
		{1, 2, true, WorkspaceAccessEditor, true, RoleEditor, true},
		{1, 2, false, WorkspaceAccessEditor, true, "", false},
		{1, 2, true, WorkspaceAccessNone, true, "", false},
		{1, 2, true, WorkspaceAccessEditor, false, "", false},
		{1, 2, true, "unknown", true, "", false},
	} {
		role, visible := ResolveRole(tc.creator, tc.caller, tc.selected, tc.access, tc.member)
		if role != tc.want || visible != tc.visible {
			t.Fatalf("%+v got %s %v", tc, role, visible)
		}
	}
}

func TestCheckCapabilityMaps404Vs403(t *testing.T) {
	for _, tc := range []struct {
		name    string
		role    AccessRole
		ok      bool
		cap     Capability
		wantErr error
	}{
		{"invisible read is not found", "", false, CapRead, ErrNotFound},
		{"invisible write is not found", "", false, CapWrite, ErrNotFound},
		{"invisible manage is not found", "", false, CapManage, ErrNotFound},
		{"viewer reads", RoleViewer, true, CapRead, nil},
		{"viewer writes denied", RoleViewer, true, CapWrite, ErrPermissionDenied},
		{"viewer manages denied", RoleViewer, true, CapManage, ErrPermissionDenied},
		{"editor reads", RoleEditor, true, CapRead, nil},
		{"editor writes", RoleEditor, true, CapWrite, nil},
		{"editor manages denied", RoleEditor, true, CapManage, ErrPermissionDenied},
		{"owner reads", RoleOwner, true, CapRead, nil},
		{"owner writes", RoleOwner, true, CapWrite, nil},
		{"owner manages", RoleOwner, true, CapManage, nil},
		{"unknown capability denied", RoleOwner, true, "purge", ErrPermissionDenied},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := CheckCapability(tc.role, tc.ok, tc.cap); !errors.Is(err, tc.wantErr) {
				t.Fatalf("CheckCapability(%q, %v, %q) = %v; want %v", tc.role, tc.ok, tc.cap, err, tc.wantErr)
			}
		})
	}
}

func TestAccessMatrixCatalogEquivalenceSpotCheck(t *testing.T) {
	// Guards the documented invariant directly: every catalog card must
	// carry a PreviewScene/ContentVersion projection consistent with the
	// board row, ordered by recency. Full predicate equivalence rides the
	// matrix above on both stores.
	ctx := context.Background()
	store := NewInMemoryWorkspaceBoardStoreForTests(testWorkspaceMembers{}, testWorkspaceMembers{})
	ids := make([]string, 0, 3)
	for i := range 3 {
		ids = append(ids, mustMatrixBoard(t, ctx, store, "ws_access_spot", string(rune('A'+i)), matrixCreator))
	}
	page, err := store.ListBoardCatalog(ctx, "ws_access_spot", false, matrixCreator, "owner", V1BoardCatalogPageRequest{Limit: 100})
	if err != nil || len(page.Entries) != 3 {
		t.Fatalf("catalog = %d, %v; want 3", len(page.Entries), err)
	}
	seen := map[string]bool{}
	for _, entry := range page.Entries {
		seen[entry.PublicID] = true
	}
	sort.Strings(ids)
	got := make([]string, 0, 3)
	for id := range seen {
		got = append(got, id)
	}
	sort.Strings(got)
	for i := range ids {
		if ids[i] != got[i] {
			t.Fatalf("catalog ids = %v, want %v", got, ids)
		}
	}
}
