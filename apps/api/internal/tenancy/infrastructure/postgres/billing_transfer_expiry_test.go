package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"ideaboard/api/internal/tenancy"
)

// Focused coverage for handover invite expiry (#948 follow-up): invites
// lapse 7 days after issue, lapsed invites never move funds or membership,
// decisions on them fail closed with conflict, and the owner may re-issue
// (same or different recipient) while a lapsed invite never blocks scope
// deletion.

func TestHandoverInviteExpiryDefaultsToSevenDays(t *testing.T) {
	db := newTenancyTestDB(t)
	ctx := context.Background()
	_, baStore, owner, recipient, _, account := handoverFixture(t, db, ctx)
	invited, err := baStore.InitiateBillingTransferInvite(ctx, account.InternalID, owner, recipient, "key-expiry-default-1")
	if err != nil {
		t.Fatalf("initiate failed: %v", err)
	}
	if invited.ExpiresAt.IsZero() {
		t.Fatal("expires_at is zero, want 7 days after issue")
	}
	if got := invited.ExpiresAt.Sub(invited.CreatedAt); got != 7*24*time.Hour {
		t.Fatalf("expires_at - created_at = %v, want exactly 7d", got)
	}
	if invited.Expired(time.Now()) {
		t.Fatal("fresh invite reports expired, want live")
	}
}

func TestHandoverDecisionsRefuseLapsedInvite(t *testing.T) {
	db := newTenancyTestDB(t)
	ctx := context.Background()
	_, baStore, owner, recipient, _, account := handoverFixture(t, db, ctx)
	invited, err := baStore.InitiateBillingTransferInvite(ctx, account.InternalID, owner, recipient, "key-expiry-refuse-1")
	if err != nil {
		t.Fatalf("initiate failed: %v", err)
	}
	if _, err := db.Exec(ctx, `UPDATE billing_account_transfer_invites SET expires_at = now() - INTERVAL '1 hour' WHERE public_id = $1`, invited.PublicID); err != nil {
		t.Fatalf("force lapse failed: %v", err)
	}
	current, err := baStore.GetBillingTransferInvite(ctx, invited.PublicID)
	if err != nil {
		t.Fatalf("fetch lapsed invite failed: %v", err)
	}
	if !current.Expired(time.Now()) {
		t.Fatal("lapsed invite does not report expired")
	}
	// Every decision fails closed with conflict; nothing moves.
	if _, err := baStore.AcceptBillingTransferInvite(ctx, account.InternalID, invited.PublicID, recipient); !errors.Is(err, tenancy.ErrBillingConflict) {
		t.Fatalf("accept-after-lapse err = %v, want ErrBillingConflict", err)
	}
	if _, err := baStore.RejectBillingTransferInvite(ctx, account.InternalID, invited.PublicID, recipient); !errors.Is(err, tenancy.ErrBillingConflict) {
		t.Fatalf("reject-after-lapse err = %v, want ErrBillingConflict", err)
	}
	if _, err := baStore.CancelBillingTransferInvite(ctx, account.InternalID, invited.PublicID, owner); !errors.Is(err, tenancy.ErrBillingConflict) {
		t.Fatalf("cancel-after-lapse err = %v, want ErrBillingConflict", err)
	}
	if role, ok, err := baStore.GetBillingAccountRole(ctx, account.InternalID, owner); err != nil || !ok || role != tenancy.BillingRoleOwner {
		t.Fatalf("owner role = %q ok=%v err=%v, want owner (nothing moved)", role, ok, err)
	}
	if _, ok, _ := baStore.GetBillingAccountRole(ctx, account.InternalID, recipient); ok {
		t.Fatal("recipient gained management from a lapsed invite, want none")
	}
	still, err := baStore.GetBillingTransferInvite(ctx, invited.PublicID)
	if err != nil || still.Status != tenancy.BillingTransferPending {
		t.Fatalf("lapsed invite status = %+v %v, want pending (re-issuable)", still, err)
	}
}

