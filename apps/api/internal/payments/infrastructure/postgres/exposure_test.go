package postgres_test

import (
	"strings"
	"testing"

	"ideaboard/api/internal/testdb"

	"ideaboard/api/internal/payments/infrastructure/postgres"
)

// TestQueryPaymentOrderExposurePinsOccupiedIdeas pins the shared
// "occupied Ideas" readmodel: provisional/terminal refunds
// (pending/succeeded/anomaly) count, failed refunds and won disputes do
// not. Disputes are seeded as won so the open/lost predicate is pinned
// without requiring a wallet restriction row.
func TestQueryPaymentOrderExposurePinsOccupiedIdeas(t *testing.T) {
	db := testdb.Open(t)
	ctx := t.Context()

	var credentialID string
	if err := db.QueryRow(ctx, `
INSERT INTO payment_stripe_credentials (account_id, mode, charges_enabled, stripe_product_id, stripe_webhook_endpoint_id, api_key_ciphertext, webhook_secret_ciphertext, secret_material_kind, created_by)
VALUES ('acct_exposure', 'test', TRUE, 'prod_exposure', 'we_exposure', 'x', 'y', 'encrypted', 42)
RETURNING id::text`).Scan(&credentialID); err != nil {
		t.Fatalf("seed stripe credential: %v", err)
	}

	orderID := "11111111-1111-4111-8111-111111111111"
	if _, err := db.Exec(ctx, `
INSERT INTO payment_orders (id, public_order_number, public_token, user_id, billing_account_id, client_idempotency_key, request_hash, status, stripe_credential_id, stripe_account_id, stripe_product_id, idea_quantity, base_currency, base_amount_minor, terms_version, refund_policy_version, return_base_url, expires_at)
VALUES ($1::uuid, 'IBP-20240101-EXPOSURE0001', gen_random_uuid(), 42, ensure_personal_billing_account_for_user(42), 'test-exposure-01', $2, 'cancelled', $3::uuid, 'acct_exposure', 'prod_exposure', 500, 'usd', 500, 'v1', 'v1', 'https://example.com/', now() + interval '1 hour')`,
		orderID, strings.Repeat("a", 64), credentialID); err != nil {
		t.Fatalf("seed payment order: %v", err)
	}

	seedRefund := func(key string, ideas, amount int64, status, succeededAt, failedAt string) {
		t.Helper()
		if _, err := db.Exec(ctx, `
INSERT INTO payment_refunds (id, payment_order_id, user_id, idempotency_key, idea_quantity, amount_minor, currency, status, stripe_charge_id, credit_hold_transaction_id, credit_transaction_id, succeeded_at, failed_at)
VALUES (gen_random_uuid(), $1::uuid, 42, $2, $3, $4, 'usd', $5, 'ch_exposure', '', $6, NULLIF($7, '')::timestamptz, NULLIF($8, '')::timestamptz)`,
			orderID, key, ideas, amount, status, "tx_"+key, succeededAt, failedAt); err != nil {
			t.Fatalf("seed refund %s: %v", key, err)
		}
	}
	seedRefund("pending-1", 100, 10000, "pending", "", "")
	seedRefund("succeeded-1", 50, 5000, "succeeded", "2024-01-02T00:00:00Z", "")
	seedRefund("anomaly-1", 25, 2500, "anomaly", "", "")
	seedRefund("failed-1", 1000, 100000, "failed", "", "2024-01-02T00:00:00Z")

	if _, err := db.Exec(ctx, `
INSERT INTO payment_disputes (id, payment_order_id, user_id, stripe_dispute_id, stripe_charge_id, provider_status, status, amount_minor, currency, target_idea_quantity, debited_idea_quantity, unrecovered_credits, opened_at, closed_at, next_reconciliation_at)
VALUES (gen_random_uuid(), $1::uuid, 42, 'dp_exposure_won', 'ch_exposure', 'closed', 'won', 999, 'usd', 777, 0, 0, now(), now(), now())`,
		orderID); err != nil {
		t.Fatalf("seed won dispute: %v", err)
	}

	exposure, err := postgres.QueryPaymentOrderExposure(ctx, db, orderID)
	if err != nil {
		t.Fatalf("query payment order exposure: %v", err)
	}
	if exposure.RefundedIdeas != 175 || exposure.RefundedAmount != 17500 {
		t.Fatalf("refunded exposure = (%d, %d), want (175, 17500)", exposure.RefundedIdeas, exposure.RefundedAmount)
	}
	if exposure.DisputedIdeas != 0 || exposure.DisputedAmount != 0 {
		t.Fatalf("disputed exposure = (%d, %d), want (0, 0)", exposure.DisputedIdeas, exposure.DisputedAmount)
	}

	emptyID := "22222222-2222-4222-8222-222222222222"
	if _, err := db.Exec(ctx, `
INSERT INTO payment_orders (id, public_order_number, public_token, user_id, billing_account_id, client_idempotency_key, request_hash, status, stripe_credential_id, stripe_account_id, stripe_product_id, idea_quantity, base_currency, base_amount_minor, terms_version, refund_policy_version, return_base_url, expires_at)
VALUES ($1::uuid, 'IBP-20240101-EXPOSURE0002', gen_random_uuid(), 42, ensure_personal_billing_account_for_user(42), 'test-exposure-02', $2, 'cancelled', $3::uuid, 'acct_exposure', 'prod_exposure', 500, 'usd', 500, 'v1', 'v1', 'https://example.com/', now() + interval '1 hour')`,
		emptyID, strings.Repeat("b", 64), credentialID); err != nil {
		t.Fatalf("seed empty payment order: %v", err)
	}
	empty, err := postgres.QueryPaymentOrderExposure(ctx, db, emptyID)
	if err != nil {
		t.Fatalf("query empty payment order exposure: %v", err)
	}
	if empty != (postgres.PaymentOrderExposure{}) {
		t.Fatalf("empty exposure = %+v, want zeros", empty)
	}
}
