import type { Pool } from 'pg'

// Remove the obsolete Better Auth 1.7.0–1.7.2 account issuer schema.
export async function ensureAccountIssuerRemoved(pool: Pool): Promise<void> {
  // Preserve the mixed-case index name; unquoted identifiers fold to lowercase.
  await pool.query('DROP INDEX IF EXISTS "account_issuer_accountId_uidx"')
  await pool.query('ALTER TABLE account DROP COLUMN IF EXISTS issuer')
}
