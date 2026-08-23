DROP FUNCTION IF EXISTS get_auth_runtime_settings();

CREATE FUNCTION get_auth_runtime_settings() RETURNS TABLE(
  registration_enabled boolean,
  branding_project_name text,
  branding_logo_url text
)
    LANGUAGE plpgsql STABLE SECURITY DEFINER
    SET search_path TO 'public', 'pg_catalog', 'pg_temp'
    AS $$
BEGIN
  RETURN QUERY
  SELECT
    COALESCE(
      (
        SELECT lower(btrim(config.value)) = 'true'
        FROM system_configs AS config
        WHERE config.config_key = 'auth_registration_enabled'
        LIMIT 1
      ),
      FALSE
    ),
    COALESCE(
      (
        SELECT btrim(config.value)
        FROM system_configs AS config
        WHERE config.config_key = 'branding_project_name'
        LIMIT 1
      ),
      'IdeaBoard'
    ),
    COALESCE(
      (
        SELECT btrim(config.value)
        FROM system_configs AS config
        WHERE config.config_key = 'branding_logo_url'
        LIMIT 1
      ),
      ''
    );
END;
$$;

DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'ideaboard_auth') THEN
    GRANT EXECUTE ON FUNCTION get_auth_runtime_settings() TO ideaboard_auth;
  END IF;
END;
$$;
