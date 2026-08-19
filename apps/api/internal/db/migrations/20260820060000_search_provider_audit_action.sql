ALTER TABLE system_settings_admin_actions
  DROP CONSTRAINT ck_system_settings_admin_actions_type;

ALTER TABLE system_settings_admin_actions
  ADD CONSTRAINT ck_system_settings_admin_actions_type CHECK (
    action_type IN ('update_operational_settings', 'update_outbound_policy', 'update_search_providers')
  );
