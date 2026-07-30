-- migrate:up
DELETE FROM system_configs
WHERE config_key IN (
  'river_worker_concurrency',
  'river_task_timeout_seconds',
  'river_no_capacity_retry_seconds',
  'river_max_retry',
  'nats_retention_days'
);

ALTER TABLE system_settings_admin_actions
  DROP CONSTRAINT ck_system_settings_admin_actions_type;

ALTER TABLE system_settings_admin_actions
  ADD CONSTRAINT ck_system_settings_admin_actions_type CHECK (
    action_type IN ('update_operational_settings', 'update_outbound_policy')
  );

-- migrate:down
-- Keep the expanded action_type constraint because append-only audit rows may
-- already contain update_outbound_policy facts.
INSERT INTO system_configs (config_key, value)
VALUES
  ('river_worker_concurrency', '5'),
  ('river_task_timeout_seconds', '180'),
  ('river_no_capacity_retry_seconds', '5'),
  ('river_max_retry', '25'),
  ('nats_retention_days', '7')
ON CONFLICT (config_key) DO NOTHING;
