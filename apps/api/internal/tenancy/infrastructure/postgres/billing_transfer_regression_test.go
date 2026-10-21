package postgres

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"ideaboard/api/internal/db/dbutil"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/tenancy"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestHandoverDefaultProvisionPreservesFinancialExit(t *testing.T) {
	db := newTenancyTestDB(t)
	ctx := context.Background()
	store, accounts := NewStore(db, nil), NewBillingAccountStore(db)
	previous, recipient := createTenancyTestUser(t, db), createTenancyTestUser(t, db)
	workspace, err := store.EnsureDefaultWorkspace(ctx, previous)
	if err != nil {
		t.Fatal(err)
	}
	account, err := accounts.EnsureBillingAccountForWorkspace(ctx, workspace.InternalID, workspace.PublicID)
	if err != nil {
		t.Fatal(err)
	}
	invite, err := accounts.InitiateBillingTransferInvite(ctx, account.InternalID, previous, recipient, "default-handover")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := accounts.AcceptBillingTransferInvite(ctx, account.InternalID, invite.PublicID, recipient); err != nil {
		t.Fatal(err)
	}
	if err := accounts.RemoveBillingAccountMember(ctx, account.InternalID, previous, previous); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		got, err := store.EnsureDefaultWorkspace(ctx, previous)
		if err != nil || got.PublicID != workspace.PublicID {
			t.Fatalf("default read = %+v, %v", got, err)
		}
	}
	if role, exists, err := accounts.GetBillingAccountRole(ctx, account.InternalID, previous); err != nil || exists {
		t.Fatalf("former financial manager restored: role=%q exists=%v err=%v", role, exists, err)
	}
	if role, exists, err := accounts.GetBillingAccountRole(ctx, account.InternalID, recipient); err != nil || !exists || role != tenancy.BillingRoleOwner {
		t.Fatalf("new owner = %q %v %v", role, exists, err)
	}
}

func TestBillingConcurrentOwnerExitRetainsOwner(t *testing.T) {
	db := newTenancyTestDB(t)
	ctx := context.Background()
	_, accounts, first, second, _, account := handoverFixture(t, db, ctx)
	if _, err := accounts.AddBillingAccountMember(ctx, account.InternalID, first, second, tenancy.BillingRoleOwner); err != nil {
		t.Fatal(err)
	}
	// Slow the actual deletes in this isolated schema so both unprotected
	// count-then-delete paths would observe two owners before either commits.
	if _, err := db.Exec(ctx, `CREATE FUNCTION slow_financial_exit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN PERFORM pg_sleep(0.15); RETURN OLD; END $$;
CREATE TRIGGER slow_financial_exit BEFORE DELETE ON billing_account_members FOR EACH ROW EXECUTE FUNCTION slow_financial_exit()`); err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for _, id := range []int64{first, second} {
		wg.Add(1)
		go func(id int64) {
			defer wg.Done()
			<-start
			errs <- accounts.RemoveBillingAccountMember(ctx, account.InternalID, id, id)
		}(id)
	}
	close(start)
	wg.Wait()
	close(errs)
	succeeded, lastOwner := 0, 0
	for err := range errs {
		if err == nil {
			succeeded++
		} else if errors.Is(err, tenancy.ErrBillingLastOwner) {
			lastOwner++
		} else {
			t.Fatalf("exit: %v", err)
		}
	}
	owners, err := dbsqlc.New(db).CountBillingAccountOwners(ctx, account.InternalID)
	if err != nil || owners != 1 || succeeded != 1 || lastOwner != 1 {
		t.Fatalf("owners=%d successes=%d protected=%d err=%v", owners, succeeded, lastOwner, err)
	}
}

