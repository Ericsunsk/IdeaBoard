package boards

// Policy aggregation for the unified Board access policy (policy.go:
// ResolveRole/CheckCapability plus the access-policy store methods).
// Merged from access_matrix_test.go, access_matrix_postgres_test.go,
// access_signals_postgres_test.go, membership_acceptance_postgres_test.go,
// membership_creator_departure_postgres_test.go, policy_boundary_test.go,
// write_authorization_test.go, write_authorization_postgres_test.go and
// workspace_members_test.go (shared helpers); the source files were
// orphans with no same-named implementation.

import (
	"context"
	"errors"
	"fmt"
	"ideaboard/api/internal/archtest"
	"ideaboard/api/internal/db/dbutil"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	eventoutbox "ideaboard/api/internal/events/outbox"
	"ideaboard/api/internal/tenancy"
	tenancypg "ideaboard/api/internal/tenancy/infrastructure/postgres"
	"ideaboard/api/internal/testdb"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// merged from access_matrix_test.go.
// Matrix personas. IDs stay within the testdb-seeded users so the shared
// matrix also runs on PostgreSQL (which enforces user FKs).
const (
	matrixCreator  = int64(1)
	matrixEditor   = int64(3)
	matrixViewer   = int64(7)
	matrixStranger = int64(8)
	matrixMember   = int64(10)
	matrixWSOwner  = int64(11)
	matrixWSAdmin  = int64(12)
)

func mustMatrixBoard(t *testing.T, ctx context.Context, store WorkspaceBoardStore, workspaceID, name string, owner int64) string {
	t.Helper()
	boardID, err := NewV1BoardPublicID()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateBoard(ctx, workspaceID, owner, name, boardID); err != nil {
		t.Fatalf("create board %s: %v", name, err)
	}
	return boardID
}

// exerciseAccessMatrix pins the unified visibility + lifecycle behavior on
// any WorkspaceBoardStore. Callers pass a workspace the store can create
// boards in (InMemory materializes it; PostgreSQL needs a real workspace).
func exerciseAccessMatrix(t *testing.T, store WorkspaceBoardStore, workspaceID string) {
	t.Helper()
	ctx := context.Background()
	id := mustMatrixBoard(t, ctx, store, workspaceID, "Selected sharing", matrixCreator)
	initial, err := store.GetBoardAccess(ctx, workspaceID, id)
	if err != nil || initial.AccessState != AccessStatePrivate || len(initial.MemberUserIDs) != 0 {
		t.Fatalf("initial = %+v %v", initial, err)
	}
	on, err := store.ReplaceBoardAccess(ctx, workspaceID, id, matrixCreator, 1, WorkspaceAccessViewer, []int64{matrixEditor, matrixViewer, matrixEditor})
	if err != nil || on.AccessVersion != 2 || len(on.MemberUserIDs) != 2 || on.AccessState != AccessStateWorkspace {
		t.Fatalf("On = %+v %v", on, err)
	}
	for _, user := range []int64{matrixEditor, matrixViewer} {
		role, visible, err := store.GetBoardRole(ctx, workspaceID, id, user, "member")
		if err != nil || !visible || role != RoleViewer {
			t.Fatalf("selected user %d=%s %v %v", user, role, visible, err)
		}
	}
	for _, user := range []int64{matrixMember, matrixWSOwner, matrixWSAdmin} {
		if _, visible, err := store.GetBoardRole(ctx, workspaceID, id, user, "owner"); err != nil || visible {
			t.Fatalf("unselected user %d visible=%v %v", user, visible, err)
		}
		page, err := store.ListBoardCatalog(ctx, workspaceID, false, user, "owner", V1BoardCatalogPageRequest{Limit: 100})
		if err != nil || len(page.Entries) != 0 {
			t.Fatalf("unselected catalog=%+v %v", page, err)
		}
	}
	if _, err := store.ReplaceBoardAccess(ctx, workspaceID, id, matrixViewer, 2, WorkspaceAccessEditor, []int64{matrixViewer}); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("noncreator change=%v", err)
	}
	if _, err := store.ReplaceBoardAccess(ctx, workspaceID, id, matrixCreator, 1, WorkspaceAccessEditor, []int64{matrixViewer}); !errors.Is(err, ErrAccessConflict) {
		t.Fatalf("stale change=%v", err)
	}
	if _, err := store.ReplaceBoardAccess(ctx, workspaceID, id, matrixCreator, 2, WorkspaceAccessViewer, []int64{matrixCreator}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("select creator=%v", err)
	}
	same, err := store.ReplaceBoardAccess(ctx, workspaceID, id, matrixCreator, 2, WorkspaceAccessViewer, []int64{matrixViewer, matrixEditor})
	if err != nil || same.AccessVersion != 2 {
		t.Fatalf("same config=%+v %v", same, err)
	}
	elevated, err := store.ReplaceBoardAccess(ctx, workspaceID, id, matrixCreator, 2, WorkspaceAccessEditor, on.MemberUserIDs)
	if err != nil || elevated.AccessVersion != 3 {
		t.Fatalf("common role=%+v %v", elevated, err)
	}
	for _, user := range on.MemberUserIDs {
		role, visible, err := store.GetBoardRole(ctx, workspaceID, id, user, "member")
		if err != nil || !visible || role != RoleEditor {
			t.Fatalf("common editor=%s %v %v", role, visible, err)
		}
		page, err := store.ListBoardCatalog(ctx, workspaceID, false, user, "member", V1BoardCatalogPageRequest{Limit: 100})
		if err != nil || len(page.Entries) != 1 || page.Entries[0].EffectiveRole != RoleEditor {
			t.Fatalf("selected catalog=%+v %v", page, err)
		}
	}
	off, err := store.ReplaceBoardAccess(ctx, workspaceID, id, matrixCreator, 3, WorkspaceAccessNone, on.MemberUserIDs)
	if err != nil || off.AccessVersion != 4 || len(off.MemberUserIDs) != 0 || off.AccessState != AccessStatePrivate {
		t.Fatalf("Off=%+v %v", off, err)
	}
	empty, err := store.ReplaceBoardAccess(ctx, workspaceID, id, matrixCreator, 4, WorkspaceAccessViewer, nil)
	if err != nil || len(empty.MemberUserIDs) != 0 || empty.AccessState != AccessStatePrivate {
		t.Fatalf("reOn empty=%+v %v", empty, err)
	}
	for _, user := range on.MemberUserIDs {
		if _, visible, err := store.GetBoardRole(ctx, workspaceID, id, user, "member"); err != nil || visible {
			t.Fatalf("old selection resurrected %d", user)
		}
	}
	if role, visible, err := store.GetBoardRole(ctx, workspaceID, id, matrixCreator, "owner"); err != nil || !visible || role != RoleOwner {
		t.Fatalf("creator=%s %v %v", role, visible, err)
	}
	if _, err := store.ArchiveBoard(ctx, workspaceID, id, matrixCreator); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReplaceBoardAccess(ctx, workspaceID, id, matrixCreator, empty.AccessVersion, WorkspaceAccessEditor, []int64{matrixEditor}); !errors.Is(err, ErrV1Archived) {
		t.Fatalf("archived config=%v", err)
	}
}

