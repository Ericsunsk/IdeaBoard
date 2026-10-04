// Capture fact fixtures for the explicit PostgreSQL load harness.
package usage

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type CaptureFactsConfig struct {
	BaseUserID       int64
	UserCount        int
	TransactionCount int
	TargetCount      int
	Anchor           time.Time
}

func SeedCaptureFacts(ctx context.Context, db *pgxpool.Pool, config CaptureFactsConfig) error {
	if db == nil || config.BaseUserID <= 0 || config.UserCount < 2 ||
		config.TransactionCount <= 0 || config.TargetCount <= 0 ||
		config.TargetCount > config.TransactionCount || config.Anchor.IsZero() {
		return fmt.Errorf("credits load test support: invalid Capture facts config")
	}
	if err := seedUsersAndAccounts(ctx, db, config); err != nil {
		return err
	}
	if err := setSeedTriggers(ctx, db, false); err != nil {
		return err
	}
	if err := seedTransactions(ctx, db, config); err != nil {
		return err
	}
	if err := seedPostings(ctx, db); err != nil {
		return err
	}
	if err := setSeedTriggers(ctx, db, true); err != nil {
		return err
	}
	return nil
}

func seedUsersAndAccounts(ctx context.Context, db *pgxpool.Pool, config CaptureFactsConfig) error {
	// The user-insert trigger already provisions wallets and user accounts,
	// so these seed inserts only ensure the required rows exist.
	statements := []string{
		`
INSERT INTO users (id, display_name, storage_uid, role, status)
SELECT $1 + series,
       'usage-load-' || series,
       'UL' || lpad(series::text, 4, '0'),
       'user',
       1
FROM generate_series(0, $2 - 1) AS series`,
		`
INSERT INTO credit_wallets (user_id)
SELECT $1 + series FROM generate_series(0, $2 - 1) AS series
ON CONFLICT DO NOTHING`,
		`
INSERT INTO credit_accounts (wallet_user_id, account_role)
SELECT $1 + series, 'user_reserved'
FROM generate_series(0, $2 - 1) AS series
ON CONFLICT DO NOTHING`,
		`
INSERT INTO credit_accounts (account_role, shard_key)
SELECT 'system_revenue', 'usage-load-' || ($1 + series)::text
FROM generate_series(0, $2 - 1) AS series`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(ctx, statement, config.BaseUserID, config.UserCount); err != nil {
			return fmt.Errorf("credits load test support: seed users and accounts: %w", err)
		}
	}
	return nil
}

func seedTransactions(ctx context.Context, db *pgxpool.Pool, config CaptureFactsConfig) error {
	_, err := db.Exec(ctx, `
INSERT INTO credit_transactions (
  transaction_id, wallet_user_id, command_type, idempotency_key, request_hash,
  actor_type, actor_id, source, reason, business_reference_type,
  business_reference_id, command_amount, posting_count,
  available_balance_after, reserved_balance_after, created_at
)
SELECT
  'usage_load_tx_' || series,
  CASE
    WHEN series <= $2::bigint THEN $3::bigint
    ELSE $3::bigint + 1 + ((series - $2::bigint - 1) % ($4::bigint - 1))
  END,
  'capture',
  'usage:load:capture:' || series,
  lpad(to_hex(series), 64, '0'),
  'system',
  'usage-load-harness',
  'usage_load_test',
  'million usage dataset',
  'usage_load_test',
  series::text,
  1 + (series % 10),
  2,
  0,
  0,
  $5::timestamptz - (
    CASE WHEN series <= $2::bigint THEN (series - 1) % 365 ELSE (series - $2::bigint - 1) % 730 END
  ) * interval '1 day'
FROM generate_series(1, $1::bigint) AS series`,
		config.TransactionCount,
		config.TargetCount,
		config.BaseUserID,
		config.UserCount,
		config.Anchor,
	)
	if err != nil {
		return fmt.Errorf("credits load test support: seed Capture transactions: %w", err)
	}
	return nil
}

func seedPostings(ctx context.Context, db *pgxpool.Pool) error {
	_, err := db.Exec(ctx, `
INSERT INTO credit_postings (transaction_id, account_id, amount)
SELECT transactions.transaction_id, posting.account_id, posting.amount
FROM credit_transactions AS transactions
JOIN credit_accounts AS reserved
  ON reserved.wallet_user_id = transactions.wallet_user_id
 AND reserved.account_role = 'user_reserved'
JOIN credit_accounts AS revenue
  ON revenue.wallet_user_id IS NULL
 AND revenue.account_role = 'system_revenue'
 AND revenue.shard_key = 'usage-load-' || transactions.wallet_user_id::text
CROSS JOIN LATERAL (
  VALUES
    (revenue.id, transactions.command_amount),
    (reserved.id, -transactions.command_amount)
) AS posting(account_id, amount)
WHERE transactions.transaction_id LIKE 'usage_load_tx_%'`)
	if err != nil {
		return fmt.Errorf("credits load test support: seed balanced postings: %w", err)
	}
	return nil
}

func setSeedTriggers(ctx context.Context, db *pgxpool.Pool, enabled bool) error {
	action := "DISABLE"
	if enabled {
		action = "ENABLE"
	}
	statements := []string{
		`ALTER TABLE credit_transactions ` + action + ` TRIGGER validate_credit_transaction_on_insert`,
		`ALTER TABLE credit_postings ` + action + ` TRIGGER guard_credit_posting_transaction_scope`,
		`ALTER TABLE credit_postings ` + action + ` TRIGGER apply_credit_posting_balances`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(ctx, statement); err != nil {
			return fmt.Errorf("credits load test support: %s seed trigger: %w", action, err)
		}
	}
	return nil
}