func TestHandoverEveryBusinessWorkspace(t *testing.T) {
	db := newTenancyTestDB(t)
	ctx := context.Background()
	_, accounts, owner, recipient, _, account := handoverFixture(t, db, ctx)
	if _, err := db.Exec(ctx, `WITH created AS (
INSERT INTO workspaces (public_id, owner_user_id, name)
SELECT 'ws_handover_scope_' || i, $1, 'business-' || i FROM generate_series(1,500) AS i RETURNING internal_id)
INSERT INTO workspace_billing_links (workspace_id, billing_account_id) SELECT internal_id, $2 FROM created`, owner, account.InternalID); err != nil {
		t.Fatal(err)
	}
	invite, err := accounts.InitiateBillingTransferInvite(ctx, account.InternalID, owner, recipient, "scope-501")
	if err != nil {
		t.Fatal(err)
	}
	accepted, err := accounts.AcceptBillingTransferInvite(ctx, account.InternalID, invite.PublicID, recipient)
	if err != nil {
		t.Fatal(err)
	}
	if len(accepted.ScopeWorkspacePublicIDs) != 501 {
		t.Fatalf("audit scope = %d, want 501", len(accepted.ScopeWorkspacePublicIDs))
	}
	var granted int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM workspace_memberships AS m JOIN workspaces AS w ON w.internal_id = m.workspace_id WHERE m.user_id = $1 AND m.role = 'owner' AND w.public_id = ANY($2::text[])`, recipient, accepted.ScopeWorkspacePublicIDs).Scan(&granted); err != nil || granted != 501 {
		t.Fatalf("granted=%d err=%v", granted, err)
	}
}

func TestHandoverDecisionRejectsAnotherAccountBeforeMutation(t *testing.T) {
	for _, decision := range []string{"accept", "reject", "cancel"} {
		t.Run(decision, func(t *testing.T) {
			db := newTenancyTestDB(t)
			ctx := context.Background()
			store, accounts, owner, recipient, _, account := handoverFixture(t, db, ctx)
			unrelated, err := store.CreateWorkspace(ctx, tenancy.CreateWorkspaceParams{Name: "unrelated", OwnerUserID: owner, NewBillingAccount: true})
			if err != nil {
				t.Fatal(err)
			}
			other, err := accounts.EnsureBillingAccountForWorkspace(ctx, unrelated.InternalID, unrelated.PublicID)
			if err != nil {
				t.Fatal(err)
			}
			invite, err := accounts.InitiateBillingTransferInvite(ctx, account.InternalID, owner, recipient, "wrong-account")
			if err != nil {
				t.Fatal(err)
			}
			switch decision {
			case "accept":
				_, err = accounts.AcceptBillingTransferInvite(ctx, other.InternalID, invite.PublicID, recipient)
			case "reject":
				_, err = accounts.RejectBillingTransferInvite(ctx, other.InternalID, invite.PublicID, recipient)
			case "cancel":
				_, err = accounts.CancelBillingTransferInvite(ctx, other.InternalID, invite.PublicID, owner)
			}
			if !errors.Is(err, tenancy.ErrBillingNotFound) {
				t.Fatalf("decision err=%v, want not found", err)
			}
			current, err := accounts.GetBillingTransferInvite(ctx, invite.PublicID)
			if err != nil || current.Status != tenancy.BillingTransferPending {
				t.Fatalf("invite changed: %+v %v", current, err)
			}
			role, exists, err := accounts.GetBillingAccountRole(ctx, account.InternalID, owner)
			if err != nil || !exists || role != tenancy.BillingRoleOwner {
				t.Fatalf("owner changed: %s %v %v", role, exists, err)
			}
			if role, exists, err := accounts.GetBillingAccountRole(ctx, account.InternalID, recipient); err != nil || exists {
				t.Fatalf("recipient unexpectedly granted %s %v %v", role, exists, err)
			}
		})
	}
}

func TestBillingMutationWaitsThenRechecksActorAuthority(t *testing.T) {
	for _, mutation := range []string{"add", "update", "detach", "transfer", "initiate", "accept"} {
		t.Run(mutation, func(t *testing.T) {
			db := newTenancyTestDB(t)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_, accounts, owner, recipient, _, account := handoverFixture(t, db, ctx)
			third := createTenancyTestUser(t, db)
			if _, err := accounts.AddBillingAccountMember(ctx, account.InternalID, owner, recipient, tenancy.BillingRoleAdmin); err != nil {
				t.Fatal(err)
			}
			invited, err := accounts.InitiateBillingTransferInvite(ctx, account.InternalID, owner, recipient, "before-revocation")
			if err != nil {
				t.Fatal(err)
			}
			tx, err := db.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx)
			if _, err := dbsqlc.New(tx).LockBillingAccount(ctx, account.InternalID); err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() {
				var err error
				switch mutation {
				case "add":
					_, err = accounts.AddBillingAccountMember(ctx, account.InternalID, owner, third, tenancy.BillingRoleAdmin)
				case "update":
					_, err = accounts.UpdateBillingAccountMemberRole(ctx, account.InternalID, owner, recipient, tenancy.BillingRoleOwner)
				case "detach":
					err = accounts.RemoveBillingAccountMember(ctx, account.InternalID, owner, recipient)
				case "transfer":
					err = accounts.TransferBillingAccountOwnership(ctx, account.InternalID, owner, recipient)
				case "initiate":
					_, err = accounts.InitiateBillingTransferInvite(ctx, account.InternalID, owner, third, "after-revocation")
				case "accept":
					_, err = accounts.AcceptBillingTransferInvite(ctx, account.InternalID, invited.PublicID, recipient)
				}
				done <- err
			}()
			// Observe the real waiting PostgreSQL backend, then revoke the
			// actor while owning the same account serialization lock.
			waitForBillingMutationBlock(t, ctx, db, done, tx.Conn().PgConn().PID())
			if _, err := tx.Exec(ctx, `DELETE FROM billing_account_members WHERE billing_account_id = $1 AND user_id = $2`, account.InternalID, owner); err != nil {
				t.Fatal(err)
			}
			if err := tx.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			err = <-done
			if !errors.Is(err, tenancy.ErrBillingForbidden) && !errors.Is(err, tenancy.ErrBillingConflict) {
				t.Fatalf("stale authority accepted: %v", err)
			}
		})
	}
}

func TestHandoverWorkspaceCapRollsBackWholeDecision(t *testing.T) {
	db := newTenancyTestDB(t)
	ctx := context.Background()
	store, accounts, owner, recipient, _, account := handoverFixture(t, db, ctx)
	full, err := store.CreateWorkspace(ctx, tenancy.CreateWorkspaceParams{Name: "full-linked", OwnerUserID: owner, BillingAccountPublicID: &account.PublicID})
	if err != nil {
		t.Fatal(err)
	}
	for range tenancy.MaxWorkspaceMembers - 1 {
		member := createTenancyTestUser(t, db)
		if _, err := store.AddWorkspaceMember(ctx, full.PublicID, member, tenancy.RoleMember); err != nil {
			t.Fatal(err)
		}
	}
	invite, err := accounts.InitiateBillingTransferInvite(ctx, account.InternalID, owner, recipient, "full-workspace")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := accounts.AcceptBillingTransferInvite(ctx, account.InternalID, invite.PublicID, recipient); !errors.Is(err, tenancy.ErrWorkspaceMemberLimitReached) {
		t.Fatalf("accept = %v, want member cap", err)
	}
	current, err := accounts.GetBillingTransferInvite(ctx, invite.PublicID)
	if err != nil || current.Status != tenancy.BillingTransferPending {
		t.Fatalf("invite changed: %+v %v", current, err)
	}
	role, exists, err := accounts.GetBillingAccountRole(ctx, account.InternalID, owner)
	if err != nil || !exists || role != tenancy.BillingRoleOwner {
		t.Fatalf("owner changed: %s %v %v", role, exists, err)
	}
	if _, exists, err := accounts.GetBillingAccountRole(ctx, account.InternalID, recipient); err != nil || exists {
		t.Fatalf("recipient granted management: %v %v", exists, err)
	}
}

func TestHandoverFinancialMemberCap(t *testing.T) {
	db := newTenancyTestDB(t)
	ctx := context.Background()
	_, accounts, owner, recipient, _, account := handoverFixture(t, db, ctx)
	for range tenancy.MaxBillingAccountMembers - 1 {
		member := createTenancyTestUser(t, db)
		if _, err := accounts.AddBillingAccountMember(ctx, account.InternalID, owner, member, tenancy.BillingRoleAdmin); err != nil {
			t.Fatal(err)
		}
	}
	invite, err := accounts.InitiateBillingTransferInvite(ctx, account.InternalID, owner, recipient, "full-finance")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := accounts.AcceptBillingTransferInvite(ctx, account.InternalID, invite.PublicID, recipient); !errors.Is(err, tenancy.ErrBillingConflict) {
		t.Fatalf("accept = %v, want financial member cap", err)
	}
}

func TestHandoverInvitationRecipientDiscovery(t *testing.T) {
	db := newTenancyTestDB(t)
	ctx := context.Background()
	_, accounts, owner, recipient, _, account := handoverFixture(t, db, ctx)
	unrelated := createTenancyTestUser(t, db)
	invite, err := accounts.InitiateBillingTransferInvite(ctx, account.InternalID, owner, recipient, "discover")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		actor int64
		count int
	}{{owner, 1}, {recipient, 1}, {unrelated, 0}} {
		invites, err := accounts.ListBillingTransferInvitesForAccount(ctx, account.InternalID, tc.actor, 0, 50)
		if err != nil || len(invites) != tc.count {
			t.Fatalf("actor %d invitations = %+v %v", tc.actor, invites, err)
		}
		if tc.count == 1 && invites[0].PublicID != invite.PublicID {
			t.Fatalf("wrong invitation: %+v", invites)
		}
	}
}

func TestBillingWorkspaceScopeChangesWaitForAccount(t *testing.T) {
	for _, mutation := range []string{"create", "link", "delete"} {
		t.Run(mutation, func(t *testing.T) {
			db := newTenancyTestDB(t)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			store, accounts, owner, recipient, _, account := handoverFixture(t, db, ctx)
			workspace, err := store.CreateWorkspace(ctx, tenancy.CreateWorkspaceParams{Name: "scope-target", OwnerUserID: owner})
			if err != nil {
				t.Fatal(err)
			}
			if mutation == "delete" {
				if err := accounts.LinkWorkspaceToBillingAccount(ctx, workspace.InternalID, account.InternalID, owner); err != nil {
					t.Fatal(err)
				}
			}
			tx, err := db.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx)
			if _, err := dbsqlc.New(tx).LockBillingAccount(ctx, account.InternalID); err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() {
				var err error
				switch mutation {
				case "create":
					_, err = store.CreateWorkspace(ctx, tenancy.CreateWorkspaceParams{Name: "blocked-create", OwnerUserID: owner, BillingAccountPublicID: &account.PublicID})
				case "link":
					err = accounts.LinkWorkspaceToBillingAccount(ctx, workspace.InternalID, account.InternalID, owner)
				case "delete":
					err = store.DeleteWorkspace(ctx, workspace.PublicID)
				}
				done <- err
			}()
			waitForBillingMutationBlock(t, ctx, db, done, tx.Conn().PgConn().PID())
			if mutation == "delete" {
				// A handover started before deletion gets the funds guard checked
				// after the account lock wait, before any scope disappears.
				carrier := dbutil.ContextWithAmbientTx(ctx, tx)
				if _, err := accounts.InitiateBillingTransferInvite(carrier, account.InternalID, owner, recipient, "before-delete"); err != nil {
					t.Fatal(err)
				}
			} else {
				if _, err := tx.Exec(ctx, `DELETE FROM billing_account_members WHERE billing_account_id = $1 AND user_id = $2`, account.InternalID, owner); err != nil {
					t.Fatal(err)
				}
			}
			if err := tx.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			err = <-done
			want := tenancy.ErrBillingForbidden
			if mutation == "delete" {
				want = tenancy.ErrBillingConflict
			}
			if !errors.Is(err, want) {
				t.Fatalf("mutation = %v, want %v", err, want)
			}
			if _, err := store.GetWorkspace(ctx, workspace.PublicID); err != nil {
				t.Fatalf("target vanished: %v", err)
			}
		})
	}
}

func TestBillingDeleteSerializesAnchoredAndResolvedAccounts(t *testing.T) {
	db := newTenancyTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	store, accounts, owner, recipient, _, shared := handoverFixture(t, db, ctx)
	workspace, err := store.CreateWorkspace(ctx, tenancy.CreateWorkspaceParams{Name: "prior-anchor", OwnerUserID: owner, NewBillingAccount: true})
	if err != nil {
		t.Fatal(err)
	}
	anchor, err := accounts.EnsureBillingAccountForWorkspace(ctx, workspace.InternalID, workspace.PublicID)
	if err != nil {
		t.Fatal(err)
	}
	if err := accounts.LinkWorkspaceToBillingAccount(ctx, workspace.InternalID, shared.InternalID, owner); err != nil {
		t.Fatal(err)
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err := dbsqlc.New(tx).LockBillingAccount(ctx, anchor.InternalID); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- store.DeleteWorkspace(ctx, workspace.PublicID) }()
	waitForBillingMutationBlock(t, ctx, db, done, tx.Conn().PgConn().PID())
	if _, err := accounts.InitiateBillingTransferInvite(dbutil.ContextWithAmbientTx(ctx, tx), anchor.InternalID, owner, recipient, "anchor-before-delete"); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-done; !errors.Is(err, tenancy.ErrBillingConflict) {
		t.Fatalf("delete = %v, want pending anchor handover conflict", err)
	}
}

func TestBillingDeleteRejectsPayerLinkedDuringWorkspaceLockWait(t *testing.T) {
	db := newTenancyTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	store, accounts, owner, _, _, shared := handoverFixture(t, db, ctx)
	workspace, err := store.CreateWorkspace(ctx, tenancy.CreateWorkspaceParams{Name: "racing-link", OwnerUserID: owner, NewBillingAccount: true})
	if err != nil {
		t.Fatal(err)
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	// NO KEY UPDATE admits the link's FK KEY SHARE while holding deletion's
	// exclusive Workspace lock back until the payer changes.
	if _, err := tx.Exec(ctx, `SELECT internal_id FROM workspaces WHERE internal_id = $1 FOR NO KEY UPDATE`, workspace.InternalID); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- store.DeleteWorkspace(ctx, workspace.PublicID) }()
	waitForBillingMutationBlock(t, ctx, db, done, tx.Conn().PgConn().PID())
	if err := accounts.LinkWorkspaceToBillingAccount(ctx, workspace.InternalID, shared.InternalID, owner); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-done; !errors.Is(err, tenancy.ErrBillingConflict) {
		t.Fatalf("delete = %v, want changed payer conflict", err)
	}
	if _, err := store.GetWorkspace(ctx, workspace.PublicID); err != nil {
		t.Fatalf("Workspace disappeared: %v", err)
	}
}

// Observe the specific transaction blocking the production mutation, without
// depending on SQL text or other concurrent tests' backends.
func waitForBillingMutationBlock(t *testing.T, ctx context.Context, db *pgxpool.Pool, done <-chan error, blockerPID uint32) {
	t.Helper()
	for {
		var blocked bool
		if err := db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_stat_activity WHERE $1::integer = ANY(pg_blocking_pids(pid)))`, int32(blockerPID)).Scan(&blocked); err != nil {
			t.Fatal(err)
		}
		if blocked {
			return
		}
		select {
		case err := <-done:
			t.Fatalf("mutation escaped transaction lock: %v", err)
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(5 * time.Millisecond):
		}
	}
}