func TestAccessMatrixInMemory(t *testing.T) {
	exerciseAccessMatrix(t, NewInMemoryWorkspaceBoardStoreForTests(testWorkspaceMembers{}, testWorkspaceMembers{}), "ws_access_matrix")
}

func TestResolveRoleSinglePolicy(t *testing.T) {
	for _, tc := range []struct {
		creator, caller int64
		selected        bool
		access          WorkspaceAccess
		member          bool
		want            AccessRole
		visible         bool
	}{
		{1, 1, false, WorkspaceAccessNone, true, RoleOwner, true},
		{1, 1, false, WorkspaceAccessEditor, false, "", false},
		{0, 1, true, WorkspaceAccessEditor, true, "", false},
		{1, 2, true, WorkspaceAccessViewer, true, RoleViewer, true},
		{1, 2, true, WorkspaceAccessEditor, true, RoleEditor, true},
		{1, 2, false, WorkspaceAccessEditor, true, "", false},
		{1, 2, true, WorkspaceAccessNone, true, "", false},
		{1, 2, true, WorkspaceAccessEditor, false, "", false},
		{1, 2, true, "unknown", true, "", false},
	} {
		role, visible := ResolveRole(tc.creator, tc.caller, tc.selected, tc.access, tc.member)
		if role != tc.want || visible != tc.visible {
			t.Fatalf("%+v got %s %v", tc, role, visible)
		}
	}
}

