-- migrate:up
-- statement:no-tx
-- statement:index-name=idx_reward_redemptions_code_id_desc
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_reward_redemptions_code_id_desc
ON reward_redemptions (code_id, id DESC) INCLUDE (redeemed_at);

-- statement:no-tx
-- statement:index-name=idx_reward_campaigns_mode_id
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_reward_campaigns_mode_id
ON reward_campaigns (mode, id DESC);

-- statement:no-tx
-- statement:index-name=idx_reward_redemptions_code_id
DROP INDEX CONCURRENTLY IF EXISTS idx_reward_redemptions_code_id;

-- migrate:down
-- statement:no-tx
-- statement:index-name=idx_reward_redemptions_code_id
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_reward_redemptions_code_id
ON reward_redemptions (code_id);

-- statement:no-tx
-- statement:index-name=idx_reward_campaigns_mode_id
DROP INDEX CONCURRENTLY IF EXISTS idx_reward_campaigns_mode_id;

-- statement:no-tx
-- statement:index-name=idx_reward_redemptions_code_id_desc
DROP INDEX CONCURRENTLY IF EXISTS idx_reward_redemptions_code_id_desc;
