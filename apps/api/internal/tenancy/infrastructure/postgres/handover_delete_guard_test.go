package postgres

import (
	"context"
	"errors"
	"strings"
	"testing"

	"ideaboard/api/internal/credits"
	creditdomain "ideaboard/api/internal/credits/domain"
	creditports "ideaboard/api/internal/credits/ports"
	"ideaboard/api/internal/tenancy"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Focused coverage for #948 workspace delete guards: in-flight Search and
// unsettled payments converge on the original account first, pending
// handovers block deletion, shared anchors never delete funds while in
// use, and history retains location snapshots with no recovery window.

func seedUnsettledPaymentOrder(t *testing.T, db *pgxpool.Pool, ctx context.Context, userID, billingAccountID int64, orderNumber string) {
	t.Helper()
	var credentialID string
	if err := db.QueryRow(ctx, `
INSERT INTO payment_stripe_credentials (account_id, mode, charges_enabled, stripe_product_id, stripe_webhook_endpoint_id, api_key_ciphertext, webhook_secret_ciphertext, secret_material_kind, created_by)
VALUES ('acct_delete_guard', 'test', TRUE, 'prod_delete_guard', 'we_delete_guard', 'x', 'y', 'encrypted', $1)
RETURNING id::text`, userID).Scan(&credentialID); err != nil {
		t.Fatalf("seed stripe credential: %v", err)
	}
	orderID := uuid.NewString()
	if _, err := db.Exec(ctx, `
INSERT INTO payment_orders (id, public_order_number, public_token, user_id, billing_account_id, client_idempotency_key, request_hash, status, stripe_credential_id, stripe_account_id, stripe_product_id, idea_quantity, base_currency, base_amount_minor, terms_version, refund_policy_version, return_base_url, expires_at)
VALUES ($1::uuid, $2, gen_random_uuid(), $3, $4, $5, $6, 'creating', $7::uuid, 'acct_delete_guard', 'prod_delete_guard', 500, 'usd', 500, 'v1', 'v1', 'https://example.com/', now() + interval '1 hour')`,
		orderID, orderNumber, userID, billingAccountID, "delete-guard-"+orderNumber, strings.Repeat("c", 64), credentialID); err != nil {
		t.Fatalf("seed unsettled payment order: %v", err)
	}
}

func newHandoverCreditService(db *pgxpool.Pool) *credits.Service {
	return credits.NewService(db)
}

func handoverGrantAndHold(t *testing.T, ctx context.Context, db *pgxpool.Pool, service *credits.Service, billingAccountID, userID int64, tag string) string {
	t.Helper()
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	metadata := func(key string) creditdomain.CommandMetadata {
		return creditdomain.CommandMetadata{
			IdempotencyKey: key,
			Actor:          creditdomain.Actor{Type: creditdomain.ActorSystem, ID: "test"},
			Source:         "test", Reason: "handover delete guard",
			BusinessReference: creditdomain.BusinessReference{Type: "vision_search", ID: tag},
		}
	}
	amount, err := creditdomain.NewCreditAmount(100)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.InTx(tx).Grant(ctx, creditports.GrantCommand{BillingAccountID: billingAccountID, Amount: amount, Metadata: metadata(tag + "-grant")}); err != nil {
		t.Fatalf("grant failed: %v", err)
	}
	hold, err := service.InTx(tx).Hold(ctx, creditports.HoldCommand{BillingAccountID: billingAccountID, Amount: amount, Metadata: metadata(tag + "-hold")})
	if err != nil {
		t.Fatalf("hold failed: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return hold.TransactionID
}

func TestDeleteWorkspaceRejectsPendingSearch(t *testing.T) {
	db := newTenancyTestDB(t)
	ctx := context.Background()
	store := NewStore(db, nil)
	baStore := NewBillingAccountStore(db)
	userID := createTenancyTestUser(t, db)
	ws, err := store.CreateWorkspace(ctx, tenancy.CreateWorkspaceParams{Name: "search-guard", OwnerUserID: userID, NewBillingAccount: true})
	if err != nil {
		t.Fatalf("create workspace failed: %v", err)
	}
	account, err := baStore.EnsureBillingAccountForWorkspace(ctx, ws.InternalID, ws.PublicID)
	if err != nil {
		t.Fatalf("resolve account failed: %v", err)
	}
	// Fund the account and hold the search cost so the pending operation
	// carries a real Ledger hold (FK-guarded).
	creditService := newHandoverCreditService(db)
	holdID := handoverGrantAndHold(t, ctx, db, creditService, account.InternalID, userID, "search-delete-guard")
	if _, err := db.Exec(ctx, `
INSERT INTO ai_search_operations (operation_id, user_id, workspace_id, billing_account_id, idempotency_key, request_hash, sku, price_version_id, unit_credits, hold_transaction_id, status, query)
VALUES (gen_random_uuid(), $1, $2, $3, 'search-delete-guard-1', $4, 'ai-search:vision', 1, 10, $5, 'pending', 'cats')`,
		userID, ws.InternalID, account.InternalID, strings.Repeat("d", 64), holdID); err != nil {
		t.Fatalf("seed pending search failed: %v", err)
	}
	if err := store.DeleteWorkspace(ctx, ws.PublicID); !errors.Is(err, tenancy.ErrWorkspaceActiveTasks) {
		t.Fatalf("delete with pending search err = %v, want ErrWorkspaceActiveTasks", err)
	}
	// Settle the operation, then deletion succeeds and the history row
	// retains its workspace location snapshot (no FK, never deleted).
	if _, err := db.Exec(ctx, `UPDATE ai_search_operations SET status = 'succeeded', settled_at = now() WHERE workspace_id = $1`, ws.InternalID); err != nil {
		t.Fatalf("settle search failed: %v", err)
	}
	if err := store.DeleteWorkspace(ctx, ws.PublicID); err != nil {
		t.Fatalf("delete after search settle failed: %v", err)
	}
	var retained int64
	if err := db.QueryRow(ctx, `SELECT COUNT(*) FROM ai_search_operations WHERE workspace_id = $1`, ws.InternalID).Scan(&retained); err != nil || retained != 1 {
		t.Fatalf("search history retained = %d err=%v, want 1 with location snapshot", retained, err)
	}
}

func TestDeleteWorkspaceRejectsUnsettledPayments(t *testing.T) {
	db := newTenancyTestDB(t)
	ctx := context.Background()
	store := NewStore(db, nil)
	baStore := NewBillingAccountStore(db)
	userID := createTenancyTestUser(t, db)
	ws, err := store.CreateWorkspace(ctx, tenancy.CreateWorkspaceParams{Name: "pay-guard", OwnerUserID: userID, NewBillingAccount: true})
	if err != nil {
		t.Fatalf("create workspace failed: %v", err)
	}
	account, err := baStore.EnsureBillingAccountForWorkspace(ctx, ws.InternalID, ws.PublicID)
	if err != nil {
		t.Fatalf("resolve account failed: %v", err)
	}
	seedUnsettledPaymentOrder(t, db, ctx, userID, account.InternalID, "IBP-20240101-DELETE000001")
	if err := store.DeleteWorkspace(ctx, ws.PublicID); !errors.Is(err, tenancy.ErrBillingConflict) {
		t.Fatalf("delete with unsettled payment err = %v, want ErrBillingConflict", err)
	}
	// Converge the payment on the original account, then deletion succeeds
	// and the money fact survives the scope delete.
	if _, err := db.Exec(ctx, `UPDATE payment_orders SET status = 'expired' WHERE billing_account_id = $1`, account.InternalID); err != nil {
		t.Fatalf("settle payment failed: %v", err)
	}
	if err := store.DeleteWorkspace(ctx, ws.PublicID); err != nil {
		t.Fatalf("delete after payment settle failed: %v", err)
	}
	var retained int64
	if err := db.QueryRow(ctx, `SELECT COUNT(*) FROM payment_orders WHERE billing_account_id = $1`, account.InternalID).Scan(&retained); err != nil || retained != 1 {
		t.Fatalf("payment history retained = %d err=%v, want 1 on original account", retained, err)
	}
	var wallets int64
	if err := db.QueryRow(ctx, `SELECT COUNT(*) FROM credit_wallets WHERE billing_account_id = $1`, account.InternalID).Scan(&wallets); err != nil || wallets != 1 {
		t.Fatalf("wallets retained = %d err=%v, want 1", wallets, err)
	}
}

func TestDeleteWorkspaceRejectsPendingHandover(t *testing.T) {
	db := newTenancyTestDB(t)
	ctx := context.Background()
	store := NewStore(db, nil)
	baStore := NewBillingAccountStore(db)
	owner := createTenancyTestUser(t, db)
	recipient := createTenancyTestUser(t, db)
	ws, err := store.CreateWorkspace(ctx, tenancy.CreateWorkspaceParams{Name: "handover-guard", OwnerUserID: owner, NewBillingAccount: true})
	if err != nil {
		t.Fatalf("create workspace failed: %v", err)
	}
	account, err := baStore.EnsureBillingAccountForWorkspace(ctx, ws.InternalID, ws.PublicID)
	if err != nil {
		t.Fatalf("resolve account failed: %v", err)
	}
	invited, err := baStore.InitiateBillingTransferInvite(ctx, account.InternalID, owner, recipient, "key-delete-guard-1")
	if err != nil {
		t.Fatalf("initiate failed: %v", err)
	}
	if err := store.DeleteWorkspace(ctx, ws.PublicID); !errors.Is(err, tenancy.ErrBillingConflict) {
		t.Fatalf("delete with pending handover err = %v, want ErrBillingConflict", err)
	}
	if _, err := baStore.CancelBillingTransferInvite(ctx, account.InternalID, invited.PublicID, owner); err != nil {
		t.Fatalf("cancel failed: %v", err)
	}
	if err := store.DeleteWorkspace(ctx, ws.PublicID); err != nil {
		t.Fatalf("delete after handover cancel failed: %v", err)
	}
	// Handover audit survives the scope delete (plain account id, ba_ snapshot).
	var audits int64
	if err := db.QueryRow(ctx, `SELECT COUNT(*) FROM billing_account_transfer_invites WHERE billing_account_id = $1`, account.InternalID).Scan(&audits); err != nil || audits != 1 {
		t.Fatalf("handover audit retained = %d err=%v, want 1", audits, err)
	}
}

func TestDeleteWorkspaceRejectsSharedAnchorInUse(t *testing.T) {
	db := newTenancyTestDB(t)
	ctx := context.Background()
	store := NewStore(db, nil)
	baStore := NewBillingAccountStore(db)
	owner := createTenancyTestUser(t, db)
	anchor, err := store.CreateWorkspace(ctx, tenancy.CreateWorkspaceParams{Name: "anchor", OwnerUserID: owner, NewBillingAccount: true})
	if err != nil {
		t.Fatalf("create anchor failed: %v", err)
	}
	account, err := baStore.EnsureBillingAccountForWorkspace(ctx, anchor.InternalID, anchor.PublicID)
	if err != nil {
		t.Fatalf("resolve anchor account failed: %v", err)
	}
	// Second business space explicitly shares the anchor account.
	linked, err := store.CreateWorkspace(ctx, tenancy.CreateWorkspaceParams{Name: "linked", OwnerUserID: owner, BillingAccountPublicID: &account.PublicID})
	if err != nil {
		t.Fatalf("create linked workspace failed: %v", err)
	}
	_ = linked
	if err := store.DeleteWorkspace(ctx, anchor.PublicID); !errors.Is(err, tenancy.ErrBillingConflict) {
		t.Fatalf("delete shared anchor err = %v, want ErrBillingConflict", err)
	}
	// The shared funds row survives the rejected delete.
	var accounts int64
	if err := db.QueryRow(ctx, `SELECT COUNT(*) FROM billing_accounts WHERE internal_id = $1`, account.InternalID).Scan(&accounts); err != nil || accounts != 1 {
		t.Fatalf("shared account retained = %d err=%v, want 1", accounts, err)
	}
}
