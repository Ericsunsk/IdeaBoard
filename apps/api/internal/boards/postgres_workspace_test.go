package boards

// Postgres WorkspaceBoardStore integration coverage (issue #802 seam,
// #809 follow-up). Runs against an isolated test schema via testdb; skips
// without API_TEST_DATABASE_DSN like every other PostgreSQL integration
// test. Mirrors the InMemoryWorkspaceBoardStore contract paths in
// v1_workspace_test.go plus cross-workspace invisibility.

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sync"
	"testing"

	"ideaboard/api/internal/tenancy"
	tenancypg "ideaboard/api/internal/tenancy/postgres"
	"ideaboard/api/internal/testdb"
)

const (
	postgresStoreOwner    = int64(1)
	postgresStoreMember   = int64(3)
	postgresStoreOutsider = int64(7)
	postgresStoreExtra    = int64(8)
)

func newPostgresWorkspaceBoardFixture(t *testing.T) (context.Context, *PostgresWorkspaceBoardStore, *tenancy.Workspace) {
	t.Helper()
	db := testdb.Open(t)
	ctx := context.Background()
	tenancyStore := tenancypg.NewStore(db)
	ws, err := tenancyStore.CreateWorkspace(ctx, tenancy.CreateWorkspaceParams{
		Name:        "boards-pg",
		OwnerUserID: postgresStoreOwner,
	})
	if err != nil {
		t.Fatalf("create workspace failed: %v", err)
	}
	seedBoardTestMembers(t, NewPostgresWorkspaceBoardStore(db, testDocumentAssetValidator), ws.InternalID, 3, 7, 8, 10)
	return ctx, NewPostgresWorkspaceBoardStore(db, testDocumentAssetValidator), ws
}

func mustCreatePostgresBoard(t *testing.T, ctx context.Context, store *PostgresWorkspaceBoardStore, ws *tenancy.Workspace, name string) string {
	t.Helper()
	boardID, err := NewV1BoardPublicID()
	if err != nil {
		t.Fatalf("mint board id failed: %v", err)
	}
	if _, err := store.CreateBoard(ctx, ws.PublicID, postgresStoreOwner, name, boardID); err != nil {
		t.Fatalf("create board failed: %v", err)
	}
	return boardID
}

func TestPostgresWorkspaceBoardStoreLifecycle(t *testing.T) {
	ctx, store, ws := newPostgresWorkspaceBoardFixture(t)

	boardID := mustCreatePostgresBoard(t, ctx, store, ws, "Launch")

	got, err := store.GetBoard(ctx, ws.PublicID, boardID)
	if err != nil {
		t.Fatalf("get board failed: %v", err)
	}
	if got.Revision != 1 || got.SchemaVersion != 1 || got.OwnerUserID != postgresStoreOwner {
		t.Fatalf("board = revision %d schema %d owner %d, want 1/1/%d", got.Revision, got.SchemaVersion, got.OwnerUserID, postgresStoreOwner)
	}
	if FormatV1ETag(got.Revision) != "1" {
		t.Fatalf("etag = %q, want 1", FormatV1ETag(got.Revision))
	}

	renamed, err := store.RenameBoard(ctx, ws.PublicID, boardID, "Renamed", postgresStoreOwner)
	if err != nil {
		t.Fatalf("rename failed: %v", err)
	}
	if renamed.Name != "Renamed" || renamed.Revision != 1 {
		t.Fatalf("renamed = %q rev %d, want Renamed/1", renamed.Name, renamed.Revision)
	}

	listed, err := store.ListBoardCatalog(ctx, ws.PublicID, false, postgresStoreOwner, "owner", V1BoardCatalogPageRequest{Limit: 100})
	if err != nil || len(listed.Entries) != 1 {
		t.Fatalf("list = %d boards err %v, want 1", len(listed.Entries), err)
	}

	archived, err := store.ArchiveBoard(ctx, ws.PublicID, boardID, postgresStoreOwner)
	if err != nil || archived.ArchivedAt == nil {
		t.Fatalf("archive failed: %v", err)
	}
	if _, err := store.ArchiveBoard(ctx, ws.PublicID, boardID, postgresStoreOwner); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second archive err = %v, want ErrNotFound", err)
	}
	if _, err := store.RenameBoard(ctx, ws.PublicID, boardID, "Nope", postgresStoreOwner); !errors.Is(err, ErrV1Archived) {
		t.Fatalf("rename archived err = %v, want ErrV1Archived", err)
	}
	visible, err := store.ListBoardCatalog(ctx, ws.PublicID, false, postgresStoreOwner, "owner", V1BoardCatalogPageRequest{Limit: 100})
	if err != nil || len(visible.Entries) != 0 {
		t.Fatalf("list live = %d err %v, want 0", len(visible.Entries), err)
	}
	all, err := store.ListBoardCatalog(ctx, ws.PublicID, true, postgresStoreOwner, "owner", V1BoardCatalogPageRequest{Limit: 100})
	if err != nil || len(all.Entries) != 1 {
		t.Fatalf("list all = %d err %v, want 1", len(all.Entries), err)
	}
	if _, err := store.RestoreBoard(ctx, ws.PublicID, boardID, postgresStoreOwner); err != nil {
		t.Fatalf("restore failed: %v", err)
	}
	if _, err := store.RestoreBoard(ctx, ws.PublicID, boardID, postgresStoreOwner); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second restore err = %v, want ErrNotFound", err)
	}
}

