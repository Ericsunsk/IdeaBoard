package rewards

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"sort"

	"github.com/jackc/pgx/v5"
)

func lockRewardCodeIdentity(ctx context.Context, tx pgx.Tx, normalized string) error {
	return lockRewardCodeIdentities(ctx, tx, []string{normalized})
}

func lockRewardCodeIdentities(ctx context.Context, tx pgx.Tx, normalizedCodes []string) error {
	keys := make([]int64, 0, len(normalizedCodes))
	seen := make(map[int64]struct{}, len(normalizedCodes))
	for _, normalized := range normalizedCodes {
		digest := sha256.Sum256([]byte(normalized))
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
			return fmt.Errorf("lock reward Code identity: %w", err)
		}
	}
	return nil
}
