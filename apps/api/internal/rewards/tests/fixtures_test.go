package tests

import (
	"bytes"
	"context"
	"ideaboard/api/internal/credits"
	"ideaboard/api/internal/rewards"
	rewardscredits "ideaboard/api/internal/rewards/infrastructure/credits"
	"ideaboard/api/internal/rewards/infrastructure/postgres"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func newTestRewardService(t *testing.T, pool *pgxpool.Pool, creditService *credits.Service) *rewards.Service {
	t.Helper()
	return newTestRewardServiceAt(t, pool, creditService, time.Now())
}

func newTestRewardServiceAt(
	t *testing.T,
	pool *pgxpool.Pool,
	creditService *credits.Service,
	now time.Time,
) *rewards.Service {
	t.Helper()
	return rewards.NewServiceWithOptions(pool, rewardscredits.New(creditService), newTestRewardKeyring(t), rewards.ServiceOptions{
		Now: func() time.Time { return now },
	})
}

func newTestRewardKeyring(t *testing.T) rewards.CodeKeyring {
	t.Helper()
	keyring, err := rewards.NewCodeKeyring(rewards.CodeKey{
		Version: "test-v1",
		Secret:  bytes.Repeat([]byte{0x2a}, 32),
	}, nil)
	if err != nil {
		t.Fatalf("NewCodeKeyring() error = %v", err)
	}
	return keyring
}

func provisionRewardWallet(t *testing.T, pool *pgxpool.Pool, creditService *credits.Service, userID int64) {
	t.Helper()
	tx, err := pool.Begin(t.Context())
	if err != nil {
		t.Fatalf("begin wallet transaction: %v", err)
	}
	defer func() { _ = tx.Rollback(t.Context()) }()
	if err := creditService.LifecycleInTx(tx).ProvisionWallet(t.Context(), userID); err != nil {
		t.Fatalf("ProvisionWallet() error = %v", err)
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatalf("commit wallet: %v", err)
	}
}

func lockRewardCodeID(ctx context.Context, tx pgx.Tx, rewardCodeID int64) error {
	return postgres.LockRewardCodeID(ctx, tx, rewardCodeID)
}

// Reward-code resource helpers (merged from code_service_helpers_test.go,
// renamed by resource: reward-code service test helpers).
func rewardCodeString(value string) *string {
	return &value
}
