package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"ideaboard/api/internal/tenancy"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// closureFixture provisions an owner, an admin, a member, and an org with
// one workspace on the shared pool. Members hold org membership (join-first)
// so workspace grants resolve.
func closureFixture(t *testing.T, db *pgxpool.Pool, ctx context.Context) (store *Store, owner, admin, member int64, org *tenancy.Organization, ws *tenancy.Workspace, accountID int64) {
	t.Helper()
	store = NewStore(db, nil)
	owner = createTenancyTestUser(t, db)
	admin = createTenancyTestUser(t, db)
	member = createTenancyTestUser(t, db)
	var err error
	org, err = store.CreateOrganization(ctx, owner, "closure-org")
	if err != nil {
		t.Fatalf("create org failed: %v", err)
	}
	if _, err := store.AddOrganizationMember(ctx, org.PublicID, admin, tenancy.RoleAdmin); err != nil {
		t.Fatalf("add admin failed: %v", err)
	}
	if _, err := store.AddOrganizationMember(ctx, org.PublicID, member, tenancy.RoleMember); err != nil {
		t.Fatalf("add member failed: %v", err)
	}
	ws, err = store.CreateOrganizationWorkspace(ctx, org.PublicID, owner, "org-space")
	if err != nil {
		t.Fatalf("create org workspace failed: %v", err)
	}
	if err := db.QueryRow(ctx, `SELECT internal_id FROM billing_accounts WHERE organization_id = $1`, org.InternalID).Scan(&accountID); err != nil {
		t.Fatalf("resolve org account failed: %v", err)
	}
	return store, owner, admin, member, org, ws, accountID
}

func seedPendingOrgTask(t *testing.T, db *pgxpool.Pool, ctx context.Context, ws *tenancy.Workspace, owner, accountID int64) {
	t.Helper()
	publicID := fmt.Sprintf("task_%s", uuid.NewString()[:12])
	if _, err := db.Exec(ctx, `INSERT INTO tasks (public_id, workspace_id, submitter_user_id, billing_account_id, service, workflow_input, status)
VALUES ($1, $2, $3, $4, 'generation', '{}', 'pending')`, publicID, ws.InternalID, owner, accountID); err != nil {
		t.Fatalf("seed pending task failed: %v", err)
	}
}

func seedUnsettledOrgPaymentOrder(t *testing.T, db *pgxpool.Pool, ctx context.Context, owner, accountID int64, keySuffix string) uuid.UUID {
	t.Helper()
	var credentialID uuid.UUID
	if err := db.QueryRow(ctx, `INSERT INTO payment_stripe_credentials
(account_id, mode, charges_enabled, stripe_product_id, stripe_webhook_endpoint_id, api_key_ciphertext, webhook_secret_ciphertext, secret_material_kind, created_by)
VALUES ('acct_test', 'test', true, 'prod_test', 'we_test', 'cipher', 'secret', 'encrypted', $1)
RETURNING id`, owner).Scan(&credentialID); err != nil {
		t.Fatalf("seed credential failed: %v", err)
	}
	orderID := uuid.New()
	orderNumber := fmt.Sprintf("IBP-20261022-%s", strings.ToUpper(strings.ReplaceAll(uuid.NewString(), "-", "")[:12]))
	if _, err := db.Exec(ctx, `INSERT INTO payment_orders
(id, public_order_number, public_token, user_id, billing_account_id, client_idempotency_key, request_hash, status,
 stripe_credential_id, stripe_account_id, stripe_customer_id, stripe_product_id, stripe_checkout_session_id,
 idea_quantity, base_amount_minor, terms_version, refund_policy_version, return_base_url, checkout_url, expires_at)
VALUES ($1, $2, $3, $4, $5, $6, '0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef', 'checkout_open',
 $7, 'acct_test', 'cus_test', 'prod_test', $8, 500, 500, 'v1', 'rp1', 'https://example.test/return', 'https://checkout.test/s', now() + INTERVAL '1 hour')`,
		orderID, orderNumber, uuid.New(), owner, accountID, "closure-key-"+keySuffix, credentialID, "cs_test_"+keySuffix); err != nil {
		t.Fatalf("seed payment order failed: %v", err)
	}
	return orderID
}

