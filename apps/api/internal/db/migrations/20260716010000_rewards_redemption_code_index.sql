-- migrate:up
-- statement:no-tx
-- statement:index-name=idx_reward_redemptions_code_id
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_reward_redemptions_code_id
ON reward_redemptions (code_id);

-- migrate:down
-- statement:no-tx
-- statement:index-name=idx_reward_redemptions_code_id
DROP INDEX CONCURRENTLY IF EXISTS idx_reward_redemptions_code_id;
