-- ai-gateway T12: allow gateway settings audit action types alongside the
-- existing operational settings actions.
ALTER TABLE system_settings_admin_actions DROP CONSTRAINT ck_system_settings_admin_actions_type;
ALTER TABLE system_settings_admin_actions ADD CONSTRAINT ck_system_settings_admin_actions_type CHECK (((action_type)::text = ANY (ARRAY[('update_operational_settings'::character varying)::text, ('update_outbound_policy'::character varying)::text, ('update_search_providers'::character varying)::text, ('update_ai_gateway'::character varying)::text, ('update_ai_gateway_model'::character varying)::text])));
