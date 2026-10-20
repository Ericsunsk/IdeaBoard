package http

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"ideaboard/api/internal/boards"
	boardrealtime "ideaboard/api/internal/boards/realtime"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	identitypostgres "ideaboard/api/internal/identity/users/infrastructure/postgres"
	"ideaboard/api/internal/tenancy"
	tenancypg "ideaboard/api/internal/tenancy/infrastructure/postgres"
	"ideaboard/api/internal/testdb"
)

// TestInternalRealtimeAdmitPostgresSuccess proves the admit success path
// against real PostgreSQL stores: ticket issuance (boards/realtime.Issue)
// verifies and admits through NewInternalRealtimeHandler wired to
// PostgresWorkspaceBoardStore + tenancy Store + the production user
// directory. Skips without API_TEST_DATABASE_DSN like every other
// PostgreSQL integration test (testdb.Open handles the skip).
func TestInternalRealtimeAdmitPostgresSuccess(t *testing.T) {
	db := testdb.Open(t)
	ctx := context.Background()

	const owner, viewer = int64(1), int64(3)

	tenancyStore := tenancypg.NewStore(db, nil)
	ws, err := tenancyStore.CreateWorkspace(ctx, tenancy.CreateWorkspaceParams{
		Name:        "realtime-pg",
		OwnerUserID: owner,
	})
	if err != nil {
		t.Fatalf("create workspace failed: %v", err)
	}
	if _, err := tenancyStore.AddWorkspaceMember(ctx, ws.PublicID, viewer, tenancy.RoleMember); err != nil {
		t.Fatalf("add workspace member failed: %v", err)
	}

	allowAssets := func(context.Context, *dbsqlc.Queries, string, int64, boards.DocumentMediaAccess, []string) error {
		return nil
	}
	boardStore := boards.NewPostgresWorkspaceBoardStore(db, allowAssets)
	boardID, err := boards.NewV1BoardPublicID()
	if err != nil {
		t.Fatalf("mint board id failed: %v", err)
	}
	if _, err := boardStore.CreateBoard(ctx, ws.PublicID, owner, "Presence", boardID); err != nil {
		t.Fatalf("create board failed: %v", err)
	}
	access, err := boardStore.GetBoardAccess(ctx, ws.PublicID, boardID)
	if err != nil {
		t.Fatalf("get board access failed: %v", err)
	}
	if _, err := boardStore.ReplaceBoardAccess(ctx, ws.PublicID, boardID, owner, access.AccessVersion, boards.WorkspaceAccessEditor, []int64{viewer}); err != nil {
		t.Fatalf("grant board access failed: %v", err)
	}
	board, err := boardStore.GetBoardByPublicID(ctx, boardID)
	if err != nil {
		t.Fatalf("get board failed: %v", err)
	}

	userRepo := identitypostgres.NewRepository(db)
	viewerRow, err := userRepo.GetByID(ctx, viewer)
	if err != nil {
		t.Fatalf("get viewer failed: %v", err)
	}
	viewerUID := tenancy.FormatUserPublicID(viewerRow.PublicID)

	ticket, _, err := boardrealtime.Issue(admitTestTicketSecret, boardID, board.AccessVersion, boardrealtime.CapRead, viewerUID, time.Now())
	if err != nil {
		t.Fatalf("issue ticket failed: %v", err)
	}

	handler := NewInternalRealtimeHandler(boardStore, tenancyStore, NewUsersRepositoryDirectory(userRepo), admitTestTicketSecret, admitTestInternalKey, nil)
	code, body := serveAdmit(t, handler, admitTestInternalKey, "?ticket="+ticket)
	if code != http.StatusOK || !strings.Contains(body, `"admit":true`) {
		t.Fatalf("pg admit = %d %s, want 200 admit:true", code, body)
	}
}
