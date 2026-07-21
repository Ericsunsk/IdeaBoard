package rewards

import (
	"bytes"
	"testing"
	"time"

	"ideaboard/api/internal/credits"

	"github.com/jackc/pgx/v5/pgxpool"
)

func newTestRewardService(t *testing.T, pool *pgxpool.Pool, creditService *credits.Service) *Service {
	t.Helper()
	return newTestRewardServiceAt(t, pool, creditService, time.Now())
}

func newTestRewardServiceAt(
	t *testing.T,
	pool *pgxpool.Pool,
	creditService *credits.Service,
	now time.Time,
) *Service {
	t.Helper()
	return NewServiceWithOptions(pool, creditService, newTestRewardKeyring(t), ServiceOptions{
		Now: func() time.Time { return now },
	})
}

func newTestRewardKeyring(t *testing.T) CodeKeyring {
	t.Helper()
	keyring, err := NewCodeKeyring(CodeKey{
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
