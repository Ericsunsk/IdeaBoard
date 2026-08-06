import type { Pool } from "pg";

export async function ensureSocialProviderSettingsSchema(pool: Pool) {
  await pool.query(`
CREATE TABLE IF NOT EXISTS social_provider_settings (
  provider VARCHAR(32) PRIMARY KEY CHECK (provider = lower(btrim(provider)) AND provider ~ '^[a-z][a-z0-9_-]{0,31}$'),
  enabled BOOLEAN NOT NULL DEFAULT FALSE,
  client_id TEXT NOT NULL DEFAULT '',
  encrypted_client_secret TEXT NOT NULL DEFAULT '',
  scopes JSONB NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(scopes) = 'array'),
  config_version BIGINT NOT NULL CHECK (config_version > 0),
  updated_at TIMESTAMPTZ(3) NOT NULL DEFAULT clock_timestamp()
);
`);
}

export async function configureSocialProviderSettingsRuntimePrivileges(
  pool: Pool,
  runtimeRole: string,
) {
  const normalizedRole = runtimeRole.trim();
  if (!normalizedRole) throw new Error("Auth runtime role is required");
  await pool.query(
    `GRANT SELECT, INSERT, UPDATE ON social_provider_settings TO ${quoteIdentifier(normalizedRole)}`,
  );
}

function quoteIdentifier(value: string) {
  return `"${value.replaceAll('"', '""')}"`;
}