func TestOrganizationCloseRestoreLifecycle(t *testing.T) {
	db := newTenancyTestDB(t)
	ctx := context.Background()
	store, owner, _, member, org, ws, accountID := closureFixture(t, db, ctx)

	// The member holds an explicit workspace grant (org Members never
	// inherit workspace access); the grant must survive closure.
	if _, err := store.AddWorkspaceMember(ctx, ws.PublicID, member, tenancy.RoleMember); err != nil {
		t.Fatalf("grant member workspace access failed: %v", err)
	}

	closed, err := store.CloseOrganization(ctx, org.PublicID, owner, "winding down")
	if err != nil {
		t.Fatalf("close failed: %v", err)
	}
	if !closed.IsClosed() || closed.Status() != tenancy.OrganizationStatusClosed {
		t.Fatalf("closed org status = %q, want closed", closed.Status())
	}
	if closed.ClosedBy == nil || *closed.ClosedBy != owner {
		t.Fatalf("closed_by = %+v, want owner %d", closed.ClosedBy, owner)
	}

	// Membership, Workspace, and permission rows are retained.
	members, err := store.ListOrganizationMembers(ctx, org.PublicID)
	if err != nil || len(members) != 3 {
		t.Fatalf("members after close = %d %+v, want 3", len(members), err)
	}
	if _, err := store.GetWorkspace(ctx, ws.PublicID); err != nil {
		t.Fatalf("workspace after close: %v", err)
	}
	// Access is suspended for every role, including the Owner.
	for _, userID := range []int64{owner, member} {
		if _, member, err := store.GetWorkspaceRole(ctx, ws.PublicID, userID); err != nil || member {
			t.Fatalf("workspace role while closed (user %d) = %v %v, want non-member", userID, member, err)
		}
	}
	listed, err := store.ListWorkspacesForUser(ctx, member)
	if err != nil {
		t.Fatalf("list workspaces while closed: %v", err)
	}
	for _, w := range listed {
		if w.OrganizationID != nil && *w.OrganizationID == org.PublicID {
			t.Fatalf("closed org workspace still listed: %s", w.PublicID)
		}
	}
	// Mutations fail closed while closed.
	if _, err := store.UpdateOrganization(ctx, org.PublicID, "renamed"); !errors.Is(err, tenancy.ErrOrganizationClosed) {
		t.Fatalf("rename while closed = %v, want ErrOrganizationClosed", err)
	}
	if _, err := store.AddOrganizationMember(ctx, org.PublicID, createTenancyTestUser(t, db), tenancy.RoleMember); !errors.Is(err, tenancy.ErrOrganizationClosed) {
		t.Fatalf("member add while closed = %v, want ErrOrganizationClosed", err)
	}
	if _, err := store.CreateOrganizationWorkspace(ctx, org.PublicID, owner, "late-space"); !errors.Is(err, tenancy.ErrOrganizationClosed) {
		t.Fatalf("workspace create while closed = %v, want ErrOrganizationClosed", err)
	}

	// Closure audit pins the snapshot plus the fail-closed disposition.
	var auditCount int
	var disposition, reason string
	var available, reserved int64
	if err := db.QueryRow(ctx, `SELECT COUNT(*), MAX(disposition), MAX(reason), MAX(available_balance), MAX(reserved_balance)
FROM organization_closures WHERE organization_id = $1 AND restored_at IS NULL`, org.InternalID).Scan(&auditCount, &disposition, &reason, &available, &reserved); err != nil {
		t.Fatalf("read closure audit failed: %v", err)
	}
	if auditCount != 1 || disposition != tenancy.OrganizationClosureRetained || reason != "winding down" {
		t.Fatalf("closure audit = count %d disposition %q reason %q", auditCount, disposition, reason)
	}
	var membershipJSON, detailJSON []byte
	if err := db.QueryRow(ctx, `SELECT membership_snapshot, disposition_detail FROM organization_closures
WHERE organization_id = $1 AND restored_at IS NULL`, org.InternalID).Scan(&membershipJSON, &detailJSON); err != nil {
		t.Fatalf("read closure snapshots failed: %v", err)
	}
	var pinned []tenancy.OrganizationClosureMemberSnapshot
	if err := json.Unmarshal(membershipJSON, &pinned); err != nil || len(pinned) != 3 {
		t.Fatalf("membership snapshot = %d members %v, want 3", len(pinned), err)
	}
	var detail map[string]any
	if err := json.Unmarshal(detailJSON, &detail); err != nil || detail["outcome"] != "recharge_terms_unresolved" {
		t.Fatalf("disposition detail = %v %v, want recharge_terms_unresolved", detail, err)
	}

	// Funds survive closure: the org Wallet is untouched and nothing
	// entered the owner's Personal account.
	var orgAvailable, personalAvailable int64
	if err := db.QueryRow(ctx, `SELECT balance FROM credit_accounts WHERE billing_account_id = $1 AND account_role = 'user_available'`, accountID).Scan(&orgAvailable); err != nil {
		t.Fatalf("read org wallet failed: %v", err)
	}
	var personalAccount int64
	if err := db.QueryRow(ctx, `SELECT ba.internal_id FROM billing_accounts AS ba
JOIN workspaces AS def ON def.internal_id = ba.workspace_id
WHERE def.owner_user_id = $1 AND def.organization_id IS NULL AND def.is_default LIMIT 1`, owner).Scan(&personalAccount); err == nil {
		if err := db.QueryRow(ctx, `SELECT balance FROM credit_accounts WHERE billing_account_id = $1 AND account_role = 'user_available'`, personalAccount).Scan(&personalAvailable); err != nil {
			t.Fatalf("read personal wallet failed: %v", err)
		}
	}
	if orgAvailable != available || personalAvailable != 0 {
		t.Fatalf("balances after close: org %d (snapshot %d), personal %d", orgAvailable, available, personalAvailable)
	}

	// Owner restores; prior memberships and permissions resume.
	restored, err := store.RestoreOrganization(ctx, org.PublicID, owner)
	if err != nil {
		t.Fatalf("restore failed: %v", err)
	}
	if restored.IsClosed() || restored.Status() != tenancy.OrganizationStatusActive {
		t.Fatalf("restored status = %q, want active", restored.Status())
	}
	if role, ok, err := store.GetWorkspaceRole(ctx, ws.PublicID, member); err != nil || !ok {
		t.Fatalf("member workspace role after restore = %v %v, want member", role, err)
	}
	if role, ok, err := store.GetWorkspaceRole(ctx, ws.PublicID, owner); err != nil || !ok || role != tenancy.RoleOwner {
		t.Fatalf("owner workspace role after restore = %v %v, want owner", role, err)
	}
	members, err = store.ListOrganizationMembers(ctx, org.PublicID)
	if err != nil || len(members) != 3 {
		t.Fatalf("members after restore = %d %+v, want 3", len(members), err)
	}
	var openAudits int
	if err := db.QueryRow(ctx, `SELECT COUNT(*) FROM organization_closures WHERE organization_id = $1 AND restored_at IS NULL`, org.InternalID).Scan(&openAudits); err != nil || openAudits != 0 {
		t.Fatalf("open audits after restore = %d %v, want 0", openAudits, err)
	}
	var history int
	if err := db.QueryRow(ctx, `SELECT COUNT(*) FROM organization_closures WHERE organization_id = $1`, org.InternalID).Scan(&history); err != nil || history != 1 {
		t.Fatalf("closure history after restore = %d %v, want 1", history, err)
	}
}

