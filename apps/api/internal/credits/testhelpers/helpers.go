package testhelpers

import (
	"fmt"
	"testing"

	"ideaboard/api/internal/credits/domain"

	"github.com/jackc/pgx/v5/pgxpool"
)

func MustCreditAmount(t testing.TB, value int64) domain.CreditAmount {
	t.Helper()
	amount, err := domain.NewCreditAmount(value)
	if err != nil {
		t.Fatalf("NewCreditAmount(%d) error = %v", value, err)
	}
	return amount
}

func TestMetadata(idempotencyKey string, referenceID string) domain.CommandMetadata {
	return domain.CommandMetadata{
		IdempotencyKey: idempotencyKey,
		Actor:          domain.Actor{Type: domain.ActorAdmin, ID: "7"},
		Source:         "integration_test",
		Reason:         "verify credits core",
		BusinessReference: domain.BusinessReference{
			Type: "test",
			ID:   referenceID,
		},
	}
}

func PurchaseMetadata(idempotencyKey, orderID string) domain.CommandMetadata {
	return domain.CommandMetadata{
		IdempotencyKey: idempotencyKey,
		Actor:          domain.Actor{Type: domain.ActorService, ID: "payments"},
		Source:         "stripe_purchase",
		Reason:         "Stripe payment credited",
		BusinessReference: domain.BusinessReference{
			Type: "payment_order",
			ID:   orderID,
		},
	}
}

func WalletSystemShard(billingAccountID int64) string {
	return fmt.Sprintf("billing-account:%d", billingAccountID)
}

// MustBillingAccount ensures the designer user row plus its deterministic
// initial personal Billing Account (#916) and returns the internal account
// id for account-keyed Commands. The users-insert trigger already
// provisions the account wallet; this only resolves it.
func MustBillingAccount(t testing.TB, pool *pgxpool.Pool, userID int64) int64 {
	t.Helper()
	EnsureAdminOperationUser(t, pool, userID)
	var accountID int64
	if err := pool.QueryRow(t.Context(), `SELECT ensure_personal_billing_account_for_user($1)`, userID).Scan(&accountID); err != nil {
		t.Fatalf("resolve personal billing account %d: %v", userID, err)
	}
	if accountID <= 0 {
		t.Fatalf("personal billing account %d not resolved", userID)
	}
	return accountID
}

func EnsureAdminOperationUser(t testing.TB, pool *pgxpool.Pool, billingAccountIDs ...int64) {
	t.Helper()
	for _, billingAccountID := range billingAccountIDs {
		storageUID := fmt.Sprintf("A%05d", billingAccountID)
		if _, err := pool.Exec(t.Context(), `
INSERT INTO users (id, display_name, storage_uid, role, status)
VALUES ($1, $2, $3, 'user', 1)
ON CONFLICT (id) DO NOTHING`, billingAccountID, fmt.Sprintf("admin-operation-user-%d", billingAccountID), storageUID); err != nil {
			t.Fatalf("seed admin operation user %d: %v", billingAccountID, err)
		}
		if _, err := pool.Exec(t.Context(), `INSERT INTO users_used_uids (storage_uid) VALUES ($1) ON CONFLICT DO NOTHING`, storageUID); err != nil {
			t.Fatalf("seed admin operation user UID %d: %v", billingAccountID, err)
		}
	}
}
