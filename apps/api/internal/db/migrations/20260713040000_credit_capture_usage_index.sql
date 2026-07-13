-- migrate:up
-- statement:no-tx
-- statement:index-name=idx_credit_transactions_capture_user_created
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_credit_transactions_capture_user_created
ON credit_transactions (wallet_user_id, created_at)
INCLUDE (command_amount)
WHERE command_type = 'capture';

-- migrate:down
-- statement:no-tx
-- statement:index-name=idx_credit_transactions_capture_user_created
DROP INDEX CONCURRENTLY IF EXISTS idx_credit_transactions_capture_user_created;
