package boards

import (
	"testing"

	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/tenancy"
	tenancypg "ideaboard/api/internal/tenancy/postgres"

	"github.com/google/uuid"
)

func TestMembershipDepartureNotifiesCreatorAndSelectedBoardsIncludingArchived(t *testing.T) {
	ctx, store, ws := newPostgresWorkspaceBoardFixture(t)
	if err := store.q.UpsertWorkspaceMembership(ctx, dbsqlc.UpsertWorkspaceMembershipParams{WorkspaceID: ws.InternalID, UserID: postgresStoreMember, Role: tenancy.RoleOwner}); err != nil {
		t.Fatal(err)
	}
	live := mustCreatePostgresBoard(t, ctx, store, ws, "Creator live")
	archived := mustCreatePostgresBoard(t, ctx, store, ws, "Creator archived")
	if _, err := store.ArchiveBoard(ctx, ws.PublicID, archived, postgresStoreOwner); err != nil {
		t.Fatal(err)
	}
	selected := mustMatrixBoard(t, ctx, store, ws.PublicID, "Selected Board", postgresStoreMember)
	if _, err := store.ReplaceBoardAccess(ctx, ws.PublicID, selected, postgresStoreMember, 1, WorkspaceAccessViewer, []int64{postgresStoreOwner}); err != nil {
		t.Fatal(err)
	}
	user, err := store.q.GetUserByID(ctx, postgresStoreOwner)
	if err != nil {
		t.Fatal(err)
	}
	recipient := tenancy.FormatUserPublicID(uuid.UUID(user.PublicID.Bytes))
	cleaned, err := tenancypg.NewStore(store.db).RemoveWorkspaceMember(ctx, ws.PublicID, postgresStoreMember, postgresStoreOwner, store)
	if err != nil || len(cleaned) != 3 {
		t.Fatalf("departure=%+v %v", cleaned, err)
	}
	versions := map[string]int64{}
	for _, entry := range cleaned {
		versions[entry.BoardPublicID] = entry.AccessVersion
	}
	if versions[live] != 2 || versions[archived] != 2 || versions[selected] != 3 {
		t.Fatalf("affected versions=%v", versions)
	}
	for _, id := range []string{live, archived, selected} {
		var count int
		if err := store.db.QueryRow(ctx, `SELECT count(*) FROM board_access_event_outbox WHERE board_id=$1 AND member_public_id=$2 AND access_version=$3`, id, recipient, versions[id]).Scan(&count); err != nil || count != 1 {
			t.Fatalf("directed outbox %s=%d %v", id, count, err)
		}
		if _, visible, err := store.GetBoardRole(ctx, ws.PublicID, id, postgresStoreOwner, tenancy.RoleOwner); err != nil || visible {
			t.Fatalf("departed creator/member visible on %s: %v %v", id, visible, err)
		}
	}
	creator, err := store.q.GetUserByID(ctx, postgresStoreMember)
	if err != nil {
		t.Fatal(err)
	}
	var creatorSignals int
	if err := store.db.QueryRow(ctx, `SELECT count(*) FROM board_access_event_outbox WHERE board_id=$1 AND member_public_id=$2 AND access_version=3`, selected, tenancy.FormatUserPublicID(uuid.UUID(creator.PublicID.Bytes))).Scan(&creatorSignals); err != nil || creatorSignals != 1 {
		t.Fatalf("retained creator roster convergence=%d %v", creatorSignals, err)
	}

}
