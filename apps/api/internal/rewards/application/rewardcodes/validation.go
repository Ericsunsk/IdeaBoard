package rewardcodes

import (
	"context"
	"fmt"
)

// ValidateActiveCodeKeyVersions prevents removing a key required to redeem a
// persisted Code or authenticate an idempotent create replay.
func ValidateActiveCodeKeyVersions(ctx context.Context, pool PoolInterface, keyring CodeKeyring) error {
	if pool == nil {
		return fmt.Errorf("validate reward Code key versions: service unavailable")
	}
	previousVersion := ""
	if keyring.PreviousVersion() != "" {
		previousVersion = keyring.PreviousVersion()
	}
	var unsupportedCodeExists bool
	err := pool.QueryRow(ctx, `
SELECT EXISTS (
  SELECT 1
  FROM reward_codes
  WHERE key_version <> $1 AND ($2 = '' OR key_version <> $2)
)`, keyring.CurrentVersion(), previousVersion).Scan(&unsupportedCodeExists)
	if err != nil {
		return fmt.Errorf("validate active reward Code key versions: %w", err)
	}
	if unsupportedCodeExists {
		return fmt.Errorf("persisted reward Codes require an unconfigured HMAC key version")
	}
	return nil
}
