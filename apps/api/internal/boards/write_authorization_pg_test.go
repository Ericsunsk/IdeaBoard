package boards

import (
	"context"
	"errors"
	"testing"
	"time"

	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/tenancy"
	tenancypg "ideaboard/api/internal/tenancy/postgres"
)

func TestWriteAuthorizationPostgres(t *testing.T) {
	ctx, store, ws := newPostgresWorkspaceBoardFixture(t)
	if _, err := tenancypg.NewStore(store.db).AddWorkspaceMember(ctx, ws.PublicID, matrixEditor, tenancy.RoleMember); err != nil {
		t.Fatal(err)
	}
	exerciseWriteAuthorization(t, store, ws.PublicID)
}

func TestCatalogRoleMergePostgres(t *testing.T) {
	_, store, ws := newPostgresWorkspaceBoardFixture(t)
	exerciseCatalogRoleMerge(t, store, ws.PublicID)
}

func TestDocumentWriteRechecksAccessAfterBoardLock(t *testing.T) {
	_, store, ws := newPostgresWorkspaceBoardFixture(t)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	if _, err := tenancypg.NewStore(store.db).AddWorkspaceMember(ctx, ws.PublicID, matrixEditor, tenancy.RoleMember); err != nil {
		t.Fatal(err)
	}
	id := mustMatrixBoard(t, ctx, store, ws.PublicID, "Concurrent authorization", matrixCreator)
	if _, err := store.ReplaceBoardAccess(ctx, ws.PublicID, id, matrixCreator, 1, WorkspaceAccess(RoleEditor), []int64{matrixEditor}); err != nil {
		t.Fatal(err)
	}
	tx, err := store.db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	queries := dbsqlc.New(tx)
	if _, err := queries.LockWorkspaceBoardAccess(ctx, dbsqlc.LockWorkspaceBoardAccessParams{
		WorkspacePublicID: ws.PublicID, BoardPublicID: id,
	}); err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() {
		_, err := store.ReplaceDocument(ctx, ws.PublicID, id, matrixEditor, 1, 1, V1InitialDocument())
		result <- err
	}()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		var waiting bool
		if _, err := tx.Exec(ctx, `SELECT pg_stat_clear_snapshot()`); err != nil {
			t.Fatal(err)
		}
		if err := tx.QueryRow(ctx, `SELECT EXISTS (
			SELECT 1 FROM pg_stat_activity WHERE pg_backend_pid() = ANY(pg_blocking_pids(pid))
		)`).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			break
		}
		select {
		case <-ticker.C:
		case <-ctx.Done():
			t.Fatal("document writer did not wait for the board lock")
		}
	}
	if _, err := queries.SetBoardWorkspaceAccess(ctx, dbsqlc.SetBoardWorkspaceAccessParams{
		WorkspacePublicID: ws.PublicID, BoardPublicID: id, ExpectedVersion: 2, WorkspaceAccess: string(WorkspaceAccessViewer),
	}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-result; !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("blocked writer error = %v, want ErrPermissionDenied", err)
	}
	board, err := store.GetBoard(ctx, ws.PublicID, id)
	if err != nil {
		t.Fatal(err)
	}
	if board.Revision != 1 {
		t.Fatalf("revision = %d, want 1", board.Revision)
	}
}