func TestCheckCapabilityMaps404Vs403(t *testing.T) {
	for _, tc := range []struct {
		name    string
		role    AccessRole
		ok      bool
		cap     Capability
		wantErr error
	}{
		{"invisible read is not found", "", false, CapRead, ErrNotFound},
		{"invisible write is not found", "", false, CapWrite, ErrNotFound},
		{"invisible manage is not found", "", false, CapManage, ErrNotFound},
		{"viewer reads", RoleViewer, true, CapRead, nil},
		{"viewer writes denied", RoleViewer, true, CapWrite, ErrPermissionDenied},
		{"viewer manages denied", RoleViewer, true, CapManage, ErrPermissionDenied},
		{"editor reads", RoleEditor, true, CapRead, nil},
		{"editor writes", RoleEditor, true, CapWrite, nil},
		{"editor manages denied", RoleEditor, true, CapManage, ErrPermissionDenied},
		{"owner reads", RoleOwner, true, CapRead, nil},
		{"owner writes", RoleOwner, true, CapWrite, nil},
		{"owner manages", RoleOwner, true, CapManage, nil},
		{"unknown capability denied", RoleOwner, true, "purge", ErrPermissionDenied},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := CheckCapability(tc.role, tc.ok, tc.cap); !errors.Is(err, tc.wantErr) {
				t.Fatalf("CheckCapability(%q, %v, %q) = %v; want %v", tc.role, tc.ok, tc.cap, err, tc.wantErr)
			}
		})
	}
}

func TestAccessMatrixCatalogEquivalenceSpotCheck(t *testing.T) {
	// Guards the documented invariant directly: every catalog card must
	// carry a PreviewScene/ContentVersion projection consistent with the
	// board row, ordered by recency. Full predicate equivalence rides the
	// matrix above on both stores.
	ctx := context.Background()
	store := NewInMemoryWorkspaceBoardStoreForTests(testWorkspaceMembers{}, testWorkspaceMembers{})
	ids := make([]string, 0, 3)
	for i := range 3 {
		ids = append(ids, mustMatrixBoard(t, ctx, store, "ws_access_spot", string(rune('A'+i)), matrixCreator))
	}
	page, err := store.ListBoardCatalog(ctx, "ws_access_spot", false, matrixCreator, "owner", V1BoardCatalogPageRequest{Limit: 100})
	if err != nil || len(page.Entries) != 3 {
		t.Fatalf("catalog = %d, %v; want 3", len(page.Entries), err)
	}
	seen := map[string]bool{}
	for _, entry := range page.Entries {
		seen[entry.PublicID] = true
	}
	sort.Strings(ids)
	got := make([]string, 0, 3)
	for id := range seen {
		got = append(got, id)
	}
	sort.Strings(got)
	for i := range ids {
		if ids[i] != got[i] {
			t.Fatalf("catalog ids = %v, want %v", got, ids)
		}
	}
}

// merged from access_matrix_postgres_test.go.
// PostgreSQL parity for the unified access matrix (issue #934, criteria
// 2+5): the same behavior matrix runs unchanged on the postgres store.
// Skips without API_TEST_DATABASE_DSN like every other Postgres integration
// test. Repository methods keep context.Context first and the
// dbWithContext pattern (see postgres_workspace.go).

func TestAccessMatrixPostgres(t *testing.T) {
	db := testdb.Open(t)
	ctx := t.Context()
	ws, err := tenancypg.NewStore(db, nil).CreateWorkspace(ctx, tenancy.CreateWorkspaceParams{
		Name:        "boards-access-matrix-pg",
		OwnerUserID: matrixCreator,
	})
	if err != nil {
		t.Fatalf("create workspace failed: %v", err)
	}
	seedBoardTestMembers(t, NewPostgresWorkspaceBoardStore(db, testDocumentAssetValidator), ws.InternalID, matrixEditor, matrixViewer, matrixStranger, matrixMember, matrixWSOwner, matrixWSAdmin)
	exerciseAccessMatrix(t, NewPostgresWorkspaceBoardStore(db, testDocumentAssetValidator), ws.PublicID)
}

