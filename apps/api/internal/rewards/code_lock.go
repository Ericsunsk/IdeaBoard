package rewards

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"sort"
	"strconv"

	"github.com/jackc/pgx/v5"
)

func lockRewardCodeIdentity(ctx context.Context, tx pgx.Tx, normalized string) error {
	return lockRewardCodeKeys(ctx, tx, "identity", []string{normalized})
}

func lockRewardCodeIdentities(ctx context.Context, tx pgx.Tx, normalizedCodes []string) error {
	return lockRewardCodeKeys(ctx, tx, "identity", normalizedCodes)
}

func lockRewardCodeID(ctx context.Context, tx pgx.Tx, rewardCodeID int64) error {
	if rewardCodeID <= 0 {
		return fmt.Errorf("lock reward Code ID: invalid ID")
	}
	return lockRewardCodeKeys(ctx, tx, "id", []string{strconv.FormatInt(rewardCodeID, 10)})
}

func lockRewardCodeKeys(ctx context.Context, tx pgx.Tx, scope string, values []string) error {
	keys := make([]int64, 0, len(values))
	seen := make(map[int64]struct{}, len(values))
	for _, value := range values {
		digest := sha256.Sum256([]byte(scope + "\x00" + value))
		key := int64(binary.BigEndian.Uint64(digest[:8]))
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		keys = append(keys, key)
	}
	sort.Slice(keys, func(left, right int) bool { return keys[left] < keys[right] })
	for _, key := range keys {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, key); err != nil {
			return fmt.Errorf("lock reward Code %s: %w", scope, err)
		}
	}
	return nil
}