func TestPostgresWorkspaceBoardStoreRejectsLegacyIDs(t *testing.T) {
	ctx, store, ws := newPostgresWorkspaceBoardFixture(t)

	if _, err := store.CreateBoard(ctx, ws.PublicID, postgresStoreOwner, "Legacy", "board-1"); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("create legacy id err = %v, want ErrInvalidInput", err)
	}
	if _, _, err := store.GetBoardRole(ctx, ws.PublicID, "board-1", postgresStoreOwner, "owner"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("legacy role err = %v, want ErrNotFound", err)
	}
}

func TestPostgresWorkspaceBoardStoreDocumentRevisions(t *testing.T) {
	ctx, store, ws := newPostgresWorkspaceBoardFixture(t)
	boardID := mustCreatePostgresBoard(t, ctx, store, ws, "Docs")

	snapshot := json.RawMessage(`{"schema_version":1,"payload":{"nodes":[]}}`)
	updated, err := store.ReplaceDocument(ctx, ws.PublicID, boardID, postgresStoreOwner, 1, 1, snapshot)
	if err != nil {
		t.Fatalf("replace failed: %v", err)
	}
	if updated.Revision != 2 {
		t.Fatalf("revision = %d, want 2", updated.Revision)
	}
	if _, err := store.ReplaceDocument(ctx, ws.PublicID, boardID, postgresStoreOwner, 1, 1, snapshot); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("stale replace err = %v, want ErrRevisionConflict", err)
	}

	revisions, err := store.ListRevisions(ctx, ws.PublicID, boardID)
	if err != nil || len(revisions) != 2 {
		t.Fatalf("revisions = %d err %v, want 2", len(revisions), err)
	}
	if revisions[0].Revision != 1 || revisions[0].SourceRevision != 0 {
		t.Fatalf("rev1 = %+v, want revision 1 source 0", revisions[0])
	}
	if revisions[1].Revision != 2 || revisions[1].SourceRevision != 1 {
		t.Fatalf("rev2 = %+v, want revision 2 source 1", revisions[1])
	}
	byNumber, err := store.GetRevision(ctx, ws.PublicID, boardID, 2)
	if err != nil {
		t.Fatalf("get rev 2 failed: %v", err)
	}
	// JSONB normalizes key order on write: compare semantically.
	var wantDoc, gotDoc map[string]any
	if err := json.Unmarshal(snapshot, &wantDoc); err != nil {
		t.Fatalf("unmarshal want failed: %v", err)
	}
	if err := json.Unmarshal(byNumber.Snapshot, &gotDoc); err != nil {
		t.Fatalf("unmarshal got failed: %v", err)
	}
	if !reflect.DeepEqual(wantDoc, gotDoc) {
		t.Fatalf("rev 2 snapshot = %s, want %s", string(byNumber.Snapshot), string(snapshot))
	}
	byID, err := store.GetRevision(ctx, ws.PublicID, boardID, revisions[0].ID)
	if err != nil || byID.Revision != 1 {
		t.Fatalf("get rev by id err = %v", err)
	}
	if _, err := store.GetRevision(ctx, ws.PublicID, boardID, 99); !errors.Is(err, ErrNotFound) {
		t.Fatalf("get rev 99 err = %v, want ErrNotFound", err)
	}
}

