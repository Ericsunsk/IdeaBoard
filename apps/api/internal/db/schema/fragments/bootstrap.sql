-- Static platform defaults use column lists and database defaults so every
-- fresh installation receives the same deterministic builtin records.
INSERT INTO storage_profiles (profile_id, name, provider, enabled, is_current, builtin, status, config)
VALUES ('builtin-local', '本地存储', 'local', TRUE, TRUE, TRUE, 'active', '{}'::jsonb)
ON CONFLICT (profile_id) DO NOTHING;

INSERT INTO system_configs (config_key, value)
VALUES ('api_outbound_allow_hosts', '[]')
ON CONFLICT (config_key) DO NOTHING;

-- Bind every privileged routine to its installed schema and keep PUBLIC from
-- bypassing the runtime privilege model.
DO $$
DECLARE
  routine RECORD;
  target_schema TEXT := current_schema();
BEGIN
  FOR routine IN
    SELECT
      schemas.nspname AS schema_name,
      functions.proname AS function_name,
      pg_catalog.pg_get_function_identity_arguments(functions.oid) AS arguments
    FROM pg_catalog.pg_proc AS functions
    JOIN pg_catalog.pg_namespace AS schemas ON schemas.oid = functions.pronamespace
    WHERE schemas.nspname = target_schema
      AND functions.prosecdef
  LOOP
    EXECUTE format(
      'ALTER FUNCTION %I.%I(%s) SET search_path TO %I, pg_catalog, pg_temp',
      routine.schema_name,
      routine.function_name,
      routine.arguments,
      target_schema
    );
    EXECUTE format(
      'REVOKE ALL ON FUNCTION %I.%I(%s) FROM PUBLIC',
      routine.schema_name,
      routine.function_name,
      routine.arguments
    );
  END LOOP;
END;
$$;
