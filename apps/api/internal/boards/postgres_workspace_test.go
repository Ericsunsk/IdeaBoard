package boards

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"ideaboard/api/internal/boards/previewscene"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/tenancy"
	tenancypg "ideaboard/api/internal/tenancy/infrastructure/postgres"
	"ideaboard/api/internal/testdb"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Postgres WorkspaceBoardStore integration coverage (issue #802 seam,
// #809 follow-up). Runs against an isolated test schema via testdb; skips
// without API_TEST_DATABASE_DSN like every other PostgreSQL integration
// test. Mirrors the InMemoryWorkspaceBoardStore contract paths in
// workspace_test.go plus cross-workspace invisibility.

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
	tenancyStore := tenancypg.NewStore(db, nil)
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

func TestPostgresWorkspaceBoardStoreOrganizationClosureSuspendsOwnerAndAdmin(t *testing.T) {
	db := testdb.Open(t)
	ctx := context.Background()
	tenancyStore := tenancypg.NewStore(db, nil)
	org, err := tenancyStore.CreateOrganization(ctx, postgresStoreOwner, "closed-board-org")
	if err != nil {
		t.Fatalf("create organization failed: %v", err)
	}
	if _, err := tenancyStore.AddOrganizationMember(ctx, org.PublicID, postgresStoreMember, tenancy.RoleAdmin); err != nil {
		t.Fatalf("add organization admin failed: %v", err)
	}
	ws, err := tenancyStore.CreateOrganizationWorkspace(ctx, org.PublicID, postgresStoreOwner, "closed-board-space")
	if err != nil {
		t.Fatalf("create organization workspace failed: %v", err)
	}
	store := NewPostgresWorkspaceBoardStore(db, testDocumentAssetValidator)
	boardID := mustCreatePostgresBoard(t, ctx, store, ws, "Org board")
	if _, err := tenancyStore.CloseOrganization(ctx, org.PublicID, postgresStoreOwner, "suspend board access"); err != nil {
		t.Fatalf("close organization failed: %v", err)
	}

	for _, userID := range []int64{postgresStoreOwner, postgresStoreMember} {
		if _, visible, err := store.GetBoardRole(ctx, ws.PublicID, boardID, userID, ""); err != nil || visible {
			t.Fatalf("closed organization user %d visible=%t err=%v, want invisible", userID, visible, err)
		}
		if _, err := store.RenameBoard(ctx, ws.PublicID, boardID, "Blocked rename", userID); !errors.Is(err, ErrNotFound) {
			t.Errorf("closed organization user %d rename err=%v, want ErrNotFound", userID, err)
		}
		newBoardID, err := NewV1BoardPublicID()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.CreateBoard(ctx, ws.PublicID, userID, "Blocked create", newBoardID); !errors.Is(err, ErrNotFound) {
			t.Errorf("closed organization user %d create err=%v, want ErrNotFound", userID, err)
		}
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
	tenancyStore := tenancypg.NewStore(db, nil)
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
// API_TEST_DATABASE_DSN like every other Postgres integration test.
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
	tenancyStore := tenancypg.NewStore(db, nil)
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

// merged from board_media_refs_postgres_test.go (access/preview aggregation over the postgres store).
// PostgreSQL parity for board document -> media reference reconciliation
// (issue #936): the same accept/remove matrix runs unchanged on the
// postgres store, including the fail-closed join (document ids without a
// workspace-scoped asset row reconcile no reference). Skips without
// API_TEST_DATABASE_DSN like every other Postgres integration test.

// seedPGMediaAsset inserts a minimal completed session + asset row pair so
// document references resolve through the workspace-scoped join. Content
// bytes are irrelevant here; only identity + scope matter.
func seedPGMediaAsset(t *testing.T, ctx context.Context, pool *pgxpool.Pool, workspacePublicID, assetPublicID string) {
	t.Helper()
	suffix := strings.TrimPrefix(assetPublicID, "asset_")
	var sessionInternal int64
	err := pool.QueryRow(ctx,
		`INSERT INTO upload_sessions (public_id, workspace_id, uploader_user_id, declared_mime_type, declared_size_bytes, declared_checksum, storage_key, status, completed_at)
		SELECT $1, w.internal_id, $2, 'image/png', 64, $3, $4, 'completed', now()
		FROM workspaces AS w WHERE w.public_id = $5
		RETURNING internal_id`,
		fmt.Sprintf("upload_%s", suffix), matrixCreator, strings.Repeat("ab", 32),
		fmt.Sprintf("k/%s", suffix), workspacePublicID,
	).Scan(&sessionInternal)
	if err != nil {
		t.Fatalf("seed upload session: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO media_assets (public_id, workspace_id, upload_session_id, owner_user_id, storage_key, mime_type, size_bytes, checksum, media_kind, width, height)
		SELECT $1, w.internal_id, $2, $3, $4, 'image/png', 64, $5, 'image', 100, 80
		FROM workspaces AS w WHERE w.public_id = $6`,
		assetPublicID, sessionInternal, matrixCreator,
		fmt.Sprintf("k/%s", suffix), strings.Repeat("ab", 32), workspacePublicID,
	); err != nil {
		t.Fatalf("seed media asset: %v", err)
	}
}

func TestBoardMediaRefsPostgres(t *testing.T) {
	db := testdb.Open(t)
	ctx := context.Background()
	ws, err := tenancypg.NewStore(db, nil).CreateWorkspace(ctx, tenancy.CreateWorkspaceParams{
		Name:        "boards-media-refs-pg",
		OwnerUserID: matrixCreator,
	})
	if err != nil {
		t.Fatalf("create workspace failed: %v", err)
	}
	seedPGMediaAsset(t, ctx, db, ws.PublicID, "asset_aaaaaaaaaaaaaaaa")
	seedPGMediaAsset(t, ctx, db, ws.PublicID, "asset_bbbbbbbbbbbbbbbb")
	exerciseBoardMediaRefs(t, ctx, NewPostgresWorkspaceBoardStore(db, testDocumentAssetValidator), ws.PublicID, matrixCreator)

	// Fail-closed join: a document id without an asset row reconciles no
	// reference even though the extractor accepts its shape.
	boardID, err := NewV1BoardPublicID()
	if err != nil {
		t.Fatal(err)
	}
	store := NewPostgresWorkspaceBoardStore(db, testDocumentAssetValidator)
	if _, err := store.CreateBoard(ctx, ws.PublicID, matrixCreator, "phantom", boardID); err != nil {
		t.Fatalf("create board: %v", err)
	}
	if _, err := store.ReplaceDocument(ctx, ws.PublicID, boardID, matrixCreator, 1, 1,
		refsTestEnvelope(t, "asset_zzzzzzzzzzzzzzzz")); err != nil {
		t.Fatalf("replace with phantom asset: %v", err)
	}
	rows, err := store.ListBoardsReferencingAsset(ctx, ws.PublicID, "asset_zzzzzzzzzzzzzzzz")
	if err != nil {
		t.Fatalf("list phantom refs: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("phantom refs = %v, want none", rows)
	}
}

// merged from preview_catalog_postgres_test.go (access/preview aggregation over the postgres store).
func newPostgresPreviewFixture(t *testing.T) (context.Context, *pgxpool.Pool, *PostgresWorkspaceBoardStore, *tenancy.Workspace) {
	t.Helper()
	db := testdb.Open(t)
	ctx := context.Background()
	tenancyStore := tenancypg.NewStore(db, nil)
	ws, err := tenancyStore.CreateWorkspace(ctx, tenancy.CreateWorkspaceParams{
		Name:        "boards-preview-pg",
		OwnerUserID: postgresStoreOwner,
	})
	if err != nil {
		t.Fatalf("create workspace failed: %v", err)
	}
	seedBoardTestMembers(t, NewPostgresWorkspaceBoardStore(db, testDocumentAssetValidator), ws.InternalID, 3, 7, 8, 10)
	return ctx, db, NewPostgresWorkspaceBoardStore(db, testDocumentAssetValidator), ws
}

func nullOutPreviewColumns(t *testing.T, db *pgxpool.Pool, boardPublicID string) {
	t.Helper()
	_, err := db.Exec(context.Background(), `UPDATE board_documents AS d SET content_version = NULL, preview_scene = NULL FROM boards AS b WHERE d.board_id = b.internal_id AND b.public_id = $1`, boardPublicID)
	if err != nil {
		t.Fatalf("null out preview columns: %v", err)
	}
}

func TestPostgresPreviewSceneLifecycle(t *testing.T) {
	ctx, _, store, ws := newPostgresPreviewFixture(t)
	boardID := mustCreatePostgresBoard(t, ctx, store, ws, "Preview PG")

	created, err := store.GetBoard(ctx, ws.PublicID, boardID)
	if err != nil {
		t.Fatal(err)
	}
	if created.ContentVersion == "" || len(created.ContentVersion) != 64 {
		t.Fatalf("create content version = %q, want hex sha256", created.ContentVersion)
	}
	if len(created.PreviewScene) != 0 {
		t.Fatalf("fresh schema-1 document must carry no scene")
	}
	if created.SchemaVersion != 1 {
		t.Fatalf("create schema = %d, want 1", created.SchemaVersion)
	}

	envelope := schema4Envelope(t, schema4ImagePayload())
	updated, err := store.ReplaceDocument(ctx, ws.PublicID, boardID, postgresStoreOwner, 1, 4, envelope)
	if err != nil {
		t.Fatal(err)
	}
	if updated.ContentVersion != previewscene.ContentVersionOf(envelope) {
		t.Fatalf("replace content version = %q, want canonical hash", updated.ContentVersion)
	}
	scene, err := previewscene.ParseScene(updated.PreviewScene)
	if err != nil {
		t.Fatalf("stored scene does not parse: %v", err)
	}
	if scene.ContentVersion != updated.ContentVersion || len(scene.Nodes) != 1 {
		t.Fatalf("stored scene mismatch: %+v", scene)
	}

	catalog, err := store.ListBoardCatalog(ctx, ws.PublicID, false, postgresStoreOwner, "owner", V1BoardCatalogPageRequest{Limit: 100})
	if err != nil || len(catalog.Entries) != 1 {
		t.Fatalf("catalog = %v, %v; want 1 entry", catalog, err)
	}
	entry := catalog.Entries[0]
	if entry.PublicID != boardID || entry.ContentVersion != updated.ContentVersion ||
		string(entry.PreviewScene) != string(updated.PreviewScene) ||
		entry.Revision != 2 || entry.SchemaVersion != 4 {
		t.Fatalf("catalog entry diverges from stored projection: %+v", entry)
	}

	wantVersion, wantScene := updated.ContentVersion, string(updated.PreviewScene)
	if _, err := store.RenameBoard(ctx, ws.PublicID, boardID, "Preview PG v2", postgresStoreOwner); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ArchiveBoard(ctx, ws.PublicID, boardID, postgresStoreOwner); err != nil {
		t.Fatal(err)
	}
	if _, err := store.RestoreBoard(ctx, ws.PublicID, boardID, postgresStoreOwner); err != nil {
		t.Fatal(err)
	}
	afterMeta, err := store.GetBoard(ctx, ws.PublicID, boardID)
	if err != nil {
		t.Fatal(err)
	}
	if afterMeta.ContentVersion != wantVersion || string(afterMeta.PreviewScene) != wantScene {
		t.Fatalf("rename/archive/restore changed the content projection")
	}

	stale := schema4Envelope(t, schema4EmptyPayload())
	if _, err := store.ReplaceDocument(ctx, ws.PublicID, boardID, postgresStoreOwner, 1, 4, stale); err != ErrRevisionConflict {
		t.Fatalf("stale write = %v, want ErrRevisionConflict", err)
	}
	afterConflict, err := store.GetBoard(ctx, ws.PublicID, boardID)
	if err != nil {
		t.Fatal(err)
	}
	if afterConflict.Revision != 2 || afterConflict.ContentVersion != wantVersion || string(afterConflict.PreviewScene) != wantScene {
		t.Fatalf("conflict published a projection: revision %d version %q", afterConflict.Revision, afterConflict.ContentVersion)
	}
}

func TestPostgresPreviewBackfillAssignsMissingProjections(t *testing.T) {
	ctx, db, store, ws := newPostgresPreviewFixture(t)
	boardID := mustCreatePostgresBoard(t, ctx, store, ws, "Backfill PG")
	envelope := schema4Envelope(t, schema4ImagePayload())
	if _, err := store.ReplaceDocument(ctx, ws.PublicID, boardID, postgresStoreOwner, 1, 4, envelope); err != nil {
		t.Fatal(err)
	}
	// Canonical document bytes remain intact while the projection is missing.
	nullOutPreviewColumns(t, db, boardID)

	dry, err := previewscene.BackfillBoardPreviews(ctx, dbsqlc.New(db), 200, 0, true)
	if err != nil {
		t.Fatal(err)
	}
	if dry.Scanned != 1 || dry.Updated != 1 || dry.Scenes != 1 {
		t.Fatalf("dry run stats = %+v, want 1/1/1", dry)
	}
	stillNull, err := store.GetBoard(ctx, ws.PublicID, boardID)
	if err != nil {
		t.Fatal(err)
	}
	if stillNull.ContentVersion != "" || len(stillNull.PreviewScene) != 0 {
		t.Fatalf("dry run must not write")
	}

	stats, err := previewscene.BackfillBoardPreviews(ctx, dbsqlc.New(db), 200, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Scanned != 1 || stats.Updated != 1 || stats.Scenes != 1 {
		t.Fatalf("backfill stats = %+v, want 1/1/1", stats)
	}
	healed, err := store.GetBoard(ctx, ws.PublicID, boardID)
	if err != nil {
		t.Fatal(err)
	}
	if healed.ContentVersion != previewscene.ContentVersionOf(envelope) {
		t.Fatalf("backfilled version = %q, want canonical hash", healed.ContentVersion)
	}
	if _, err := previewscene.ParseScene(healed.PreviewScene); err != nil {
		t.Fatalf("backfilled scene does not parse: %v", err)
	}
	// The backfill must not reorder the Gallery: boards.updated_at is
	// untouched, so catalog order is stable across the run.
	catalog, err := store.ListBoardCatalog(ctx, ws.PublicID, false, postgresStoreOwner, "owner", V1BoardCatalogPageRequest{Limit: 100})
	if err != nil || len(catalog.Entries) != 1 || catalog.Entries[0].PublicID != boardID {
		t.Fatalf("catalog after backfill = %v, %v", catalog, err)
	}

	again, err := previewscene.BackfillBoardPreviews(ctx, dbsqlc.New(db), 200, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	if again.Scanned != 0 || again.Updated != 0 {
		t.Fatalf("backfill is not idempotent: %+v", again)
	}
}

func TestPostgresPreviewBackfillNonExtractableKeepsNullScene(t *testing.T) {
	ctx, db, store, ws := newPostgresPreviewFixture(t)
	boardID := mustCreatePostgresBoard(t, ctx, store, ws, "Backfill legacy PG")
	// Store a schema-2 document through the normal path, then simulate a
	// row with a missing projection.
	schema2 := json.RawMessage(`{"schema_version":2,"payload":{"nodes":[]}}`)
	if _, err := store.ReplaceDocument(ctx, ws.PublicID, boardID, postgresStoreOwner, 1, 2, schema2); err != nil {
		t.Fatal(err)
	}
	nullOutPreviewColumns(t, db, boardID)

	stats, err := previewscene.BackfillBoardPreviews(ctx, dbsqlc.New(db), 200, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Scanned != 1 || stats.Updated != 1 || stats.Scenes != 0 {
		t.Fatalf("backfill stats = %+v, want 1/1/0", stats)
	}
	healed, err := store.GetBoard(ctx, ws.PublicID, boardID)
	if err != nil {
		t.Fatal(err)
	}
	if healed.ContentVersion != previewscene.ContentVersionOf(schema2) {
		t.Fatalf("backfilled version = %q, want canonical hash", healed.ContentVersion)
	}
	if len(healed.PreviewScene) != 0 {
		t.Fatalf("non-extractable document must keep a NULL scene")
	}
}

func TestPostgresPreviewCatalogFiltersEffectiveAccess(t *testing.T) {
	_, _, store, ws := newPostgresPreviewFixture(t)
	exerciseCatalogVisibility(t, store, ws.PublicID)
}

func TestPostgresPreviewCatalogRoleParityAndWorkspaceScope(t *testing.T) {
	ctx, db, store, ws := newPostgresPreviewFixture(t)
	boardID := mustCreatePostgresBoard(t, ctx, store, ws, "Role parity")
	if _, err := store.ReplaceBoardAccess(ctx, ws.PublicID, boardID, postgresStoreOwner, 1, WorkspaceAccessEditor, []int64{postgresStoreMember, postgresStoreOutsider}); err != nil {
		t.Fatal(err)
	}

	other, err := tenancypg.NewStore(db, nil).CreateWorkspace(ctx, tenancy.CreateWorkspaceParams{
		Name: "Foreign catalog", OwnerUserID: postgresStoreExtra,
	})
	if err != nil {
		t.Fatal(err)
	}
	seedBoardTestMembers(t, store, other.InternalID, postgresStoreOwner)
	mustCreatePostgresBoard(t, ctx, store, other, "Foreign board")
	for _, tc := range []struct {
		name          string
		user          int64
		workspaceRole string
		wantRole      AccessRole
	}{
		{"creator-without-membership-denied", postgresStoreOwner, "", ""},
		{"explicit-editor-with-membership", postgresStoreMember, "member", RoleEditor},
		{"explicit-grant-without-membership-denied", postgresStoreMember, "", ""},
		{"selected-common-editor", postgresStoreOutsider, "owner", RoleEditor},
		{"no-implicit-owner", postgresStoreExtra, "owner", ""},
		{"no-implicit-admin", postgresStoreExtra, "admin", ""},
		{"no-implicit-member", postgresStoreExtra, "member", ""},
		{"trimmed-member-without-grant-denied", postgresStoreExtra, " member ", ""},
		{"no-workspace-role", postgresStoreExtra, "", ""},
		{"unknown-workspace-role", postgresStoreExtra, "unknown", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			role, allowed, err := store.GetBoardRole(ctx, ws.PublicID, boardID, tc.user, tc.workspaceRole)
			if err != nil || allowed != (tc.wantRole != "") || role != tc.wantRole {
				t.Fatalf("effective role=%q allowed=%v err=%v; want %q", role, allowed, err, tc.wantRole)
			}
			catalog, err := store.ListBoardCatalog(ctx, ws.PublicID, true, tc.user, tc.workspaceRole, V1BoardCatalogPageRequest{Limit: 100})
			if err != nil {
				t.Fatal(err)
			}
			if !allowed {
				if len(catalog.Entries) != 0 {
					t.Fatalf("denied catalog count=%d, want 0", len(catalog.Entries))
				}
				return
			}
			if len(catalog.Entries) != 1 || catalog.Entries[0].PublicID != boardID || catalog.Entries[0].WorkspacePublicID != ws.PublicID {
				t.Fatalf("catalog does not match effective access in requested workspace: count=%d", len(catalog.Entries))
			}
		})
	}
}

type catalogQueryTrace struct {
	queries []string
	rows    []int64
	args    [][]any
}

func (trace *catalogQueryTrace) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	trace.queries = append(trace.queries, data.SQL)
	trace.args = append(trace.args, data.Args)
	return ctx
}

func (trace *catalogQueryTrace) TraceQueryEnd(_ context.Context, _ *pgx.Conn, data pgx.TraceQueryEndData) {
	trace.rows = append(trace.rows, data.CommandTag.RowsAffected())
}

func TestPostgresPreviewCatalogUsesOneLightweightRead(t *testing.T) {
	ctx, db, store, ws := newPostgresPreviewFixture(t)
	for range 3 {
		boardID := mustCreatePostgresBoard(t, ctx, store, ws, "One read")
		// Explicit grants authorize the reader (workspace_access=none
		// carries no implicit visibility).
		if _, err := store.ReplaceBoardAccess(ctx, ws.PublicID, boardID, postgresStoreOwner, 1, WorkspaceAccess(RoleViewer), []int64{postgresStoreMember}); err != nil {
			t.Fatal(err)
		}
	}
	trace := &catalogQueryTrace{}
	cfg := db.Config()
	cfg.ConnConfig.Tracer = trace
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	catalog, err := NewPostgresWorkspaceBoardStore(pool, testDocumentAssetValidator).ListBoardCatalog(ctx, ws.PublicID, false, postgresStoreMember, "member", V1BoardCatalogPageRequest{Limit: 1})
	if err != nil || len(catalog.Entries) != 1 || !catalog.HasMore {
		t.Fatalf("catalog count=%d err=%v", len(catalog.Entries), err)
	}
	if len(trace.queries) != 1 {
		t.Fatalf("catalog executed %d SQL statements; want 1", len(trace.queries))
	}
	if strings.Contains(trace.queries[0], "snapshot_data") {
		t.Fatal("catalog query reads the full document")
	}
	if len(trace.rows) != 1 || trace.rows[0] != 1 {
		t.Fatalf("PostgreSQL returned row counts %v; want only one page row", trace.rows)
	}
	if len(trace.args[0]) != 7 || trace.args[0][4] != int32(maxBoardCatalogRows) || trace.args[0][5] != int32(0) || trace.args[0][6] != int32(1) {
		t.Fatalf("query did not receive bounded page window")
	}
}

func TestPostgresPreviewCatalogAuthorizesBeforeLimit(t *testing.T) {
	ctx, db, store, ws := newPostgresPreviewFixture(t)
	boardID := mustCreatePostgresBoard(t, ctx, store, ws, "Older shared board")
	if _, err := store.ReplaceBoardAccess(ctx, ws.PublicID, boardID, postgresStoreOwner, 1, WorkspaceAccess(RoleViewer), []int64{postgresStoreOutsider}); err != nil {
		t.Fatal(err)
	}
	_, err := db.Exec(ctx, `WITH inserted AS (
		INSERT INTO boards (public_id, workspace_id, owner_user_id, title, updated_at)
		SELECT 'board_denied_' || n, w.internal_id, $2, 'Denied board', now() + interval '1 day'
		FROM workspaces AS w CROSS JOIN generate_series(1, 501) AS n
		WHERE w.public_id = $1 RETURNING internal_id
	) INSERT INTO board_documents (board_id, schema_version, snapshot_data, etag)
	SELECT internal_id, 1, '{"schema_version":1,"payload":{}}'::jsonb, 'test-etag' FROM inserted`, ws.PublicID, postgresStoreOwner)
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := store.ListBoardCatalog(ctx, ws.PublicID, false, postgresStoreOutsider, "member", V1BoardCatalogPageRequest{Limit: 100})
	if err != nil || len(catalog.Entries) != 1 || catalog.Entries[0].PublicID != boardID {
		t.Fatalf("older accessible board lost behind denied rows: count=%d err=%v", len(catalog.Entries), err)
	}
}

func TestPostgresBoardCatalogPageContract(t *testing.T) {
	ctx, db, store, ws := newPostgresPreviewFixture(t)
	if _, err := db.Exec(ctx, `ALTER TABLE boards DISABLE TRIGGER set_boards_updated_at`); err != nil {
		t.Fatal(err)
	}

	exerciseCatalogPageContract(t, store, ws.PublicID, func(id string, at time.Time) {
		if _, err := db.Exec(ctx, `UPDATE boards SET updated_at = $1 WHERE public_id = $2`, at, id); err != nil {
			t.Fatal(err)
		}
	})
}

func TestPostgresBoardCatalogPageCap(t *testing.T) {
	ctx, db, store, ws := newPostgresPreviewFixture(t)
	_, err := db.Exec(ctx, `WITH inserted AS (
		INSERT INTO boards (public_id, workspace_id, owner_user_id, title, updated_at)
		SELECT 'board_' || lpad(n::text, 4, '0'), w.internal_id, $2, 'Capped board', '2026-10-15'::timestamptz
		FROM workspaces AS w CROSS JOIN generate_series(1, 505) AS n
		WHERE w.public_id = $1 RETURNING internal_id
	) INSERT INTO board_documents (board_id, schema_version, snapshot_data, etag)
	SELECT internal_id, 1, '{"schema_version":1,"payload":{}}'::jsonb, 'test-etag' FROM inserted`, ws.PublicID, postgresStoreOwner)
	if err != nil {
		t.Fatal(err)
	}
	assertCatalogCap(t, store, ws.PublicID, postgresStoreOwner)
}