func TestPostgresWorkspaceBoardStoreCrossWorkspaceInvisible(t *testing.T) {
	ctx, store, ws := newPostgresWorkspaceBoardFixture(t)
	boardID := mustCreatePostgresBoard(t, ctx, store, ws, "Private")

	db := store.db
	tenancyStore := tenancypg.NewStore(db)
	other, err := tenancyStore.CreateWorkspace(ctx, tenancy.CreateWorkspaceParams{
		Name:        "boards-pg-other",
		OwnerUserID: postgresStoreOutsider,
	})
	if err != nil {
		t.Fatalf("create other workspace failed: %v", err)
	}

	if _, err := store.GetBoard(ctx, other.PublicID, boardID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign get err = %v, want ErrNotFound", err)
	}
	if _, _, err := store.GetBoardRole(ctx, other.PublicID, boardID, postgresStoreOwner, "owner"); err == nil {
		t.Fatal("foreign role succeeded, want error")
	} else if !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign role err = %v, want ErrNotFound", err)
	}
	if page, err := store.ListBoardCatalog(ctx, other.PublicID, true, postgresStoreOutsider, "owner", V1BoardCatalogPageRequest{Limit: 100}); err != nil || len(page.Entries) != 0 {
		t.Fatalf("foreign catalog = %v, %v; want no entries", page, err)
	}
}

// TestPostgresSharingOffConcurrent pins the #941 revocation race on real
// PostgreSQL: concurrent sharing Off writers at one version serialize on
// the board row lock and losers read ErrAccessConflict (412 at the edge),
// never silent last-write-wins. A mixed access-write/sharing Off race
// admits exactly one winner, repeats on the current version converge
// without moving it, and non-creators stay denied. Skips without
// API_TEST_DATABASE_DSN like every other PG integration test.
func TestPostgresSharingOffConcurrent(t *testing.T) {
	ctx, store, ws := newPostgresWorkspaceBoardFixture(t)

	boardID := mustCreatePostgresBoard(t, ctx, store, ws, "Concurrent private")
	if _, err := store.ReplaceBoardAccess(ctx, ws.PublicID, boardID, postgresStoreOwner, 1, WorkspaceAccess(RoleViewer), []int64{postgresStoreMember}); err != nil {
		t.Fatalf("grant viewer: %v", err)
	}

	const writers = 8
	type outcome struct {
		only *V1BoardAccess
		err  error
	}
	results := make(chan outcome, writers)
	var wg sync.WaitGroup
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			only, err := store.ReplaceBoardAccess(ctx, ws.PublicID, boardID, postgresStoreOwner, 2, WorkspaceAccessNone, nil)
			results <- outcome{only: only, err: err}
		}()
	}
	wg.Wait()
	close(results)
	wins, conflicts := 0, 0
	for result := range results {
		if result.err == nil {
			wins++
			if result.only == nil || result.only.AccessVersion != 3 || result.only.AccessState != AccessStatePrivate {
				t.Fatalf("winning sharing Off = %+v, want version 3 private", result.only)
			}
			continue
		}
		if !errors.Is(result.err, ErrAccessConflict) {
			t.Fatalf("losing sharing Off err = %v, want ErrAccessConflict", result.err)
		}
		conflicts++
	}
	if wins != 1 || conflicts != writers-1 {
		t.Fatalf("wins=%d conflicts=%d, want 1 and %d", wins, conflicts, writers-1)
	}
	if _, ok, _ := store.GetBoardRole(ctx, ws.PublicID, boardID, postgresStoreMember, "member"); ok {
		t.Fatal("granted member must lose visibility after the winning reset")
	}
	// Repeating on the current version converges without moving it.
	again, err := store.ReplaceBoardAccess(ctx, ws.PublicID, boardID, postgresStoreOwner, 3, WorkspaceAccessNone, nil)
	if err != nil || again.AccessVersion != 3 {
		t.Fatalf("sharing Off replay = %+v, %v; want version 3", again, err)
	}
	// Stale versions conflict; visible non-creators stay denied.
	if _, err := store.ReplaceBoardAccess(ctx, ws.PublicID, boardID, postgresStoreOwner, 2, WorkspaceAccessNone, nil); !errors.Is(err, ErrAccessConflict) {
		t.Fatalf("stale sharing Off = %v, want ErrAccessConflict", err)
	}

	// Mixed race: one access-write and one sharing Off at one version
	// admit exactly one winner; the board converges to the winner's fact.
	raced := mustCreatePostgresBoard(t, ctx, store, ws, "Concurrent mixed")
	if _, err := store.ReplaceBoardAccess(ctx, ws.PublicID, raced, postgresStoreOwner, 1, WorkspaceAccess(RoleViewer), []int64{postgresStoreMember}); err != nil {
		t.Fatalf("grant raced viewer: %v", err)
	}
	type mixedOutcome struct {
		winner string
		err    error
	}
	mixed := make(chan mixedOutcome, 2)
	var mixedWG sync.WaitGroup
	mixedWG.Add(2)
	go func() {
		defer mixedWG.Done()
		_, err := store.ReplaceBoardAccess(ctx, ws.PublicID, raced, postgresStoreOwner, 2, WorkspaceAccessViewer, nil)
		mixed <- mixedOutcome{winner: "access", err: err}
	}()
	go func() {
		defer mixedWG.Done()
		_, err := store.ReplaceBoardAccess(ctx, ws.PublicID, raced, postgresStoreOwner, 2, WorkspaceAccessNone, nil)
		mixed <- mixedOutcome{winner: "private", err: err}
	}()
	mixedWG.Wait()
	close(mixed)
	succeeded := 0
	for result := range mixed {
		if result.err == nil {
			succeeded++
			continue
		}
		if !errors.Is(result.err, ErrAccessConflict) {
			t.Fatalf("mixed loser (%s) err = %v, want ErrAccessConflict", result.winner, result.err)
		}
	}
	if succeeded != 1 {
		t.Fatalf("mixed race successes = %d, want exactly 1", succeeded)
	}
	access, err := store.GetBoardAccess(ctx, ws.PublicID, raced)
	if err != nil {
		t.Fatalf("get raced access: %v", err)
	}
	if access.AccessVersion != 3 {
		t.Fatalf("raced version = %d, want exactly one bump to 3", access.AccessVersion)
	}
}

