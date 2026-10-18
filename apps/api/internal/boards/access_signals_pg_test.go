package boards

import (
	"testing"

	"ideaboard/api/internal/events"
	tenancypg "ideaboard/api/internal/tenancy/postgres"
)

func TestBoardAccessMutationsReturnCommittedSignalRecords(t *testing.T) {
	ctx, store, workspace := newPostgresWorkspaceBoardFixture(t)
	boardID := mustCreatePostgresBoard(t, ctx, store, workspace, "Signals")
	updated, err := store.ReplaceBoardAccess(ctx, workspace.PublicID, boardID, postgresStoreOwner, 1, WorkspaceAccessViewer, []int64{postgresStoreMember})
	if err != nil {
		t.Fatal(err)
	}
	if len(updated.AccessSignals) != 2 {
		t.Fatalf("signals=%d, want Creator and selected member", len(updated.AccessSignals))
	}
	assertStored := func(records []events.BoardAccessOutboxRecord) {
		t.Helper()
		for _, record := range records {
			var stored events.BoardAccessOutboxRecord
			err := store.db.QueryRow(ctx, `SELECT event_id, workspace_id, board_id, member_public_id, access_version, occurred_at FROM board_access_event_outbox WHERE event_id=$1`, record.EventID).Scan(&stored.EventID, &stored.WorkspaceID, &stored.BoardID, &stored.MemberPublicID, &stored.AccessVersion, &stored.OccurredAt)
			if err != nil {
				t.Fatal(err)
			}
			if record.EventID != stored.EventID || record.WorkspaceID != stored.WorkspaceID || record.BoardID != stored.BoardID || record.MemberPublicID != stored.MemberPublicID || record.AccessVersion != stored.AccessVersion || !record.OccurredAt.Equal(stored.OccurredAt) {
				t.Fatalf("returned signal differs from transaction-committed fact: %+v / %+v", record, stored)
			}
		}
	}
	assertStored(updated.AccessSignals)
	same, err := store.ReplaceBoardAccess(ctx, workspace.PublicID, boardID, postgresStoreOwner, updated.AccessVersion, WorkspaceAccessViewer, []int64{postgresStoreMember})
	if err != nil || len(same.AccessSignals) != 0 {
		t.Fatalf("no-op signals=%+v err=%v", same, err)
	}
	cleaned, err := tenancypg.NewStore(store.db).RemoveWorkspaceMember(ctx, workspace.PublicID, postgresStoreOwner, postgresStoreMember, store)
	if err != nil || len(cleaned) != 1 || len(cleaned[0].AccessSignals) != 2 {
		t.Fatalf("departure=%+v err=%v", cleaned, err)
	}
	assertStored(cleaned[0].AccessSignals)
	if _, member, err := tenancypg.NewStore(store.db).GetWorkspaceRole(ctx, workspace.PublicID, postgresStoreMember); err != nil || member {
		t.Fatalf("membership departure did not commit: member=%v err=%v", member, err)
	}
}
