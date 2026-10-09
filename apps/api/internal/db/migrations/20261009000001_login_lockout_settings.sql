-- Expose login-lockout budgets to the Auth guard-settings reader.
-- Additive only: get_auth_runtime_settings() stays frozen (changing its
-- return table would need DROP + CREATE), so lockout travels in its own
-- SECURITY DEFINER seam with the same search_path discipline. Bounds live
-- in readers, not here; missing or non-numeric rows fall back to 10/900.
CREATE FUNCTION get_auth_login_lockout_settings() RETURNS TABLE(
  login_lockout_threshold integer,
  login_lockout_duration_seconds integer
)
    LANGUAGE plpgsql STABLE SECURITY DEFINER
    SET search_path TO 'public', 'pg_catalog', 'pg_temp'
    AS $$
BEGIN
  RETURN QUERY
  SELECT
    COALESCE(
      (
        SELECT
          CASE
            WHEN btrim(config.value) ~ '^[0-9]+$' THEN btrim(config.value)::integer
            ELSE NULL
          END
        FROM system_configs AS config
        WHERE config.config_key = 'auth_login_lockout_threshold'
        LIMIT 1
      ),
      10
    ),
    COALESCE(
      (
        SELECT
          CASE
            WHEN btrim(config.value) ~ '^[0-9]+$' THEN btrim(config.value)::integer
            ELSE NULL
          END
        FROM system_configs AS config
        WHERE config.config_key = 'auth_login_lockout_duration_seconds'
        LIMIT 1
      ),
      900
    );
END;
$$;

REVOKE ALL ON FUNCTION get_auth_login_lockout_settings() FROM PUBLIC;

-- Bind the new privileged routine to the installed schema, mirroring the
-- bootstrap hardening block: every SECURITY DEFINER routine resolves
-- tables through the installing schema first. The baseline-era generic
-- block only covers routines that existed at baseline time, so post
-- baseline routines carry their own binding (see identity.sql).
DO $$
DECLARE trusted_schema TEXT := current_schema();
BEGIN
  EXECUTE format(
    'ALTER FUNCTION %I.get_auth_login_lockout_settings() SET search_path TO %I, pg_catalog, pg_temp',
    trusted_schema, trusted_schema
  );
END;
$$;
