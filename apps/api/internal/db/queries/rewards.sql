-- name: ListRewardCampaigns :many
SELECT id, display_name, internal_note, credit_amount, mode, status,
       max_redemptions, redemption_count, starts_at, expires_at,
       created_by, created_at, updated_at
FROM reward_campaigns
WHERE (sqlc.arg(before_id)::BIGINT <= 0 OR id < sqlc.arg(before_id))
ORDER BY id DESC
LIMIT sqlc.arg(page_limit);

-- name: GetRewardCampaign :one
SELECT id, display_name, internal_note, credit_amount, mode, status,
       max_redemptions, redemption_count, starts_at, expires_at,
       created_by, created_at, updated_at
FROM reward_campaigns
WHERE id = sqlc.arg(id);

-- name: GetRewardPublicCodeMetadata :one
SELECT id, campaign_id, code_hint, issued_by, issued_at
FROM reward_codes
WHERE campaign_id = sqlc.arg(campaign_id);