// TestPostgresMemberGrantPurgeParity pins RemoveWorkspaceMemberGrants on
// real PostgreSQL beyond the shared matrix: multi-board deterministic
// order, live-version bumps with grant deletes in one transaction, and a
// version-stable repeat. Skips without API_TEST_DATABASE_DSN.
func TestPostgresMemberGrantPurgeParity(t *testing.T) {
	ctx, store, ws := newPostgresWorkspaceBoardFixture(t)

	first := mustCreatePostgresBoard(t, ctx, store, ws, "Purge first")
	second := mustCreatePostgresBoard(t, ctx, store, ws, "Purge second")
	for _, boardID := range []string{first, second} {
		if _, err := store.ReplaceBoardAccess(ctx, ws.PublicID, boardID, postgresStoreOwner, 1, WorkspaceAccess(RoleViewer), []int64{postgresStoreMember}); err != nil {
			t.Fatalf("grant viewer on %s: %v", boardID, err)
		}
	}
	cleaned, err := cleanupMemberGrantsForTests(ctx, store, ws.PublicID, postgresStoreMember)
	if err != nil {
		t.Fatalf("purge grants: %v", err)
	}
	if len(cleaned) != 2 {
		t.Fatalf("purged boards = %+v, want both boards", cleaned)
	}
	// Deterministic public-id order, one version bump each.
	if cleaned[0].BoardPublicID > cleaned[1].BoardPublicID {
		t.Fatalf("purge order = %q then %q, want public_id ASC", cleaned[0].BoardPublicID, cleaned[1].BoardPublicID)
	}
	for _, entry := range cleaned {
		if entry.AccessVersion != 3 {
			t.Fatalf("purge entry = %+v, want live version 3", entry)
		}
		if _, ok, _ := store.GetBoardRole(ctx, ws.PublicID, entry.BoardPublicID, postgresStoreMember, "member"); ok {
			t.Fatalf("departed member still visible on %s", entry.BoardPublicID)
		}
	}
	// Repeat: version-stable no-op, and the rest of the workspace is
	// untouched (creator still owns both boards).
	again, err := cleanupMemberGrantsForTests(ctx, store, ws.PublicID, postgresStoreMember)
	if err != nil || len(again) != 0 {
		t.Fatalf("purge repeat = %+v, %v; want empty no-op", again, err)
	}
	for _, boardID := range []string{first, second} {
		access, err := store.GetBoardAccess(ctx, ws.PublicID, boardID)
		if err != nil || access.AccessVersion != 3 {
			t.Fatalf("board %s access = %+v, %v; want version 3", boardID, access, err)
		}
		if _, ok, _ := store.GetBoardRole(ctx, ws.PublicID, boardID, postgresStoreOwner, "owner"); !ok {
			t.Fatalf("creator lost visibility on %s", boardID)
		}
	}
	// Rejoin after purge mints a fresh grant (no stale row resurrects):
	// the re-grant succeeds and moves the version again.
	if _, err := store.ReplaceBoardAccess(ctx, ws.PublicID, first, postgresStoreOwner, 3, WorkspaceAccess(RoleViewer), []int64{postgresStoreMember}); err != nil {
		t.Fatalf("re-grant after purge: %v", err)
	}
	if access, err := store.GetBoardAccess(ctx, ws.PublicID, first); err != nil || access.AccessVersion != 4 {
		t.Fatalf("board %s access after rejoin = %+v, %v; want version 4", first, access, err)
	}
}

