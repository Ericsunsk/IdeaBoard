package workspacemedia

import (
	"context"
	"errors"
	"testing"
	"time"

	"ideaboard/api/internal/boards"
	dbsqlc "ideaboard/api/internal/db/sqlc"
)

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
	if _, err := env.media.AuthorizeAsset(ctx, env.ws, asset.ID, env.member, "member", env.boards); err != nil {
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
