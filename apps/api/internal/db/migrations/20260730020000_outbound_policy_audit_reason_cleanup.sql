-- migrate:up
ALTER TABLE system_settings_admin_actions
  DROP CONSTRAINT ck_system_settings_admin_actions_reason,
  DROP COLUMN reason;

-- migrate:down
ALTER TABLE system_settings_admin_actions
  ADD COLUMN reason VARCHAR(1000) NOT NULL DEFAULT 'outbound policy update';

ALTER TABLE system_settings_admin_actions
  ALTER COLUMN reason DROP DEFAULT,
  ADD CONSTRAINT ck_system_settings_admin_actions_reason CHECK (
    reason = btrim(reason, E' \t\n\r\f') AND char_length(reason) BETWEEN 1 AND 1000
  );
