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

func WalletSystemShard(userID int64) string {
	return fmt.Sprintf("wallet:%d", userID)
}

func EnsureAdminOperationUser(t testing.TB, pool *pgxpool.Pool, userIDs ...int64) {
	t.Helper()
	for _, userID := range userIDs {
		storageUID := fmt.Sprintf("A%05d", userID)
		if _, err := pool.Exec(t.Context(), `
INSERT INTO users (id, display_name, storage_uid, role, status)
VALUES ($1, $2, $3, 'user', 1)
ON CONFLICT (id) DO NOTHING`, userID, fmt.Sprintf("admin-operation-user-%d", userID), storageUID); err != nil {
			t.Fatalf("seed admin operation user %d: %v", userID, err)
		}
		if _, err := pool.Exec(t.Context(), `INSERT INTO users_used_uids (storage_uid) VALUES ($1) ON CONFLICT DO NOTHING`, storageUID); err != nil {
			t.Fatalf("seed admin operation user UID %d: %v", userID, err)
		}
	}
}
