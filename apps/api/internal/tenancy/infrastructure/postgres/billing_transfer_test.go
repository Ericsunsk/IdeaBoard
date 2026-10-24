package postgres

import (
	"context"
	"errors"
	"sync"
	"testing"

	"ideaboard/api/internal/tenancy"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Focused coverage for #948 invite-style studio handover: only Owner
// initiates, recipient accepts/rejects, initiator cancels, idempotent
// replay, concurrent collapse, Default exclusion, workspace scope, old
// Owner exit, and funds stability (no new account, no balance move).

func handoverFixture(t *testing.T, db *pgxpool.Pool, ctx context.Context) (store *Store, baStore *BillingAccountStore, owner, recipient int64, ws *tenancy.Workspace, account tenancy.BillingAccount) {
	t.Helper()
	store = NewStore(db, nil)
	baStore = NewBillingAccountStore(db)
	owner = createTenancyTestUser(t, db)
	recipient = createTenancyTestUser(t, db)
	// Organization business workspace (ADR-0038 handover scope: org
	// workspaces on the shared pool; personal workspaces resolve to the
	// owner's survivor and carry no handover scope).
	org, err := store.CreateOrganization(ctx, owner, "handover-org")
	if err != nil {
		t.Fatalf("create org failed: %v", err)
	}
	ws, err = store.CreateOrganizationWorkspace(ctx, org.PublicID, owner, "studio-a")
	if err != nil {
		t.Fatalf("create workspace failed: %v", err)
	}
	account, err = baStore.EnsureBillingAccountForWorkspace(ctx, ws.InternalID, ws.PublicID)
	if err != nil {
		t.Fatalf("resolve account failed: %v", err)
	}
	// Recipient joins the Organization first (#951 join-first: org
	// workspaces admit only org members), then holds workspace membership
	// so the HTTP prelude (member-only) admits accept/reject; acceptance
	// then promotes to Owner.
	if _, err := store.AddOrganizationMember(ctx, org.PublicID, recipient, tenancy.RoleMember); err != nil {
		t.Fatalf("add recipient to org failed: %v", err)
	}
	if _, err := store.AddWorkspaceMember(ctx, ws.PublicID, recipient, tenancy.RoleMember); err != nil {
		t.Fatalf("add recipient to workspace failed: %v", err)
	}
	return store, baStore, owner, recipient, ws, account
}

func TestHandoverInviteRequiresOwner(t *testing.T) {
	db := newTenancyTestDB(t)
	ctx := context.Background()
	_, baStore, owner, recipient, ws, account := handoverFixture(t, db, ctx)
	// Non-owner (recipient, not yet billing manager) cannot initiate.
	if _, err := baStore.InitiateBillingTransferInvite(ctx, account.InternalID, recipient, owner, "key-nonowner-1"); !errors.Is(err, tenancy.ErrBillingForbidden) {
		t.Fatalf("non-owner initiate err = %v, want ErrBillingForbidden", err)
	}
	// Owner initiates.
	invited, err := baStore.InitiateBillingTransferInvite(ctx, account.InternalID, owner, recipient, "key-owner-1")
	if err != nil || invited == nil {
		t.Fatalf("owner initiate failed: %v %+v", err, invited)
	}
	if invited.Status != tenancy.BillingTransferPending || invited.FromUserID != owner || invited.ToUserID != recipient {
		t.Fatalf("invite = %+v, want pending owner->recipient", invited)
	}
	_ = ws
}

func TestHandoverInviteIdempotentReplayAndConflict(t *testing.T) {
	db := newTenancyTestDB(t)
	ctx := context.Background()
	_, baStore, owner, recipient, _, account := handoverFixture(t, db, ctx)
	other := createTenancyTestUser(t, db)
	first, err := baStore.InitiateBillingTransferInvite(ctx, account.InternalID, owner, recipient, "key-replay-1")
	if err != nil {
		t.Fatalf("first initiate failed: %v", err)
	}
	// Same key + same parties replays the first invite.
	replayed, err := baStore.InitiateBillingTransferInvite(ctx, account.InternalID, owner, recipient, "key-replay-1")
	if err != nil || replayed.PublicID != first.PublicID {
		t.Fatalf("replay = %+v %v, want same %q", replayed, err, first.PublicID)
	}
	// Same key + different recipient conflicts (never replays another's invite).
	if _, err := baStore.InitiateBillingTransferInvite(ctx, account.InternalID, owner, other, "key-replay-1"); !errors.Is(err, tenancy.ErrBillingConflict) {
		t.Fatalf("same-key different-recipient err = %v, want ErrBillingConflict", err)
	}
	// Different key while pending conflicts (at most one pending per account).
	if _, err := baStore.InitiateBillingTransferInvite(ctx, account.InternalID, owner, other, "key-replay-2"); !errors.Is(err, tenancy.ErrBillingConflict) {
		t.Fatalf("second pending err = %v, want ErrBillingConflict", err)
	}
}

func TestHandoverAcceptGrantsScopeWithoutDefault(t *testing.T) {
	db := newTenancyTestDB(t)
	ctx := context.Background()
	store := NewStore(db, nil)
	baStore := NewBillingAccountStore(db)
	owner := createTenancyTestUser(t, db)
	recipient := createTenancyTestUser(t, db)
	// Organization with two business workspaces sharing the org pool
	// (ADR-0038 handover scope: org workspaces, never Default).
	org, err := store.CreateOrganization(ctx, owner, "Handover Org")
	if err != nil {
		t.Fatalf("create org failed: %v", err)
	}
	ws, err := store.CreateOrganizationWorkspace(ctx, org.PublicID, owner, "org-studio-a")
	if err != nil {
		t.Fatalf("create org workspace failed: %v", err)
	}
	wsB, err := store.CreateOrganizationWorkspace(ctx, org.PublicID, owner, "org-studio-b")
	if err != nil {
		t.Fatalf("create second org workspace failed: %v", err)
	}
	account, err := baStore.EnsureBillingAccountForWorkspace(ctx, ws.InternalID, ws.PublicID)
	if err != nil {
		t.Fatalf("resolve org pool failed: %v", err)
	}
	if account.OwnerType != BillingOwnerOrganization {
		t.Fatalf("handover account owner_type = %q, want organization", account.OwnerType)
	}
	if _, err := store.AddOrganizationMember(ctx, org.PublicID, recipient, tenancy.RoleMember); err != nil {
		t.Fatalf("add recipient to org failed: %v", err)
	}
	if _, err := store.AddWorkspaceMember(ctx, ws.PublicID, recipient, tenancy.RoleMember); err != nil {
		t.Fatalf("add recipient to workspace failed: %v", err)
	}
	// Default workspace for the owner (never transferred).
	def, err := store.EnsureDefaultWorkspace(ctx, owner)
	if err != nil {
		t.Fatalf("ensure default failed: %v", err)
	}
	var beforeAccounts int64
	if err := db.QueryRow(ctx, `SELECT COUNT(*) FROM billing_accounts`).Scan(&beforeAccounts); err != nil {
		t.Fatalf("count accounts failed: %v", err)
	}
	invited, err := baStore.InitiateBillingTransferInvite(ctx, account.InternalID, owner, recipient, "key-accept-1")
	if err != nil {
		t.Fatalf("initiate failed: %v", err)
	}
	decided, err := baStore.AcceptBillingTransferInvite(ctx, account.InternalID, invited.PublicID, recipient)
	if err != nil || decided.Status != tenancy.BillingTransferAccepted {
		t.Fatalf("accept failed: %v %+v", err, decided)
	}
	// Billing roles: recipient Owner, previous Owner Admin.
	roleTo, ok, err := baStore.GetBillingAccountRole(ctx, account.InternalID, recipient)
	if err != nil || !ok || roleTo != tenancy.BillingRoleOwner {
		t.Fatalf("recipient role = %q ok=%v err=%v, want owner", roleTo, ok, err)
	}
	roleFrom, ok, err := baStore.GetBillingAccountRole(ctx, account.InternalID, owner)
	if err != nil || !ok || roleFrom != tenancy.BillingRoleAdmin {
		t.Fatalf("previous owner role = %q ok=%v err=%v, want admin", roleFrom, ok, err)
	}
	// Workspace scope: recipient Owner on both business workspaces.
	for _, publicID := range []string{ws.PublicID, wsB.PublicID} {
		role, ok, err := store.GetWorkspaceRole(ctx, publicID, recipient)
		if err != nil || !ok || role != tenancy.RoleOwner {
			t.Fatalf("workspace %s recipient role = %q ok=%v err=%v, want owner", publicID, role, ok, err)
		}
	}
	// Default never transfers: recipient holds no membership there.
	if _, ok, _ := store.GetWorkspaceRole(ctx, def.PublicID, recipient); ok {
		t.Fatalf("recipient gained Default workspace membership, want none")
	}
	// Scope snapshot names the business workspaces only.
	if len(decided.ScopeWorkspacePublicIDs) != 2 {
		t.Fatalf("scope = %v, want 2 business workspaces", decided.ScopeWorkspacePublicIDs)
	}
	for _, id := range decided.ScopeWorkspacePublicIDs {
		if id == def.PublicID {
			t.Fatalf("scope contains Default workspace %q", id)
		}
	}
	// No new billing account (no wallet fork, no funds move).
	var afterAccounts int64
	if err := db.QueryRow(ctx, `SELECT COUNT(*) FROM billing_accounts`).Scan(&afterAccounts); err != nil {
		t.Fatalf("count accounts failed: %v", err)
	}
	if afterAccounts != beforeAccounts {
		t.Fatalf("billing accounts %d -> %d, want stable (no new wallet)", beforeAccounts, afterAccounts)
	}
	// Old Owner can exit (detach management, funds retained).
	if err := baStore.RemoveBillingAccountMember(ctx, account.InternalID, owner, owner); err != nil {
		t.Fatalf("previous owner exit failed: %v", err)
	}
	if _, ok, _ := baStore.GetBillingAccountRole(ctx, account.InternalID, owner); ok {
		t.Fatalf("previous owner still holds management after exit")
	}
	// Wallet row retained after exit (shared funds never deleted/frozen).
	var wallets int64
	if err := db.QueryRow(ctx, `SELECT COUNT(*) FROM credit_wallets WHERE billing_account_id = $1`, account.InternalID).Scan(&wallets); err != nil || wallets != 1 {
		t.Fatalf("wallets for account = %d err=%v, want 1 retained", wallets, err)
	}
}

func TestHandoverRejectAndCancel(t *testing.T) {
	db := newTenancyTestDB(t)
	ctx := context.Background()
	_, baStore, owner, recipient, _, account := handoverFixture(t, db, ctx)
	invited, err := baStore.InitiateBillingTransferInvite(ctx, account.InternalID, owner, recipient, "key-reject-1")
	if err != nil {
		t.Fatalf("initiate failed: %v", err)
	}
	// Only the recipient may reject.
	other := createTenancyTestUser(t, db)
	if _, err := baStore.RejectBillingTransferInvite(ctx, account.InternalID, invited.PublicID, other); !errors.Is(err, tenancy.ErrBillingForbidden) {
		t.Fatalf("outsider reject err = %v, want ErrBillingForbidden", err)
	}
	rejected, err := baStore.RejectBillingTransferInvite(ctx, account.InternalID, invited.PublicID, recipient)
	if err != nil || rejected.Status != tenancy.BillingTransferRejected {
		t.Fatalf("reject failed: %v %+v", err, rejected)
	}
	// Terminal invites reject further transitions.
	if _, err := baStore.AcceptBillingTransferInvite(ctx, account.InternalID, invited.PublicID, recipient); !errors.Is(err, tenancy.ErrBillingConflict) {
		t.Fatalf("accept-after-reject err = %v, want ErrBillingConflict", err)
	}
	// Cancel path: fresh invite, only initiator cancels.
	invited2, err := baStore.InitiateBillingTransferInvite(ctx, account.InternalID, owner, recipient, "key-cancel-1")
	if err != nil {
		t.Fatalf("second initiate failed: %v", err)
	}
	if _, err := baStore.CancelBillingTransferInvite(ctx, account.InternalID, invited2.PublicID, recipient); !errors.Is(err, tenancy.ErrBillingForbidden) {
		t.Fatalf("recipient cancel err = %v, want ErrBillingForbidden", err)
	}
	cancelled, err := baStore.CancelBillingTransferInvite(ctx, account.InternalID, invited2.PublicID, owner)
	if err != nil || cancelled.Status != tenancy.BillingTransferCancelled {
		t.Fatalf("cancel failed: %v %+v", err, cancelled)
	}
}

func TestHandoverConcurrentInitiatesCollapse(t *testing.T) {
	db := newTenancyTestDB(t)
	ctx := context.Background()
	_, baStore, owner, recipient, _, account := handoverFixture(t, db, ctx)
	const callers = 8
	results := make([]*tenancy.BillingTransferInvite, callers)
	errs := make([]error, callers)
	var wg sync.WaitGroup
	for i := range results {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			invited, err := baStore.InitiateBillingTransferInvite(ctx, account.InternalID, owner, recipient, "key-concurrent-1")
			results[index] = invited
			errs[index] = err
		}(i)
	}
	wg.Wait()
	var winner string
	for i, err := range errs {
		if err != nil {
			// Losers that raced the pending guard report conflict; both
			// outcomes converge without forking a second pending invite.
			if !errors.Is(err, tenancy.ErrBillingConflict) {
				t.Fatalf("caller %d err = %v, want nil or ErrBillingConflict", i, err)
			}
			continue
		}
		if results[i] == nil {
			t.Fatalf("caller %d nil invite without error", i)
		}
		if winner == "" {
			winner = results[i].PublicID
		} else if results[i].PublicID != winner {
			t.Fatalf("concurrent invites forked: %q vs %q", winner, results[i].PublicID)
		}
	}
	if winner == "" {
		t.Fatal("no concurrent initiate succeeded")
	}
	var pending int64
	if err := db.QueryRow(ctx, `SELECT COUNT(*) FROM billing_account_transfer_invites WHERE billing_account_id = $1 AND status = 'pending'`, account.InternalID).Scan(&pending); err != nil || pending != 1 {
		t.Fatalf("pending invites = %d err=%v, want exactly 1", pending, err)
	}
}

