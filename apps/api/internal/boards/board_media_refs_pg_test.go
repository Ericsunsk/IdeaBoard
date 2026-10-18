package boards

// PostgreSQL parity for board document -> media reference reconciliation
// (issue #936): the same accept/remove matrix runs unchanged on the
// postgres store, including the fail-closed join (document ids without a
// workspace-scoped asset row reconcile no reference). Skips without
// API_TEST_DATABASE_DSN like every other PG integration test.

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"ideaboard/api/internal/tenancy"
	tenancypg "ideaboard/api/internal/tenancy/postgres"
	"ideaboard/api/internal/testdb"

	"github.com/jackc/pgx/v5/pgxpool"
)

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
	ws, err := tenancypg.NewStore(db).CreateWorkspace(ctx, tenancy.CreateWorkspaceParams{
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
