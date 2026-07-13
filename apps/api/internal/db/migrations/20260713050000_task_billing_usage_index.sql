-- migrate:up
-- statement:no-tx
-- statement:index-name=idx_task_billing_snapshots_user_created
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_task_billing_snapshots_user_created
ON task_billing_snapshots (user_id, created_at);

-- migrate:down
-- statement:no-tx
-- statement:index-name=idx_task_billing_snapshots_user_created
DROP INDEX CONCURRENTLY IF EXISTS idx_task_billing_snapshots_user_created;
