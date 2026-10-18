package refunds

import (
	"strings"
	"testing"

	"ideaboard/api/internal/testdb"
)

// TestLoadPendingRefundBalancesPinsOccupiedIdeas pins the refund side of the
// shared "occupied Ideas" readmodel: provisional/terminal refunds
// (pending/succeeded/anomaly) count, failed refunds do not.
func TestLoadPendingRefundBalancesPinsOccupiedIdeas(t *testing.T) {
	db := testdb.Open(t)
	ctx := t.Context()

	var credentialID string
	if err := db.QueryRow(ctx, `
INSERT INTO payment_stripe_credentials (account_id, mode, charges_enabled, stripe_product_id, stripe_webhook_endpoint_id, api_key_ciphertext, webhook_secret_ciphertext, secret_material_kind, created_by)
VALUES ('acct_balances', 'test', TRUE, 'prod_balances', 'we_balances', 'x', 'y', 'encrypted', 42)
RETURNING id::text`).Scan(&credentialID); err != nil {
		t.Fatalf("seed stripe credential: %v", err)
	}

	orderID := "33333333-3333-4333-8333-333333333333"
	if _, err := db.Exec(ctx, `
INSERT INTO payment_orders (id, public_order_number, public_token, user_id, client_idempotency_key, request_hash, status, stripe_credential_id, stripe_account_id, stripe_product_id, idea_quantity, base_currency, base_amount_minor, terms_version, refund_policy_version, return_base_url, expires_at)
VALUES ($1::uuid, 'IBP-20240101-BALANCES0001', gen_random_uuid(), 42, 'test-balances-01', $2, 'cancelled', $3::uuid, 'acct_balances', 'prod_balances', 500, 'usd', 500, 'v1', 'v1', 'https://example.com/', now() + interval '1 hour')`,
		orderID, strings.Repeat("c", 64), credentialID); err != nil {
		t.Fatalf("seed payment order: %v", err)
	}

	seedRefund := func(key string, ideas, amount int64, status, succeededAt, failedAt string) {
		t.Helper()
		if _, err := db.Exec(ctx, `
INSERT INTO payment_refunds (id, payment_order_id, user_id, idempotency_key, idea_quantity, amount_minor, currency, status, stripe_charge_id, credit_hold_transaction_id, credit_transaction_id, succeeded_at, failed_at)
VALUES (gen_random_uuid(), $1::uuid, 42, $2, $3, $4, 'usd', $5, 'ch_balances', '', $6, NULLIF($7, '')::timestamptz, NULLIF($8, '')::timestamptz)`,
			orderID, key, ideas, amount, status, "tx_"+key, succeededAt, failedAt); err != nil {
			t.Fatalf("seed refund %s: %v", key, err)
		}
	}
	seedRefund("pending-1", 100, 10000, "pending", "", "")
	seedRefund("succeeded-1", 50, 5000, "succeeded", "2024-01-02T00:00:00Z", "")
	seedRefund("anomaly-1", 25, 2500, "anomaly", "", "")
	seedRefund("failed-1", 1000, 100000, "failed", "", "2024-01-02T00:00:00Z")

	already, disputed, err := loadPendingRefundBalances(ctx, db, orderID)
	if err != nil {
		t.Fatalf("load pending refund balances: %v", err)
	}
	if already != 175 || disputed != 0 {
		t.Fatalf("pending refund balances = (%d, %d), want (175, 0)", already, disputed)
	}
}
