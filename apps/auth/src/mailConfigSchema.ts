import type { Pool } from "pg";

export async function ensureMailConfigSchema(pool: Pool) {
  await pool.query(`
CREATE TABLE IF NOT EXISTS auth_mail_settings (
  id SMALLINT PRIMARY KEY CHECK (id = 1),
  encrypted_api_key TEXT NOT NULL,
  from_name VARCHAR(100) NOT NULL CHECK (char_length(btrim(from_name)) BETWEEN 1 AND 100),
  from_email VARCHAR(320) NOT NULL CHECK (from_email = lower(btrim(from_email))),
  reply_to VARCHAR(320) NOT NULL DEFAULT '' CHECK (reply_to = lower(btrim(reply_to))),
  config_version BIGINT NOT NULL CHECK (config_version > 0),
  readiness VARCHAR(24) NOT NULL CHECK (readiness IN ('not_configured', 'not_tested', 'ready', 'unavailable')),
  updated_at TIMESTAMPTZ(3) NOT NULL DEFAULT clock_timestamp(),
  last_tested_at TIMESTAMPTZ(3),
  last_test_error_code VARCHAR(64)
);

CREATE TABLE IF NOT EXISTS auth_mail_settings_audit (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  actor_id BIGINT NOT NULL CHECK (actor_id > 0),
  action VARCHAR(40) NOT NULL CHECK (action IN ('create_mail_config', 'update_mail_config', 'delete_mail_config', 'test_mail_success', 'test_mail_failure')),
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(metadata) = 'object'),
  created_at TIMESTAMPTZ(3) NOT NULL DEFAULT clock_timestamp()
);
`);
  await pool.query(`
CREATE OR REPLACE FUNCTION prevent_auth_mail_settings_audit_mutation()
RETURNS TRIGGER AS $$
BEGIN
  RAISE EXCEPTION USING ERRCODE = 'IB401', MESSAGE = 'auth mail settings audit is append-only';
END;
$$ LANGUAGE plpgsql SET search_path = pg_catalog;
`);
  await pool.query(`
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_trigger WHERE tgname = 'auth_mail_settings_audit_append_only'
  ) THEN
    CREATE TRIGGER auth_mail_settings_audit_append_only
    BEFORE UPDATE OR DELETE ON auth_mail_settings_audit
    FOR EACH ROW EXECUTE FUNCTION prevent_auth_mail_settings_audit_mutation();
  END IF;
END $$;
`);
}

export async function configureMailConfigRuntimePrivileges(
  pool: Pool,
  runtimeRole: string,
) {
  const role = quoteIdentifier(runtimeRole.trim());
  if (role === '""') throw new Error("Auth runtime role is required");
  await pool.query(
    `GRANT SELECT, INSERT, UPDATE, DELETE ON auth_mail_settings TO ${role}`,
  );
  await pool.query(
    `GRANT SELECT, INSERT ON auth_mail_settings_audit TO ${role}`,
  );
  await pool.query(
    `GRANT USAGE, SELECT ON SEQUENCE auth_mail_settings_audit_id_seq TO ${role}`,
  );
}

export async function verifyMailConfigRuntimePrivileges(pool: Pool) {
  const result = await pool.query<{
    can_manage_settings: boolean;
    can_insert_audit: boolean;
    can_mutate_audit: boolean;
  }>(`
SELECT
  has_table_privilege(current_user, 'auth_mail_settings', 'SELECT,INSERT,UPDATE,DELETE') AS can_manage_settings,
  has_table_privilege(current_user, 'auth_mail_settings_audit', 'SELECT,INSERT') AS can_insert_audit,
  has_table_privilege(current_user, 'auth_mail_settings_audit', 'UPDATE')
    OR has_table_privilege(current_user, 'auth_mail_settings_audit', 'DELETE')
    OR has_table_privilege(current_user, 'auth_mail_settings_audit', 'TRUNCATE') AS can_mutate_audit
`);
  const row = result.rows[0];
  if (
    !row?.can_manage_settings ||
    !row.can_insert_audit ||
    row.can_mutate_audit
  ) {
    throw new Error(
      `Unsafe Auth mail config privileges: manage_settings=${Boolean(row?.can_manage_settings)} insert_audit=${Boolean(row?.can_insert_audit)} mutate_audit=${Boolean(row?.can_mutate_audit)}`,
    );
  }
}

function quoteIdentifier(value: string) {
  return `"${value.replaceAll('"', '""')}"`;
}