func TestHandoverAcceptFailsWhenInitiatorDemoted(t *testing.T) {
	db := newTenancyTestDB(t)
	ctx := context.Background()
	_, baStore, owner, recipient, _, account := handoverFixture(t, db, ctx)
	// Second owner to demote the initiator before acceptance.
	second := createTenancyTestUser(t, db)
	if _, err := baStore.AddBillingAccountMember(ctx, account.InternalID, owner, second, tenancy.BillingRoleOwner); err != nil {
		t.Fatalf("add second owner failed: %v", err)
	}
	invited, err := baStore.InitiateBillingTransferInvite(ctx, account.InternalID, owner, recipient, "key-demote-1")
	if err != nil {
		t.Fatalf("initiate failed: %v", err)
	}
	// Concurrent management change: initiator steps down before accept.
	if _, err := baStore.UpdateBillingAccountMemberRole(ctx, account.InternalID, second, owner, tenancy.BillingRoleAdmin); err != nil {
		t.Fatalf("demote initiator failed: %v", err)
	}
	if _, err := baStore.AcceptBillingTransferInvite(ctx, account.InternalID, invited.PublicID, recipient); !errors.Is(err, tenancy.ErrBillingConflict) {
		t.Fatalf("accept-after-demote err = %v, want ErrBillingConflict", err)
	}
}