func TestHandoverReissueAfterLapse(t *testing.T) {
	db := newTenancyTestDB(t)
	ctx := context.Background()
	_, baStore, owner, recipient, _, account := handoverFixture(t, db, ctx)
	invited, err := baStore.InitiateBillingTransferInvite(ctx, account.InternalID, owner, recipient, "key-expiry-reissue-1")
	if err != nil {
		t.Fatalf("initiate failed: %v", err)
	}
	if _, err := db.Exec(ctx, `UPDATE billing_account_transfer_invites SET expires_at = now() - INTERVAL '1 hour' WHERE public_id = $1`, invited.PublicID); err != nil {
		t.Fatalf("force lapse failed: %v", err)
	}
	// Same recipient, fresh key: the pending row refreshes in place.
	reissued, err := baStore.InitiateBillingTransferInvite(ctx, account.InternalID, owner, recipient, "key-expiry-reissue-2")
	if err != nil {
		t.Fatalf("re-issue failed: %v", err)
	}
	if reissued.PublicID != invited.PublicID || reissued.Status != tenancy.BillingTransferPending {
		t.Fatalf("reissued = %+v, want same bti_ id back to pending", reissued)
	}
	if reissued.Expired(time.Now()) {
		t.Fatal("reissued invite reports expired, want fresh 7-day lapse")
	}
	var pending int64
	if err := db.QueryRow(ctx, `SELECT COUNT(*) FROM billing_account_transfer_invites WHERE billing_account_id = $1 AND status = 'pending'`, account.InternalID).Scan(&pending); err != nil || pending != 1 {
		t.Fatalf("pending invites = %d err=%v, want exactly 1 (no fork)", pending, err)
	}
	// The refreshed invite accepts normally: lapse never moved anything,
	// re-issue restores the live handover.
	if _, err := baStore.AcceptBillingTransferInvite(ctx, account.InternalID, reissued.PublicID, recipient); err != nil {
		t.Fatalf("accept after re-issue failed: %v", err)
	}
}

func TestHandoverReissueAfterLapseToDifferentRecipient(t *testing.T) {
	db := newTenancyTestDB(t)
	ctx := context.Background()
	store, baStore, owner, recipient, ws, account := handoverFixture(t, db, ctx)
	other := createTenancyTestUser(t, db)
	if _, err := store.AddOrganizationMember(ctx, *ws.OrganizationID, other, tenancy.RoleMember); err != nil {
		t.Fatalf("add other to org failed: %v", err)
	}
	if _, err := store.AddWorkspaceMember(ctx, ws.PublicID, other, tenancy.RoleMember); err != nil {
		t.Fatalf("add other to workspace failed: %v", err)
	}
	invited, err := baStore.InitiateBillingTransferInvite(ctx, account.InternalID, owner, recipient, "key-expiry-redirect-1")
	if err != nil {
		t.Fatalf("initiate failed: %v", err)
	}
	if _, err := db.Exec(ctx, `UPDATE billing_account_transfer_invites SET expires_at = now() - INTERVAL '1 hour' WHERE public_id = $1`, invited.PublicID); err != nil {
		t.Fatalf("force lapse failed: %v", err)
	}
	redirected, err := baStore.InitiateBillingTransferInvite(ctx, account.InternalID, owner, other, "key-expiry-redirect-2")
	if err != nil {
		t.Fatalf("re-issue to another recipient failed: %v", err)
	}
	if redirected.PublicID != invited.PublicID || redirected.ToUserID != other {
		t.Fatalf("redirected = %+v, want same row re-addressed to other", redirected)
	}
}

func TestHandoverLapsedInviteNeverBlocksScopeDelete(t *testing.T) {
	db := newTenancyTestDB(t)
	ctx := context.Background()
	store, baStore, owner, recipient, ws, account := handoverFixture(t, db, ctx)
	invited, err := baStore.InitiateBillingTransferInvite(ctx, account.InternalID, owner, recipient, "key-expiry-delete-1")
	if err != nil {
		t.Fatalf("initiate failed: %v", err)
	}
	if _, err := db.Exec(ctx, `UPDATE billing_account_transfer_invites SET expires_at = now() - INTERVAL '1 hour' WHERE public_id = $1`, invited.PublicID); err != nil {
		t.Fatalf("force lapse failed: %v", err)
	}
	// A lapsed invite moves nothing, so scope deletion proceeds; the lapsed
	// row survives as audit history on the plain account id.
	if err := store.DeleteWorkspace(ctx, ws.PublicID); err != nil {
		t.Fatalf("delete with only a lapsed invite failed: %v", err)
	}
	var audits int64
	if err := db.QueryRow(ctx, `SELECT COUNT(*) FROM billing_account_transfer_invites WHERE billing_account_id = $1`, account.InternalID).Scan(&audits); err != nil || audits != 1 {
		t.Fatalf("handover audit retained = %d err=%v, want 1", audits, err)
	}
}
