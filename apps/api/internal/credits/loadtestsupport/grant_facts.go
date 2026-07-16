package loadtestsupport

import (
	"context"
	"database/sql"
	"fmt"
)

type SequentialGrantFactsConfig struct {
	BaseUserID           int64
	Count                int
	SystemAccountID      int64
	AvailableAccountBase int64
	ReservedAccountBase  int64
}

func SeedSequentialGrantFacts(ctx context.Context, tx *sql.Tx, config SequentialGrantFactsConfig) error {
	if tx == nil || config.BaseUserID <= 0 || config.Count <= 0 || config.SystemAccountID <= 0 ||
		config.AvailableAccountBase <= 0 || config.ReservedAccountBase <= 0 {
		return fmt.Errorf("credits load test support: invalid sequential Grant facts config")
	}
	statements := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO credit_wallets (user_id)
SELECT $1 + series FROM generate_series(1, $2::BIGINT) series`, []any{config.BaseUserID, config.Count}},
		{`INSERT INTO credit_accounts (id, wallet_user_id, account_role, balance)
SELECT $1 + series, $3 + series, 'user_available', 0 FROM generate_series(1, $4::BIGINT) series
UNION ALL
SELECT $2 + series, $3 + series, 'user_reserved', 0 FROM generate_series(1, $4::BIGINT) series`, []any{config.AvailableAccountBase, config.ReservedAccountBase, config.BaseUserID, config.Count}},
		{`INSERT INTO credit_accounts (id, wallet_user_id, account_role, shard_key, balance)
VALUES ($1, NULL, 'system_issuance', 'reward-load', 0)`, []any{config.SystemAccountID}},
		{`ALTER TABLE credit_transactions DISABLE TRIGGER validate_credit_transaction_on_insert`, nil},
		{`ALTER TABLE credit_postings DISABLE TRIGGER guard_credit_posting_transaction_scope`, nil},
		{`INSERT INTO credit_transactions (
  transaction_id, wallet_user_id, command_type, idempotency_key, request_hash,
  actor_type, actor_id, source, reason, business_reference_type,
  business_reference_id, command_amount, posting_count, available_balance_after, reserved_balance_after, created_at
)
SELECT 'reward_load_tx_' || series, $1 + series, 'grant', 'reward:load:' || series,
       lpad(to_hex(series), 64, '0'), 'system', 'reward-load', 'reward_redemption',
       '兑换奖励', 'reward_redemption', series::TEXT, 1, 2, 1, 0,
       now() - (($2 - series)::TEXT || ' microseconds')::INTERVAL
FROM generate_series(1, $2::BIGINT) series`, []any{config.BaseUserID, config.Count}},
		{`INSERT INTO credit_postings (transaction_id, account_id, amount)
SELECT 'reward_load_tx_' || series, posting.account_id, posting.amount
FROM generate_series(1, $1::BIGINT) series
CROSS JOIN LATERAL (VALUES ($2::BIGINT, -1::BIGINT), ($3 + series, 1::BIGINT)) posting(account_id, amount)`, []any{config.Count, config.SystemAccountID, config.AvailableAccountBase}},
		{`ALTER TABLE credit_transactions ENABLE TRIGGER validate_credit_transaction_on_insert`, nil},
		{`ALTER TABLE credit_postings ENABLE TRIGGER guard_credit_posting_transaction_scope`, nil},
	}
	return executeStatements(ctx, tx, "seed sequential Grant facts", statements)
}

type SingleWalletGrantFactsConfig struct {
	UserID                int64
	Count                 int
	SystemAccountID       int64
	AvailableAccountID    int64
	BusinessReferenceBase int64
}

func SeedSingleWalletGrantFacts(ctx context.Context, tx *sql.Tx, config SingleWalletGrantFactsConfig) error {
	if tx == nil || config.UserID <= 0 || config.Count <= 0 || config.SystemAccountID <= 0 ||
		config.AvailableAccountID <= 0 || config.BusinessReferenceBase <= 0 {
		return fmt.Errorf("credits load test support: invalid single-Wallet Grant facts config")
	}
	statements := []struct {
		query string
		args  []any
	}{
		{`ALTER TABLE credit_transactions DISABLE TRIGGER validate_credit_transaction_on_insert`, nil},
		{`ALTER TABLE credit_postings DISABLE TRIGGER guard_credit_posting_transaction_scope`, nil},
		{`INSERT INTO credit_transactions (
  transaction_id, wallet_user_id, command_type, idempotency_key, request_hash,
  actor_type, actor_id, source, reason, business_reference_type,
  business_reference_id, command_amount, posting_count, available_balance_after, reserved_balance_after
)
SELECT 'reward_history_tx_' || series, $1, 'grant', 'reward:history:' || series,
       lpad(to_hex($2 + series), 64, '0'), 'system', 'reward-load', 'reward_redemption',
       '兑换奖励', 'reward_redemption', ($3 + series)::TEXT,
       1, 2, series + 1, 0
FROM generate_series(1, $4::BIGINT) series`, []any{config.UserID, config.Count, config.BusinessReferenceBase, config.Count}},
		{`INSERT INTO credit_postings (transaction_id, account_id, amount)
SELECT 'reward_history_tx_' || series, posting.account_id, posting.amount
FROM generate_series(1, $1::BIGINT) series
CROSS JOIN LATERAL (VALUES ($2::BIGINT, -1::BIGINT), ($3::BIGINT, 1::BIGINT)) posting(account_id, amount)`, []any{config.Count, config.SystemAccountID, config.AvailableAccountID}},
		{`ALTER TABLE credit_transactions ENABLE TRIGGER validate_credit_transaction_on_insert`, nil},
		{`ALTER TABLE credit_postings ENABLE TRIGGER guard_credit_posting_transaction_scope`, nil},
	}
	return executeStatements(ctx, tx, "seed single-Wallet Grant facts", statements)
}

func SeedSequentialWallets(ctx context.Context, db *sql.DB, baseUserID int64, count int) error {
	if db == nil || baseUserID <= 0 || count <= 0 {
		return fmt.Errorf("credits load test support: invalid sequential Wallet config")
	}
	statements := []string{
		`INSERT INTO credit_wallets (user_id)
SELECT $1 + series FROM generate_series(0, $2::BIGINT - 1) series`,
		`INSERT INTO credit_accounts (wallet_user_id, account_role)
SELECT $1 + series, role
FROM generate_series(0, $2::BIGINT - 1) series
CROSS JOIN (VALUES ('user_available'), ('user_reserved')) roles(role)`,
	}
	for _, statement := range statements {
		if _, err := db.ExecContext(ctx, statement, baseUserID, count); err != nil {
			return fmt.Errorf("credits load test support: seed sequential Wallets: %w", err)
		}
	}
	return nil
}

func executeStatements(
	ctx context.Context,
	tx *sql.Tx,
	operation string,
	statements []struct {
		query string
		args  []any
	},
) error {
	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement.query, statement.args...); err != nil {
			return fmt.Errorf("credits load test support: %s: %w", operation, err)
		}
	}
	return nil
}
