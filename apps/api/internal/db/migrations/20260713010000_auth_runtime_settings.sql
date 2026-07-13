-- migrate:up
-- Auth reads only the settings it owns through this typed boundary. The
-- underlying key/value store remains private to the API runtime.
-- statement:begin
CREATE FUNCTION get_auth_runtime_settings()
RETURNS TABLE (registration_enabled BOOLEAN)
AS $$
BEGIN
  RETURN QUERY
  SELECT COALESCE(
      (
        SELECT lower(btrim(config.value)) = 'true'
        FROM system_configs AS config
        WHERE config.config_key = 'auth_registration_enabled'
        LIMIT 1
      ),
      FALSE
    );
END;
$$ LANGUAGE plpgsql STABLE SECURITY DEFINER SET search_path = pg_catalog;
-- statement:end

-- statement:begin
DO $$
DECLARE
  trusted_schema TEXT := current_schema();
BEGIN
  EXECUTE format(
    'ALTER FUNCTION %I.get_auth_runtime_settings() SET search_path TO %I, pg_catalog, pg_temp',
    trusted_schema,
    trusted_schema
  );
END;
$$;
-- statement:end

REVOKE ALL ON FUNCTION get_auth_runtime_settings() FROM PUBLIC;

-- migrate:down
DROP FUNCTION IF EXISTS get_auth_runtime_settings();