func TestOrganizationCloseAuthorization(t *testing.T) {
	db := newTenancyTestDB(t)
	ctx := context.Background()
	store, owner, admin, member, org, _, _ := closureFixture(t, db, ctx)

	if _, err := store.CloseOrganization(ctx, org.PublicID, admin, ""); !errors.Is(err, tenancy.ErrForbidden) {
		t.Fatalf("admin close = %v, want ErrForbidden", err)
	}
	if _, err := store.CloseOrganization(ctx, org.PublicID, member, ""); !errors.Is(err, tenancy.ErrForbidden) {
		t.Fatalf("member close = %v, want ErrForbidden", err)
	}
	outsider := createTenancyTestUser(t, db)
	if _, err := store.CloseOrganization(ctx, org.PublicID, outsider, ""); !errors.Is(err, tenancy.ErrNotFound) {
		t.Fatalf("outsider close = %v, want ErrNotFound", err)
	}
	if _, err := store.CloseOrganization(ctx, org.PublicID, owner, ""); err != nil {
		t.Fatalf("owner close failed: %v", err)
	}
	if _, err := store.CloseOrganization(ctx, org.PublicID, owner, "again"); !errors.Is(err, tenancy.ErrOrganizationClosed) {
		t.Fatalf("second close = %v, want ErrOrganizationClosed", err)
	}
	if _, err := store.RestoreOrganization(ctx, org.PublicID, admin); !errors.Is(err, tenancy.ErrForbidden) {
		t.Fatalf("admin restore = %v, want ErrForbidden", err)
	}
	if _, err := store.RestoreOrganization(ctx, org.PublicID, owner); err != nil {
		t.Fatalf("owner restore failed: %v", err)
	}
	if _, err := store.RestoreOrganization(ctx, org.PublicID, owner); !errors.Is(err, tenancy.ErrOrganizationNotClosed) {
		t.Fatalf("second restore = %v, want ErrOrganizationNotClosed", err)
	}
}

