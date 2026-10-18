package workspacemedia

import (
	"context"
	"errors"
	"ideaboard/api/internal/boards"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/tenancy"
	"testing"
	"time"
)

func TestValidateDocumentAssetsPreconditions(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := ValidateDocumentAssets(ctx, nil, "ws_test", 1, nil, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled validation = %v", err)
	}

	if err := ValidateDocumentAssets(context.Background(), nil, "ws_test", 1, nil, nil); !errors.Is(err, errNotConfigured) {
		t.Fatalf("unconfigured validation = %v", err)
	}
}

func TestValidateDocumentAssetsPostgres(t *testing.T) {
	env, ctx, pool := openAccessPGEnv(t)
	ownerAsset := pgUploadComplete(t, ctx, env.media, env.ws, env.owner, "")
	memberAsset := pgUploadComplete(t, ctx, env.media, env.ws, env.member, "")
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })
	q := dbsqlc.New(tx)
	access := &stubBoardAccess{}
	if err := ValidateDocumentAssets(ctx, q, env.ws, env.member, access, []string{memberAsset.ID, ownerAsset.ID}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("forged private asset mixed with owned asset = %v, want ErrNotFound", err)
	}
	if err := ValidateDocumentAssets(ctx, q, env.ws, env.owner, access, []string{"asset_missing00000001"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing asset = %v, want ErrNotFound", err)
	}
	if err := ValidateDocumentAssets(ctx, q, "ws_other", env.owner, access, []string{ownerAsset.ID}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign-workspace asset = %v, want ErrNotFound", err)
	}
	if err := ValidateDocumentAssets(ctx, q, env.ws, env.member, access, []string{memberAsset.ID}); err != nil {
		t.Fatalf("owned asset = %v", err)
	}
	const sourceBoard = "board_source00000001"
	access.roles = map[string]boards.AccessRole{sourceBoard: boards.RoleViewer}
	access.refs = map[string][]string{ownerAsset.ID: {sourceBoard}}
	if err := ValidateDocumentAssets(ctx, q, env.ws, env.member, access, []string{ownerAsset.ID}); err != nil {
		t.Fatalf("readable existing reference = %v", err)
	}
	delete(access.roles, sourceBoard)
	if err := ValidateDocumentAssets(ctx, q, env.ws, env.member, access, []string{ownerAsset.ID}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("revoked existing reference = %v, want ErrNotFound", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE media_assets SET owner_user_id = $1 WHERE public_id = $2`, env.member, ownerAsset.ID); err != nil {
		t.Fatal(err)
	}
	if err := ValidateDocumentAssets(ctx, q, env.ws, env.member, access, []string{ownerAsset.ID}); err != nil {
		t.Fatalf("uncommitted ownership from accepting transaction = %v", err)
	}
}

// merged from document_acceptance_postgres_test.go (document_access.go aggregation: acceptance revocation while waiting).
func TestDocumentAcceptanceRejectsSourceRevokedWhileWaiting(t *testing.T) {
	env, _, pool := openAccessPGEnv(t)
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	asset := pgUploadComplete(t, ctx, env.media, env.ws, env.owner, "")
	createBoard := func(owner int64, name string) string {
		t.Helper()
		id, err := boards.NewV1BoardPublicID()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := env.boards.CreateBoard(ctx, env.ws, owner, name, id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	source := createBoard(env.owner, "Source")
	destination := createBoard(env.member, "Destination")
	if _, err := env.boards.ReplaceDocument(ctx, env.ws, source, env.owner, 1, 1, pgDocEnvelope(t, asset.ID)); err != nil {
		t.Fatal(err)
	}
	if _, err := env.boards.ReplaceBoardAccess(ctx, env.ws, source, env.owner, 1, boards.WorkspaceAccess(boards.RoleViewer), []int64{env.member}); err != nil {
		t.Fatal(err)
	}
	if _, err := env.media.AuthorizeAsset(ctx, env.ws, asset.ID, env.member, tenancy.RoleMember, env.boards); err != nil {
		t.Fatalf("preflight must allow: %v", err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	q := dbsqlc.New(tx)
	if _, err := q.LockWorkspaceMemberships(ctx, env.ws); err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() {
		_, err := env.boards.ReplaceDocument(ctx, env.ws, destination, env.member, 1, 1, pgDocEnvelope(t, asset.ID))
		result <- err
	}()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		var waiting bool
		if _, err := tx.Exec(ctx, `SELECT pg_stat_clear_snapshot()`); err != nil {
			t.Fatal(err)
		}
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_stat_activity WHERE pg_backend_pid() = ANY(pg_blocking_pids(pid)))`).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			break
		}
		select {
		case <-ticker.C:
		case <-ctx.Done():
			t.Fatal("document acceptance did not wait for Workspace lock")
		}
	}
	if _, err := q.DeleteWorkspaceBoardMembership(ctx, dbsqlc.DeleteWorkspaceBoardMembershipParams{WorkspacePublicID: env.ws, BoardPublicID: source, UserID: env.member}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-result; !errors.Is(err, ErrNotFound) {
		t.Fatalf("accepted a revoked source: %v", err)
	}
	board, err := env.boards.GetBoard(ctx, env.ws, destination)
	if err != nil {
		t.Fatal(err)
	}
	if board.Revision != 1 {
		t.Fatalf("denied acceptance changed revision to %d", board.Revision)
	}
	refs, err := env.boards.ListBoardsReferencingAsset(ctx, env.ws, asset.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 1 || refs[0] != source {
		t.Fatalf("denied acceptance changed references: %v", refs)
	}
}
