package boards

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"ideaboard/api/internal/db/dbutil"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/tenancy"
	tenancypg "ideaboard/api/internal/tenancy/postgres"
)

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
			tenancyStore := tenancypg.NewStore(store.db)
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
