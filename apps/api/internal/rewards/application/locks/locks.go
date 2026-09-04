// Package locks contains PostgreSQL advisory-lock helpers shared by Rewards
// application services. Keeping key derivation in one place is required so
// RewardCode mutation and redemption serialize on the same lock.
package locks

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5"
)

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
	digest := sha256.Sum256([]byte(scope + "\x00" + value))
	key := int64(binary.BigEndian.Uint64(digest[:8]))
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1::BIGINT)`, key); err != nil {
		return fmt.Errorf("acquire reward code %s lock: %w", scope, err)
	}
	return nil
}
