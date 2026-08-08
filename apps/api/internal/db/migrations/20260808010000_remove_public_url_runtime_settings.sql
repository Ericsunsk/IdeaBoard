-- migrate:up
DELETE FROM system_configs
WHERE config_key IN ('public_base_url', 'public_asset_base_url');

-- migrate:down
-- Deployment-owned origins intentionally have no database representation.
SELECT 1;