func TestOrganizationCloseBlockedByInflightTasks(t *testing.T) {
	db := newTenancyTestDB(t)
	ctx := context.Background()
	store, owner, _, _, org, ws, accountID := closureFixture(t, db, ctx)

	seedPendingOrgTask(t, db, ctx, ws, owner, accountID)
	if _, err := store.CloseOrganization(ctx, org.PublicID, owner, ""); !errors.Is(err, tenancy.ErrOrganizationClosureBlocked) {
		t.Fatalf("close with pending task = %v, want ErrOrganizationClosureBlocked", err)
	}
	// Settle the task; closure proceeds and the terminal fact survives.
	if _, err := db.Exec(ctx, `UPDATE tasks SET status = 'completed', completed_at = now()
WHERE workspace_id = $1 AND status = 'pending'`, ws.InternalID); err != nil {
		t.Fatalf("settle task failed: %v", err)
	}
	closed, err := store.CloseOrganization(ctx, org.PublicID, owner, "")
	if err != nil {
		t.Fatalf("close after settle failed: %v", err)
	}
	if !closed.IsClosed() {
		t.Fatalf("org not closed after settle")
	}
	var terminal int
	if err := db.QueryRow(ctx, `SELECT COUNT(*) FROM tasks WHERE workspace_id = $1 AND status = 'completed'`, ws.InternalID).Scan(&terminal); err != nil || terminal != 1 {
		t.Fatalf("terminal tasks = %d %v, want 1", terminal, err)
	}
}

func TestOrganizationCloseBlockedByUnsettledPayments(t *testing.T) {
	db := newTenancyTestDB(t)
	ctx := context.Background()
	store, owner, _, _, org, _, accountID := closureFixture(t, db, ctx)

	orderID := seedUnsettledOrgPaymentOrder(t, db, ctx, owner, accountID, "blocked")
	if _, err := store.CloseOrganization(ctx, org.PublicID, owner, ""); !errors.Is(err, tenancy.ErrOrganizationClosureBlocked) {
		t.Fatalf("close with unsettled order = %v, want ErrOrganizationClosureBlocked", err)
	}
	if _, err := db.Exec(ctx, `UPDATE payment_orders SET status = 'expired', checkout_url = '', updated_at = now() WHERE id = $1`, orderID); err != nil {
		t.Fatalf("expire order failed: %v", err)
	}
	if _, err := store.CloseOrganization(ctx, org.PublicID, owner, ""); err != nil {
		t.Fatalf("close after order settle failed: %v", err)
	}
}

