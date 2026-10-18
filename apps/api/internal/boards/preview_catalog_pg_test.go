package boards

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"ideaboard/api/internal/boards/previewscene"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/tenancy"
	tenancypg "ideaboard/api/internal/tenancy/postgres"
	"ideaboard/api/internal/testdb"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func newPostgresPreviewFixture(t *testing.T) (context.Context, *pgxpool.Pool, *PostgresWorkspaceBoardStore, *tenancy.Workspace) {
	t.Helper()
	db := testdb.Open(t)
	ctx := context.Background()
	tenancyStore := tenancypg.NewStore(db)
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

	dry, err := BackfillBoardPreviews(ctx, dbsqlc.New(db), 200, 0, true)
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

	stats, err := BackfillBoardPreviews(ctx, dbsqlc.New(db), 200, 0, false)
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

	again, err := BackfillBoardPreviews(ctx, dbsqlc.New(db), 200, 0, false)
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

	stats, err := BackfillBoardPreviews(ctx, dbsqlc.New(db), 200, 0, false)
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

	other, err := tenancypg.NewStore(db).CreateWorkspace(ctx, tenancy.CreateWorkspaceParams{
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
