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
	"testing"

	"ideaboard/api/internal/platform/contract"
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
	return ctx, NewPostgresWorkspaceBoardStore(db), ws
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

	renamed, err := store.RenameBoard(ctx, ws.PublicID, boardID, "Renamed")
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

	archived, err := store.ArchiveBoard(ctx, ws.PublicID, boardID)
	if err != nil || archived.ArchivedAt == nil {
		t.Fatalf("archive failed: %v", err)
	}
	if _, err := store.ArchiveBoard(ctx, ws.PublicID, boardID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second archive err = %v, want ErrNotFound", err)
	}
	if _, err := store.RenameBoard(ctx, ws.PublicID, boardID, "Nope"); !errors.Is(err, ErrV1Archived) {
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
	if _, err := store.RestoreBoard(ctx, ws.PublicID, boardID); err != nil {
		t.Fatalf("restore failed: %v", err)
	}
	if _, err := store.RestoreBoard(ctx, ws.PublicID, boardID); !errors.Is(err, ErrNotFound) {
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

func TestPostgresWorkspaceBoardStoreMembers(t *testing.T) {
	ctx, store, ws := newPostgresWorkspaceBoardFixture(t)
	boardID := mustCreatePostgresBoard(t, ctx, store, ws, "Members")

	// Seed visible workspace memberships: member defaults to viewer,
	// a second workspace owner defaults to owner; outsiders stay denied.
	db := store.db
	tenancyStore := tenancypg.NewStore(db)
	wsOwnerRole, wsOwnerMember, err := tenancyStore.GetWorkspaceRole(ctx, ws.PublicID, postgresStoreOwner)
	if err != nil || !wsOwnerMember {
		t.Fatalf("fixture owner workspace role = %q member=%v err=%v, want owner member", wsOwnerRole, wsOwnerMember, err)
	}
	if _, err := tenancyStore.AddWorkspaceMember(ctx, ws.PublicID, postgresStoreMember, tenancy.RoleMember); err != nil {
		t.Fatalf("seed workspace member failed: %v", err)
	}
	wsMemberRole, wsMemberMember, err := tenancyStore.GetWorkspaceRole(ctx, ws.PublicID, postgresStoreMember)
	if err != nil || !wsMemberMember {
		t.Fatalf("seeded member workspace role = %q member=%v err=%v", wsMemberRole, wsMemberMember, err)
	}
	wsExtraOwner := postgresStoreExtra
	if _, err := tenancyStore.AddWorkspaceMember(ctx, ws.PublicID, wsExtraOwner, tenancy.RoleOwner); err != nil {
		t.Fatalf("seed workspace owner failed: %v", err)
	}
	wsExtraRole, wsExtraMember, err := tenancyStore.GetWorkspaceRole(ctx, ws.PublicID, wsExtraOwner)
	if err != nil || !wsExtraMember {
		t.Fatalf("seeded owner workspace role = %q member=%v err=%v", wsExtraRole, wsExtraMember, err)
	}

	if _, ok, err := store.GetBoardRole(ctx, ws.PublicID, boardID, postgresStoreOutsider, ""); err != nil || ok {
		t.Fatalf("outsider role = %v err %v, want not-ok", ok, err)
	}
	if role, ok, err := store.GetBoardRole(ctx, ws.PublicID, boardID, postgresStoreOwner, wsOwnerRole); err != nil || !ok || role != RoleOwner {
		t.Fatalf("owner role = %v %v err %v, want owner", role, ok, err)
	}
	if role, ok, err := store.GetBoardRole(ctx, ws.PublicID, boardID, postgresStoreMember, wsMemberRole); err != nil || !ok || role != RoleViewer {
		t.Fatalf("member default role = %v %v err %v, want viewer", role, ok, err)
	}
	if role, ok, err := store.GetBoardRole(ctx, ws.PublicID, boardID, wsExtraOwner, wsExtraRole); err != nil || !ok || role != RoleOwner {
		t.Fatalf("workspace owner default role = %v %v err %v, want owner", role, ok, err)
	}

	if _, err := store.AddBoardMember(ctx, ws.PublicID, boardID, postgresStoreOutsider, postgresStoreMember, RoleViewer, ""); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("outsider grant err = %v, want ErrPermissionDenied", err)
	}
	added, err := store.AddBoardMember(ctx, ws.PublicID, boardID, postgresStoreOwner, postgresStoreMember, RoleViewer, wsOwnerRole)
	if err != nil || added.Role != RoleViewer {
		t.Fatalf("add member err = %v role = %v", err, added)
	}
	// Re-adding keeps the existing role (idempotent).
	replayed, err := store.AddBoardMember(ctx, ws.PublicID, boardID, postgresStoreOwner, postgresStoreMember, RoleEditor, wsOwnerRole)
	if err != nil || replayed.Role != RoleViewer {
		t.Fatalf("re-add err = %v role = %v, want viewer", err, replayed)
	}
	// Explicit rows override the workspace default.
	if role, ok, err := store.GetBoardRole(ctx, ws.PublicID, boardID, postgresStoreMember, wsMemberRole); err != nil || !ok || role != RoleViewer {
		t.Fatalf("explicit override role = %v %v err %v, want viewer", role, ok, err)
	}
	if _, err := store.AddBoardMember(ctx, ws.PublicID, boardID, postgresStoreOwner, postgresStoreOwner, RoleViewer, wsOwnerRole); !errors.Is(err, ErrMemberInviteSelf) {
		t.Fatalf("add owner err = %v, want ErrMemberInviteSelf", err)
	}

	members, err := store.ListBoardMembers(ctx, ws.PublicID, boardID)
	if err != nil || len(members) != 1 || members[0].UserID != postgresStoreMember {
		t.Fatalf("members = %+v err %v, want 1 viewer", members, err)
	}

	promoted, err := store.UpdateBoardMemberRole(ctx, ws.PublicID, boardID, postgresStoreOwner, postgresStoreMember, RoleEditor, wsOwnerRole)
	if err != nil || promoted.Role != RoleEditor {
		t.Fatalf("promote err = %v", err)
	}
	if err := store.RemoveBoardMember(ctx, ws.PublicID, boardID, postgresStoreMember, postgresStoreMember, wsMemberRole); err != nil {
		t.Fatalf("self-remove failed: %v", err)
	}

	// Sole owner cannot be demoted or removed.
	if _, err := store.UpdateBoardMemberRole(ctx, ws.PublicID, boardID, postgresStoreOwner, postgresStoreOwner, RoleEditor, wsOwnerRole); !errors.Is(err, ErrV1LastOwner) {
		t.Fatalf("demote sole owner err = %v, want ErrV1LastOwner", err)
	}
	if err := store.RemoveBoardMember(ctx, ws.PublicID, boardID, postgresStoreOwner, postgresStoreOwner, wsOwnerRole); !errors.Is(err, ErrMemberInviteSelf) {
		t.Fatalf("remove owner err = %v, want ErrMemberInviteSelf", err)
	}
}

func TestPostgresWorkspaceBoardStoreShareTokens(t *testing.T) {
	ctx, store, ws := newPostgresWorkspaceBoardFixture(t)
	boardID := mustCreatePostgresBoard(t, ctx, store, ws, "Shares")

	tokenID, err := NewV1ShareTokenID()
	if err != nil {
		t.Fatalf("mint token id failed: %v", err)
	}
	created, err := store.CreateShareToken(ctx, ws.PublicID, boardID, postgresStoreOwner, RoleViewer, tokenID, V1ShareTokenHash(tokenID), tokenID)
	if err != nil {
		t.Fatalf("create share failed: %v", err)
	}
	if created.Bearer == "" || created.TokenPrefix == "" {
		t.Fatal("created share must carry the bearer once with a prefix")
	}
	if !contract.IsValidPublicID("sht_", created.TokenID) {
		t.Fatalf("token id = %q, want sht_ shape", created.TokenID)
	}

	tokens, err := store.ListShareTokens(ctx, ws.PublicID, boardID)
	if err != nil || len(tokens) != 1 {
		t.Fatalf("list shares = %d err %v, want 1", len(tokens), err)
	}
	if tokens[0].Bearer != "" {
		t.Fatal("listed share must not carry the bearer")
	}
	if tokens[0].UsageCount != 0 {
		t.Fatalf("usage = %d, want 0", tokens[0].UsageCount)
	}

	joined, role, err := store.JoinByToken(ctx, ws.PublicID, tokenID, postgresStoreMember)
	if err != nil || role != RoleViewer || joined.PublicID != boardID {
		t.Fatalf("join err = %v role = %v", err, role)
	}
	after, err := store.ListShareTokens(ctx, ws.PublicID, boardID)
	if err != nil || after[0].UsageCount != 1 {
		t.Fatalf("usage after join = %+v err %v, want 1", after, err)
	}
	// Repeat joins converge idempotently.
	if _, _, err := store.JoinByToken(ctx, ws.PublicID, tokenID, postgresStoreMember); err != nil {
		t.Fatalf("rejoin failed: %v", err)
	}

	if err := store.RevokeShareToken(ctx, ws.PublicID, boardID, tokenID, postgresStoreOwner); err != nil {
		t.Fatalf("revoke failed: %v", err)
	}
	// Revoking twice succeeds (idempotent).
	if err := store.RevokeShareToken(ctx, ws.PublicID, boardID, tokenID, postgresStoreOwner); err != nil {
		t.Fatalf("second revoke failed: %v", err)
	}
	if _, _, err := store.JoinByToken(ctx, ws.PublicID, tokenID, postgresStoreOutsider); !errors.Is(err, ErrShareTokenRevoked) {
		t.Fatalf("join revoked err = %v, want ErrShareTokenRevoked", err)
	}
	if _, _, err := store.JoinByToken(ctx, ws.PublicID, "sht_bogusbogusbogus00", postgresStoreOutsider); !errors.Is(err, ErrShareTokenNotFound) {
		t.Fatalf("join unknown err = %v, want ErrShareTokenNotFound", err)
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

	tokenID, err := NewV1ShareTokenID()
	if err != nil {
		t.Fatalf("mint token id failed: %v", err)
	}
	if _, err := store.CreateShareToken(ctx, ws.PublicID, boardID, postgresStoreOwner, RoleViewer, tokenID, V1ShareTokenHash(tokenID), tokenID); err != nil {
		t.Fatalf("create share failed: %v", err)
	}
	if _, _, err := store.JoinByToken(ctx, other.PublicID, tokenID, postgresStoreExtra); !errors.Is(err, ErrShareTokenNotFound) {
		t.Fatalf("foreign join err = %v, want ErrShareTokenNotFound", err)
	}
}
