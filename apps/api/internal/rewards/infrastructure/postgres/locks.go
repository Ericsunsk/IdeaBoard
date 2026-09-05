// Package postgres implements PostgreSQL persistence and transaction advisory locking
// for the Rewards bounded context.
package postgres

import (
	"context"
	"fmt"
	"strconv"

	"ideaboard/api/internal/rewards/domain"
	"ideaboard/api/internal/rewards/ports"

	"github.com/jackc/pgx/v5"
)

// Locker implements ports.CodeLocker using PostgreSQL advisory transaction locks.
type Locker struct{}

// NewLocker constructs a new PostgreSQL-backed advisory locker.
func NewLocker() *Locker {
	return &Locker{}
}

func (l *Locker) LockRewardCodeID(ctx context.Context, tx pgx.Tx, rewardCodeID int64) error {
	return LockRewardCodeID(ctx, tx, rewardCodeID)
}

func (l *Locker) LockRewardCodeIdentity(ctx context.Context, tx pgx.Tx, normalized string) error {
	return LockRewardCodeIdentity(ctx, tx, normalized)
}

// LockRewardCodeID acquires the transaction-scoped lock for a RewardCode ID.
// The key is derived from the full int64 ID; it is never narrowed to int32.
func LockRewardCodeID(ctx context.Context, tx pgx.Tx, rewardCodeID int64) error {
	if rewardCodeID <= 0 {
		return fmt.Errorf("invalid reward code id: %d", rewardCodeID)
	}
	return lockKey(ctx, tx, "id", strconv.FormatInt(rewardCodeID, 10))
}

// LockRewardCodeIdentity acquires the transaction-scoped lock for a normalized
// RewardCode identity.
func LockRewardCodeIdentity(ctx context.Context, tx pgx.Tx, normalized string) error {
	if normalized == "" {
		return fmt.Errorf("normalized code required")
	}
	return lockKey(ctx, tx, "identity", normalized)
}

func lockKey(ctx context.Context, tx pgx.Tx, scope, value string) error {
	key := domain.DeriveAdvisoryLockKey(scope, value)
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1::BIGINT)`, key); err != nil {
		return fmt.Errorf("acquire reward code %s lock: %w", scope, err)
	}
	return nil
}

var _ ports.CodeLocker = (*Locker)(nil)