func TestBillingMemberDetachKeepsFunds(t *testing.T) {
	db := newTenancyTestDB(t)
	ctx := context.Background()
	_, baStore, owner, recipient, _, account := handoverFixture(t, db, ctx)
	if _, err := baStore.AddBillingAccountMember(ctx, account.InternalID, owner, recipient, tenancy.BillingRoleAdmin); err != nil {
		t.Fatalf("add admin failed: %v", err)
	}
	// Admin self-exits: only the management row leaves.
	if err := baStore.RemoveBillingAccountMember(ctx, account.InternalID, recipient, recipient); err != nil {
		t.Fatalf("admin self-exit failed: %v", err)
	}
	var wallets, members int64
	if err := db.QueryRow(ctx, `SELECT COUNT(*) FROM credit_wallets WHERE billing_account_id = $1`, account.InternalID).Scan(&wallets); err != nil || wallets != 1 {
		t.Fatalf("wallets = %d err=%v, want 1 retained", wallets, err)
	}
	if err := db.QueryRow(ctx, `SELECT COUNT(*) FROM billing_account_members WHERE billing_account_id = $1`, account.InternalID).Scan(&members); err != nil || members != 1 {
		t.Fatalf("members = %d err=%v, want 1 (owner only)", members, err)
	}
	// Last Owner cannot exit (never orphan funds management).
	if err := baStore.RemoveBillingAccountMember(ctx, account.InternalID, owner, owner); !errors.Is(err, tenancy.ErrBillingLastOwner) {
		t.Fatalf("last-owner exit err = %v, want ErrBillingLastOwner", err)
	}
}

// Membership fixtures no longer seed an account default (#953 retires the
// #947 default gate): invites always succeed and absent quotas read as
// unlimited.
func configureWorkspaceDefaultQuota(t *testing.T, db *pgxpool.Pool, workspace *tenancy.Workspace) {
	t.Helper()
	ctx := context.Background()
	if _, err := NewBillingAccountStore(db).EnsureBillingAccountForWorkspace(ctx, workspace.InternalID, workspace.PublicID); err != nil {
		t.Fatal(err)
	}
}