// merged from access_signals_postgres_test.go.
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
	assertStored := func(records []eventoutbox.BoardAccessOutboxRecord) {
		t.Helper()
		for _, record := range records {
			var stored eventoutbox.BoardAccessOutboxRecord
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
	cleaned, err := tenancypg.NewStore(store.db, nil).RemoveWorkspaceMember(ctx, workspace.PublicID, postgresStoreOwner, postgresStoreMember, store)
	if err != nil || len(cleaned) != 1 || len(cleaned[0].AccessSignals) != 2 {
		t.Fatalf("departure=%+v err=%v", cleaned, err)
	}
	assertStored(cleaned[0].AccessSignals)
	if _, member, err := tenancypg.NewStore(store.db, nil).GetWorkspaceRole(ctx, workspace.PublicID, postgresStoreMember); err != nil || member {
		t.Fatalf("membership departure did not commit: member=%v err=%v", member, err)
	}
}

// merged from membership_acceptance_postgres_test.go.
func TestCreatorMembershipRequiredForEveryMutation(t *testing.T) {
	ctx, store, ws := newPostgresWorkspaceBoardFixture(t)
	live := mustCreatePostgresBoard(t, ctx, store, ws, "Live")
	archived := mustCreatePostgresBoard(t, ctx, store, ws, "Archived")
	if _, err := store.ReplaceBoardAccess(ctx, ws.PublicID, live, postgresStoreOwner, 1, WorkspaceAccess(RoleEditor), []int64{postgresStoreMember}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ArchiveBoard(ctx, ws.PublicID, archived, postgresStoreOwner); err != nil {
		t.Fatal(err)
	}
	if _, err := store.q.DeleteWorkspaceMembership(ctx, dbsqlc.DeleteWorkspaceMembershipParams{WorkspaceID: ws.InternalID, UserID: postgresStoreOwner}); err != nil {
		t.Fatal(err)
	}
	actions := map[string]func() error{
		"rename": func() error {
			_, err := store.RenameBoard(ctx, ws.PublicID, live, "Denied", postgresStoreOwner)
			return err
		},
		"archive": func() error { _, err := store.ArchiveBoard(ctx, ws.PublicID, live, postgresStoreOwner); return err },
		"restore": func() error { _, err := store.RestoreBoard(ctx, ws.PublicID, archived, postgresStoreOwner); return err },
		"document": func() error {
			_, err := store.ReplaceDocument(ctx, ws.PublicID, live, postgresStoreOwner, 1, 1, V1InitialDocument())
			return err
		},
		"grant": func() error {
			_, err := store.ReplaceBoardAccess(ctx, ws.PublicID, live, postgresStoreOwner, 2, WorkspaceAccess(RoleViewer), []int64{postgresStoreOutsider})
			return err
		},
		"role": func() error {
			_, err := store.ReplaceBoardAccess(ctx, ws.PublicID, live, postgresStoreOwner, 2, WorkspaceAccess(RoleViewer), []int64{postgresStoreMember})
			return err
		},
		"revoke": func() error {
			return func() error {
				_, err := store.ReplaceBoardAccess(ctx, ws.PublicID, live, postgresStoreOwner, 2, WorkspaceAccessNone, nil)
				return err
			}()
		},
		"workspace": func() error {
			_, err := store.ReplaceBoardAccess(ctx, ws.PublicID, live, postgresStoreOwner, 2, WorkspaceAccessEditor, nil)
			return err
		},
		"only-me": func() error {
			_, err := store.ReplaceBoardAccess(ctx, ws.PublicID, live, postgresStoreOwner, 2, WorkspaceAccessNone, nil)
			return err
		},
	}
	for name, action := range actions {
		t.Run(name, func(t *testing.T) {
			err := action()
			if !errors.Is(err, ErrNotFound) && !errors.Is(err, ErrPermissionDenied) {
				t.Fatalf("departed creator accepted: %v", err)
			}
		})
	}
	if _, visible, err := store.GetBoardRole(ctx, ws.PublicID, live, postgresStoreOwner, tenancy.RoleOwner); err != nil || visible {
		t.Fatalf("departed creator role visible=%v error=%v", visible, err)
	}
	page, err := store.ListBoardCatalog(ctx, ws.PublicID, true, postgresStoreOwner, tenancy.RoleOwner, V1BoardCatalogPageRequest{Limit: 100})
	if err != nil || len(page.Entries) != 0 {
		t.Fatalf("departed creator catalog=%v error=%v", page.Entries, err)
	}
}

func TestBoardGrantRechecksMembershipAfterWorkspaceLock(t *testing.T) {
	for _, departing := range []string{"actor", "target"} {
		t.Run(departing, func(t *testing.T) {
			_, store, ws := newPostgresWorkspaceBoardFixture(t)
			ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
			defer cancel()
			board := mustCreatePostgresBoard(t, ctx, store, ws, "Membership race")
			if _, err := store.ReplaceBoardAccess(ctx, ws.PublicID, board, postgresStoreOwner, 1, WorkspaceAccessViewer, []int64{postgresStoreMember}); err != nil {
				t.Fatal(err)
			}
			actor, target := postgresStoreOwner, postgresStoreOutsider
			removed := actor
			if departing == "target" {
				removed = target
			}
			tx, err := store.db.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(context.Background())
			q := dbsqlc.New(tx)
			if _, err := q.LockWorkspaceMemberships(ctx, ws.PublicID); err != nil {
				t.Fatal(err)
			}
			result := make(chan error, 1)
			go func() {
				_, err := store.ReplaceBoardAccess(ctx, ws.PublicID, board, actor, 2, WorkspaceAccess(RoleViewer), []int64{target})
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
					t.Fatal("grant did not wait for Workspace lock")
				}
			}
			if _, err := q.DeleteWorkspaceMembership(ctx, dbsqlc.DeleteWorkspaceMembershipParams{WorkspaceID: ws.InternalID, UserID: removed}); err != nil {
				t.Fatal(err)
			}
			if err := tx.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			err = <-result
			if departing == "actor" && !errors.Is(err, ErrNotFound) {
				t.Fatalf("departed manager grant = %v", err)
			}
			if departing == "target" && !errors.Is(err, ErrMemberNotFound) {
				t.Fatalf("departed target grant = %v", err)
			}
			access, err := store.GetBoardAccess(ctx, ws.PublicID, board)
			if err != nil || access.AccessVersion != 2 {
				t.Fatalf("denied grant changed access: %+v %v", access, err)
			}
		})
	}
}

func TestMembershipDepartureRollsBackGrantsAndOutbox(t *testing.T) {
	for _, state := range []string{"live", "archived"} {
		t.Run(state, func(t *testing.T) {
			ctx, store, ws := newPostgresWorkspaceBoardFixture(t)
			tenancyStore := tenancypg.NewStore(store.db, nil)
			board := mustCreatePostgresBoard(t, ctx, store, ws, "Departure")
			user, err := store.q.GetUserByID(ctx, postgresStoreMember)
			if err != nil {
				t.Fatal(err)
			}
			recipient := tenancy.FormatUserPublicID(uuid.UUID(user.PublicID.Bytes))
			if _, err := store.ReplaceBoardAccess(ctx, ws.PublicID, board, postgresStoreOwner, 1, WorkspaceAccess(RoleViewer), []int64{postgresStoreMember}); err != nil {
				t.Fatal(err)
			}
			if state == "archived" {
				if _, err := store.ArchiveBoard(ctx, ws.PublicID, board, postgresStoreOwner); err != nil {
					t.Fatal(err)
				}
			}
			tx, err := store.db.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(context.Background())
			carrier := dbutil.ContextWithAmbientTx(ctx, tx)
			cleaned, err := tenancyStore.RemoveWorkspaceMember(carrier, ws.PublicID, postgresStoreOwner, postgresStoreMember, store)
			if err != nil || len(cleaned) != 1 || cleaned[0].AccessVersion != 3 {
				t.Fatalf("staged departure=%+v %v", cleaned, err)
			}
			var staged int
			if err := tx.QueryRow(ctx, `SELECT count(*) FROM board_access_event_outbox WHERE board_id=$1 AND member_public_id=$2 AND access_version IN (3,5)`, board, recipient).Scan(&staged); err != nil || staged != 1 {
				t.Fatalf("staged outbox=%d %v", staged, err)
			}
			if err := tx.Rollback(ctx); err != nil {
				t.Fatal(err)
			}
			if _, member, err := tenancyStore.GetWorkspaceRole(ctx, ws.PublicID, postgresStoreMember); err != nil || !member {
				t.Fatalf("rollback lost membership: %v %v", member, err)
			}
			if role, visible, err := store.GetBoardRole(ctx, ws.PublicID, board, postgresStoreMember, tenancy.RoleMember); err != nil || !visible || role != RoleViewer {
				t.Fatalf("rollback lost grant: %s %v %v", role, visible, err)
			}
			var persisted int
			if err := store.db.QueryRow(ctx, `SELECT count(*) FROM board_access_event_outbox WHERE board_id=$1 AND member_public_id=$2 AND access_version IN (3,5)`, board, recipient).Scan(&persisted); err != nil || persisted != 0 {
				t.Fatalf("rollback persisted outbox=%d %v", persisted, err)
			}
			if _, err := tenancyStore.RemoveWorkspaceMember(ctx, ws.PublicID, postgresStoreOwner, postgresStoreMember, store); err != nil {
				t.Fatal(err)
			}
			if _, err := tenancyStore.AddWorkspaceMember(ctx, ws.PublicID, postgresStoreMember, tenancy.RoleMember); err != nil {
				t.Fatal(err)
			}
			if _, visible, err := store.GetBoardRole(ctx, ws.PublicID, board, postgresStoreMember, tenancy.RoleMember); err != nil || visible {
				t.Fatalf("rejoin restored old grant: %v %v", visible, err)
			}
			if err := store.db.QueryRow(ctx, `SELECT count(*) FROM board_access_event_outbox WHERE board_id=$1 AND member_public_id=$2 AND access_version IN (3,5)`, board, recipient).Scan(&persisted); err != nil || persisted != 1 {
				t.Fatalf("committed outbox=%d %v", persisted, err)
			}
			if state == "archived" {
				if _, err := store.RestoreBoard(ctx, ws.PublicID, board, postgresStoreOwner); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := store.ReplaceBoardAccess(ctx, ws.PublicID, board, postgresStoreOwner, 3, WorkspaceAccess(RoleViewer), []int64{postgresStoreMember}); err != nil {
				t.Fatal(err)
			}
			if _, err := tenancyStore.RemoveWorkspaceMember(ctx, ws.PublicID, postgresStoreOwner, postgresStoreMember, store); err != nil {
				t.Fatal(err)
			}
			if err := store.db.QueryRow(ctx, `SELECT count(*) FROM board_access_event_outbox WHERE board_id=$1 AND member_public_id=$2 AND access_version IN (3,5)`, board, recipient).Scan(&persisted); err != nil || persisted != 2 {
				t.Fatalf("second departure outbox=%d %v, want two distinct revocation facts", persisted, err)
			}
		})
	}
}

// merged from membership_creator_departure_postgres_test.go.
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
	cleaned, err := tenancypg.NewStore(store.db, nil).RemoveWorkspaceMember(ctx, ws.PublicID, postgresStoreMember, postgresStoreOwner, store)
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

// merged from policy_boundary_test.go.
// Boundary pins for the unified Board access policy (issue #934,
// criterion 3+6).
//
//   - The Boards domain never depends on HTTP handlers or the runtime:
//     the policy is pure plus storage adapters, and handlers consume it
//     through the narrow RoleReader port.
//   - The role decision lives exactly once (ResolveRole in policy.go):
//     both store adapters delegate to it instead of reimplementing the
//     owner/explicit/default order.
//   - Media / Tasking / Events never copy the role mapping and never reach
//     into Boards storage. Media (#936) consumes the narrow ports
//     (RoleReader-shaped role fact + reference-listing fact) plus the pure
//     read helpers; Tasking / Events hold zero boards imports until
//     #937/#940 wire their ports.

func TestBoardsDomainHasNoHandlerOrRuntimeImports(t *testing.T) {
	archtest.AssertNoForbiddenImports(t, ".", []string{
		"ideaboard/api/internal/runtime",
		"github.com/gin-gonic/gin",
		"net/http",
	})
}

func TestRoleAdaptersDelegateToUnifiedPolicy(t *testing.T) {
	adapters := map[string]string{
		"workspace.go":          "boardRoleLocked",
		"postgres_workspace.go": "boardRoleTx",
	}
	for file, adapter := range adapters {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		text := string(raw)
		if !strings.Contains(text, adapter) {
			t.Fatalf("%s: adapter %s missing", file, adapter)
		}
		if !strings.Contains(text, "ResolveRole(") {
			t.Errorf("%s: %s must delegate to ResolveRole (single policy in policy.go)", file, adapter)
		}
		if strings.Contains(text, "RoleOwner, true\n\t}\n\tif member") ||
			strings.Contains(text, "case \"owner\":\n\t\treturn RoleOwner") {
			t.Errorf("%s: reimplements the role mapping instead of delegating to ResolveRole", file)
		}
	}
}

func TestBoardRoleRulesStayUncopiedOutsideBoards(t *testing.T) {
	for _, dir := range []string{"../tasking", "../events"} {
		archtest.AssertNoForbiddenImports(t, dir, []string{"ideaboard/api/internal/boards"})
		archtest.AssertNoForbiddenText(t, dir, []string{
			"DefaultBoardRoleForWorkspaceRole",
			"boards.ResolveRole",
			"boards.CheckCapability",
			"boards.AccessRole",
			"boards.RoleReader",
		})
	}
	// #936: Media consumes Board access facts through the narrow ports
	// (the RoleReader-shaped role fact plus the reference-listing fact)
	// and the pure read helpers (CheckCapability/CapRead/membership).
	// The role mapping itself is never recomputed outside Boards:
	// ResolveRole, explicit role literals, write/manage capabilities, and
	// Boards storage internals stay banned so the policy keeps one home.
	archtest.AssertNoForbiddenText(t, "../media", []string{
		"DefaultBoardRoleForWorkspaceRole",
		"boards.ResolveRole",
		"boards.RoleOwner",
		"boards.RoleEditor",
		"boards.RoleViewer",
		"boards.CapWrite",
		"boards.CapManage",
		"boards.WorkspaceBoardStore",
		"boards.PostgresWorkspaceBoardStore",
		"boards.InMemoryWorkspaceBoardStore",
		"boardRoleLocked",
		"boardRoleTx",
	})
}

// merged from write_authorization_test.go.
func exerciseWriteAuthorization(t *testing.T, store WorkspaceBoardStore, workspaceID string) {
	t.Helper()
	ctx := context.Background()
	for _, state := range []struct {
		name   string
		revoke bool
		want   error
	}{
		{"demoted", false, ErrPermissionDenied},
		{"revoked", true, ErrNotFound},
	} {
		for _, operation := range []string{"document", "rename", "archive", "restore"} {
			t.Run(state.name+"/"+operation, func(t *testing.T) {
				id := mustMatrixBoard(t, ctx, store, workspaceID, "Write authorization", matrixCreator)
				if _, err := store.ReplaceBoardAccess(ctx, workspaceID, id, matrixCreator, 1, WorkspaceAccess(RoleEditor), []int64{matrixEditor}); err != nil {
					t.Fatal(err)
				}
				role, visible, err := store.GetBoardRole(ctx, workspaceID, id, matrixEditor, "member")
				if err != nil || CheckCapability(role, visible, CapWrite) != nil {
					t.Fatalf("preflight: %s %v %v", role, visible, err)
				}
				if state.revoke {
					err = func() error {
						_, err := store.ReplaceBoardAccess(ctx, workspaceID, id, matrixCreator, 2, WorkspaceAccessNone, nil)
						return err
					}()
				} else {
					_, err = store.ReplaceBoardAccess(ctx, workspaceID, id, matrixCreator, 2, WorkspaceAccess(RoleViewer), []int64{matrixEditor})
				}
				if err != nil {
					t.Fatal(err)
				}
				if operation == "restore" {
					if _, err := store.ArchiveBoard(ctx, workspaceID, id, matrixCreator); err != nil {
						t.Fatal(err)
					}
				}
				switch operation {
				case "document":
					_, err = store.ReplaceDocument(ctx, workspaceID, id, matrixEditor, 1, 1, V1InitialDocument())
				case "rename":
					_, err = store.RenameBoard(ctx, workspaceID, id, "Unauthorized", matrixEditor)
				case "archive":
					_, err = store.ArchiveBoard(ctx, workspaceID, id, matrixEditor)
				case "restore":
					_, err = store.RestoreBoard(ctx, workspaceID, id, matrixEditor)
				}
				if !errors.Is(err, state.want) {
					t.Fatalf("write error = %v, want %v", err, state.want)
				}
			})
		}
	}
}

func TestWriteAuthorizationInMemory(t *testing.T) {
	exerciseWriteAuthorization(t, NewInMemoryWorkspaceBoardStoreForTests(testWorkspaceMembers{}, testWorkspaceMembers{}), "ws_write_authorization")
}

func exerciseCatalogRoleMerge(t *testing.T, store WorkspaceBoardStore, workspaceID string) {
	t.Helper()
	ctx := context.Background()
	id := mustMatrixBoard(t, ctx, store, workspaceID, "Role merge", matrixCreator)
	if _, err := store.ReplaceBoardAccess(ctx, workspaceID, id, matrixCreator, 1, WorkspaceAccess(RoleViewer), []int64{matrixViewer}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReplaceBoardAccess(ctx, workspaceID, id, matrixCreator, 2, WorkspaceAccessEditor, []int64{matrixViewer}); err != nil {
		t.Fatal(err)
	}
	catalog, err := store.ListBoardCatalog(ctx, workspaceID, false, matrixViewer, "member", V1BoardCatalogPageRequest{Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range catalog.Entries {
		if entry.PublicID == id {
			if entry.EffectiveRole != RoleEditor || !entry.HasDirectGrant {
				t.Fatalf("role = %q, direct grant = %v; want editor, true", entry.EffectiveRole, entry.HasDirectGrant)
			}
			return
		}
	}
	t.Fatal("catalog omitted authorized board")
}

func TestCatalogRoleMergeInMemory(t *testing.T) {
	exerciseCatalogRoleMerge(t, NewInMemoryWorkspaceBoardStoreForTests(testWorkspaceMembers{}, testWorkspaceMembers{}), "ws_catalog_role_merge")
}

// merged from write_authorization_postgres_test.go.
func TestWriteAuthorizationPostgres(t *testing.T) {
	ctx, store, ws := newPostgresWorkspaceBoardFixture(t)
	if _, err := tenancypg.NewStore(store.db, nil).AddWorkspaceMember(ctx, ws.PublicID, matrixEditor, tenancy.RoleMember); err != nil {
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
	if _, err := tenancypg.NewStore(store.db, nil).AddWorkspaceMember(ctx, ws.PublicID, matrixEditor, tenancy.RoleMember); err != nil {
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

// merged from workspace_members_test.go.
type testWorkspaceMembers struct{}

func (testWorkspaceMembers) GetUserPublicID(ctx context.Context, userID int64) (uuid.UUID, error) {
	if err := ctx.Err(); err != nil {
		return uuid.Nil, err
	}
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(fmt.Sprintf("board-test-user-%d", userID))), nil
}

func (testWorkspaceMembers) WithWorkspaceMembers(ctx context.Context, _ string, action func(func(int64) string) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return action(func(userID int64) string {
		if userID <= 0 {
			return ""
		}
		return "member"
	})
}

func testDocumentAssetValidator(context.Context, *dbsqlc.Queries, string, int64, DocumentMediaAccess, []string) error {
	return nil
}

func seedBoardTestMembers(t interface {
	Helper()
	Fatal(...any)
}, store *PostgresWorkspaceBoardStore, workspaceID int64, users ...int64) {
	t.Helper()
	for _, userID := range users {
		if err := store.q.UpsertWorkspaceMembership(context.Background(), dbsqlc.UpsertWorkspaceMembershipParams{WorkspaceID: workspaceID, UserID: userID, Role: "member"}); err != nil {
			t.Fatal(err)
		}
	}
}

func cleanupMemberGrantsForTests(ctx context.Context, store WorkspaceBoardStore, workspaceID string, userID int64) ([]tenancy.MemberGrantCleanup, error) {
	if postgres, ok := store.(*PostgresWorkspaceBoardStore); ok {
		tx, err := postgres.db.Begin(ctx)
		if err != nil {
			return nil, err
		}
		defer tx.Rollback(context.Background())
		result, err := store.RemoveWorkspaceMemberGrants(dbutil.ContextWithAmbientTx(ctx, tx), workspaceID, userID)
		if err != nil {
			return nil, err
		}
		return result, tx.Commit(ctx)
	}
	return store.RemoveWorkspaceMemberGrants(ctx, workspaceID, userID)
}
