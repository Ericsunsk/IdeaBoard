-- migrate:up
INSERT INTO system_configs (config_key, value)
VALUES
  ('river_worker_concurrency', '5'),
  ('river_task_timeout_seconds', '180'),
  ('river_no_capacity_retry_seconds', '5'),
  ('river_max_retry', '25'),
  ('api_outbound_allow_hosts', '[]'),
  ('nats_retention_days', '7')
ON CONFLICT (config_key) DO NOTHING;

CREATE TABLE system_settings_admin_actions (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  action_type VARCHAR(64) NOT NULL,
  actor_id BIGINT NOT NULL CHECK (actor_id > 0),
  reason VARCHAR(1000) NOT NULL,
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ(3) NOT NULL DEFAULT clock_timestamp(),
  CONSTRAINT ck_system_settings_admin_actions_type CHECK (
    action_type = 'update_operational_settings'
  ),
  CONSTRAINT ck_system_settings_admin_actions_reason CHECK (
    reason = btrim(reason, E' \t\n\r\f') AND char_length(reason) BETWEEN 1 AND 1000
  ),
  CONSTRAINT ck_system_settings_admin_actions_metadata CHECK (
    jsonb_typeof(metadata) = 'object'
  )
);

CREATE INDEX idx_system_settings_admin_actions_actor_id
  ON system_settings_admin_actions (actor_id, id DESC);

-- statement:begin
CREATE FUNCTION prevent_system_settings_admin_action_mutation()
RETURNS TRIGGER AS $$
BEGIN
  RAISE EXCEPTION USING ERRCODE = 'IB401', MESSAGE = 'system settings admin actions are append-only';
END;
$$ LANGUAGE plpgsql SET search_path = pg_catalog;
-- statement:end

CREATE TRIGGER system_settings_admin_actions_append_only
BEFORE UPDATE OR DELETE ON system_settings_admin_actions
FOR EACH ROW EXECUTE FUNCTION prevent_system_settings_admin_action_mutation();

-- statement:begin
DO $$
DECLARE
  trusted_schema TEXT := current_schema();
BEGIN
  EXECUTE format(
    'ALTER FUNCTION %I.prevent_system_settings_admin_action_mutation() SET search_path TO %I, pg_catalog, pg_temp',
    trusted_schema,
    trusted_schema
  );
END $$;
-- statement:end

REVOKE ALL ON FUNCTION prevent_system_settings_admin_action_mutation() FROM PUBLIC;

-- migrate:down
DROP TABLE IF EXISTS system_settings_admin_actions;
DROP FUNCTION IF EXISTS prevent_system_settings_admin_action_mutation();
DELETE FROM system_configs
WHERE config_key IN (
  'river_worker_concurrency',
  'river_task_timeout_seconds',
  'river_no_capacity_retry_seconds',
  'river_max_retry',
  'api_outbound_allow_hosts',
  'nats_retention_days'
);