func TestOrganizationCloseBlockedByPendingRefund(t *testing.T) {
	db := newTenancyTestDB(t)
	ctx := context.Background()
	store, owner, _, _, org, _, accountID := closureFixture(t, db, ctx)

	// A terminal order with a pending refund still blocks: the refund must
	// settle on the fixed payer account first.
	orderID := seedUnsettledOrgPaymentOrder(t, db, ctx, owner, accountID, "refund")
	if _, err := db.Exec(ctx, `UPDATE payment_orders SET status = 'expired', checkout_url = '', updated_at = now() WHERE id = $1`, orderID); err != nil {
		t.Fatalf("expire order failed: %v", err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO payment_refunds
(id, payment_order_id, user_id, idempotency_key, idea_quantity, amount_minor, currency, status, stripe_charge_id, credit_hold_transaction_id)
VALUES ($1, $2, $3, 'closure-refund-1', 100, 100, 'usd', 'pending', 'ch_test', 'ctx_hold_test')`,
		uuid.New(), orderID, owner); err != nil {
		t.Fatalf("seed pending refund failed: %v", err)
	}
	if _, err := store.CloseOrganization(ctx, org.PublicID, owner, ""); !errors.Is(err, tenancy.ErrOrganizationClosureBlocked) {
		t.Fatalf("close with pending refund = %v, want ErrOrganizationClosureBlocked", err)
	}
	if _, err := db.Exec(ctx, `UPDATE payment_refunds SET status = 'failed', failed_at = now(), updated_at = now()
WHERE payment_order_id = $1 AND status = 'pending'`, orderID); err != nil {
		t.Fatalf("fail refund failed: %v", err)
	}
	if _, err := store.CloseOrganization(ctx, org.PublicID, owner, ""); err != nil {
		t.Fatalf("close after refund settle failed: %v", err)
	}
}

func TestOrganizationCloseBlockedByOpenDispute(t *testing.T) {
	db := newTenancyTestDB(t)
	ctx := context.Background()
	store, owner, _, _, org, _, accountID := closureFixture(t, db, ctx)

	orderID := seedUnsettledOrgPaymentOrder(t, db, ctx, owner, accountID, "dispute")
	if _, err := db.Exec(ctx, `UPDATE payment_orders SET status = 'expired', checkout_url = '', updated_at = now() WHERE id = $1`, orderID); err != nil {
		t.Fatalf("expire order failed: %v", err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO credit_wallets (billing_account_id) VALUES ($1) ON CONFLICT DO NOTHING`, accountID); err != nil {
		t.Fatalf("seed wallet failed: %v", err)
	}
	var restrictionID uuid.UUID
	if err := db.QueryRow(ctx, `INSERT INTO credit_wallet_restrictions
(billing_account_id, restriction_type, source, related_object_type, related_object_id, reason, created_by_actor_type, created_by_actor_id)
VALUES ($1, 'spending_frozen', 'stripe_dispute', 'payment_dispute', 'dp_test', '', 'admin', 'test')
RETURNING restriction_id`, accountID).Scan(&restrictionID); err != nil {
		t.Fatalf("seed restriction failed: %v", err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO payment_disputes
(id, payment_order_id, user_id, stripe_dispute_id, stripe_charge_id, provider_status, status, amount_minor, currency,
 target_idea_quantity, debited_idea_quantity, unrecovered_credits, restriction_id, opened_at, next_reconciliation_at)
VALUES ($1, $2, $3, 'dp_test', 'ch_test', 'needs_response', 'open', 500, 'usd', 5, 0, 5, $4, now(), now() + INTERVAL '1 day')`,
		uuid.New(), orderID, owner, restrictionID); err != nil {
		t.Fatalf("seed open dispute failed: %v", err)
	}
	if _, err := store.CloseOrganization(ctx, org.PublicID, owner, ""); !errors.Is(err, tenancy.ErrOrganizationClosureBlocked) {
		t.Fatalf("close with open dispute = %v, want ErrOrganizationClosureBlocked", err)
	}
	if _, err := db.Exec(ctx, `UPDATE payment_disputes SET status = 'lost', closed_at = now(), updated_at = now()
WHERE payment_order_id = $1 AND status = 'open'`, orderID); err != nil {
		t.Fatalf("lose dispute failed: %v", err)
	}
	if _, err := store.CloseOrganization(ctx, org.PublicID, owner, ""); err != nil {
		t.Fatalf("close after dispute settle failed: %v", err)
	}
}

func TestOrganizationCloseBlockedByPendingHandover(t *testing.T) {
	db := newTenancyTestDB(t)
	ctx := context.Background()
	store, owner, _, _, org, _, accountID := closureFixture(t, db, ctx)

	recipient := createTenancyTestUser(t, db)
	var accountPublicID string
	if err := db.QueryRow(ctx, `SELECT public_id FROM billing_accounts WHERE internal_id = $1`, accountID).Scan(&accountPublicID); err != nil {
		t.Fatalf("read account public id failed: %v", err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO billing_account_transfer_invites
(public_id, billing_account_id, billing_account_public_id, from_user_id, to_user_id, idempotency_key)
VALUES ('bti_closuretest01', $1, $2, $3, $4, 'closure-handover-1')`, accountID, accountPublicID, owner, recipient); err != nil {
		t.Fatalf("seed pending handover failed: %v", err)
	}
	if _, err := store.CloseOrganization(ctx, org.PublicID, owner, ""); !errors.Is(err, tenancy.ErrOrganizationClosureBlocked) {
		t.Fatalf("close with pending handover = %v, want ErrOrganizationClosureBlocked", err)
	}
	if _, err := db.Exec(ctx, `UPDATE billing_account_transfer_invites SET status = 'cancelled', decided_at = now(), decided_by = $1
WHERE billing_account_id = $2 AND status = 'pending'`, owner, accountID); err != nil {
		t.Fatalf("cancel handover failed: %v", err)
	}
	closed, err := store.CloseOrganization(ctx, org.PublicID, owner, "")
	if err != nil {
		t.Fatalf("close after handover settle failed: %v", err)
	}
	if !closed.IsClosed() {
		t.Fatalf("org not closed after handover settle")
	}
}