// TestPostgresWorkspaceLeaveRejoinGrantsNothing verifies membership departure
// removes direct grants atomically, and rejoining grants no Board access.
func TestPostgresWorkspaceLeaveRejoinGrantsNothing(t *testing.T) {
	db := testdb.Open(t)
	ctx := context.Background()
	tenancyStore := tenancypg.NewStore(db)
	ws, err := tenancyStore.CreateWorkspace(ctx, tenancy.CreateWorkspaceParams{
		Name:        "boards-pg-rejoin",
		OwnerUserID: postgresStoreOwner,
	})
	if err != nil {
		t.Fatalf("create workspace failed: %v", err)
	}
	if _, err := tenancyStore.AddWorkspaceMember(ctx, ws.PublicID, postgresStoreMember, tenancy.RoleMember); err != nil {
		t.Fatalf("add workspace member failed: %v", err)
	}
	store := NewPostgresWorkspaceBoardStore(db, testDocumentAssetValidator)
	boardID := mustCreatePostgresBoard(t, ctx, store, ws, "Rejoin cycle")
	if _, err := store.ReplaceBoardAccess(ctx, ws.PublicID, boardID, postgresStoreOwner, 1, WorkspaceAccess(RoleViewer), []int64{postgresStoreMember}); err != nil {
		t.Fatalf("grant viewer: %v", err)
	}
	if _, ok, _ := store.GetBoardRole(ctx, ws.PublicID, boardID, postgresStoreMember, "member"); !ok {
		t.Fatal("member must see the board before leaving")
	}
	if _, err := tenancyStore.RemoveWorkspaceMember(ctx, ws.PublicID, postgresStoreOwner, postgresStoreMember, store); err != nil {
		t.Fatalf("remove workspace member failed: %v", err)
	}
	if _, ok, _ := store.GetBoardRole(ctx, ws.PublicID, boardID, postgresStoreMember, ""); ok {
		t.Fatal("departed member must lose visibility after leave")
	}
	if _, err := tenancyStore.AddWorkspaceMember(ctx, ws.PublicID, postgresStoreMember, tenancy.RoleMember); err != nil {
		t.Fatalf("re-add workspace member failed: %v", err)
	}
	if _, ok, _ := store.GetBoardRole(ctx, ws.PublicID, boardID, postgresStoreMember, "member"); ok {
		t.Fatal("rejoined member must stay invisible without an explicit re-grant")
	}
	members, err := store.ListBoardMembers(ctx, ws.PublicID, boardID)
	if err != nil {
		t.Fatalf("list members: %v", err)
	}
	for _, member := range members {
		if member.UserID == postgresStoreMember {
			t.Fatalf("rejoined member resurrected in roster: %+v", member)
		}
	}
}
