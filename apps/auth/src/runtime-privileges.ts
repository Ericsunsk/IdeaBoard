import type { Pool } from 'pg'
import {
  configureMailConfigRuntimePrivileges,
} from './mailConfigSchema.js'

const authTables = ['user', 'session', 'account', 'verification'] as const
const creditTables = [
  'credit_wallets',
  'credit_accounts',
  'credit_transactions',
  'credit_postings',
  'credit_holds',
  'credit_charges',
] as const
const creditFunctions = [
  'execute_credit_command_v2(text,bigint,bigint,text,text,text,text,text,text,text,text)',
  'prevent_credit_ledger_mutation()',
  'guard_credit_account_projection()',
  'apply_credit_posting_balances()',
  'validate_credit_transaction_balance()',
  'guard_credit_posting_transaction_scope()',
] as const

export async function configureAuthRuntimePrivileges(
  pool: Pool,
  runtimeRole: string,
) {
  const normalizedRole = runtimeRole.trim();
  if (!normalizedRole) {
    throw new Error('Auth runtime role is required')
  }
  const roleSQL = quoteIdentifier(normalizedRole)
  const tableSQL = authTables.map(quoteIdentifier).join(', ')
  const client = await pool.connect()
  try {
    await client.query('BEGIN')
    const schemaResult = await client.query<{ current_schema: string }>(
      'SELECT current_schema()',
    )
    const schemaName = schemaResult.rows[0]?.current_schema
    if (!schemaName) {
      throw new Error('Could not resolve Auth database schema')
    }
    await client.query(
      `GRANT USAGE ON SCHEMA ${quoteIdentifier(schemaName)} TO ${roleSQL}`,
    )
    await client.query(
      `GRANT SELECT, INSERT, UPDATE, DELETE ON ${tableSQL} TO ${roleSQL}`,
    )
    await client.query(
      `REVOKE ALL ON TABLE ${quoteIdentifier(schemaName)}.system_configs FROM ${roleSQL}`,
    )
    await client.query(
      `GRANT EXECUTE ON FUNCTION ${quoteIdentifier(schemaName)}.get_auth_runtime_settings() TO ${roleSQL}`,
    )
    await client.query('COMMIT')
  } catch (error) {
    try {
      await client.query('ROLLBACK')
    } catch (rollbackError) {
      throw new AggregateError(
        [error, rollbackError],
        'Auth runtime privilege transaction failed',
      )
    }
    throw error
  } finally {
    client.release()
  }
  if (typeof (pool as Pool & { query?: unknown }).query === 'function') {
    await configureMailConfigRuntimePrivileges(pool, normalizedRole)
  }
}

export async function verifyAuthRuntimePrivileges(pool: Pool) {
  const result = await pool.query<{
    can_execute_auth_settings: boolean;
    can_execute_credit_functions: boolean;
    can_read_system_configs: boolean;
    can_write_credits: boolean;
    can_read_auth_users: boolean;
    can_manage_mail_settings?: boolean;
    can_insert_mail_audit?: boolean;
    can_mutate_mail_audit?: boolean;
    current_user: string;
  }>(
    `
SELECT current_user,
       has_table_privilege(current_user, '"user"', 'SELECT') AS can_read_auth_users,
       has_table_privilege(current_user, 'system_configs', 'SELECT') AS can_read_system_configs,
       has_function_privilege(current_user, 'get_auth_runtime_settings()', 'EXECUTE') AS can_execute_auth_settings,
       EXISTS (
         SELECT 1
         FROM unnest($1::TEXT[]) AS credit_table(table_name)
         CROSS JOIN unnest(ARRAY['INSERT', 'UPDATE', 'DELETE', 'TRUNCATE', 'TRIGGER']::TEXT[]) AS privilege(privilege_name)
         WHERE has_table_privilege(current_user, credit_table.table_name, privilege.privilege_name)
       ) AS can_write_credits,
       EXISTS (
         SELECT 1
         FROM unnest($2::TEXT[]) AS credit_function(function_name)
         WHERE has_function_privilege(current_user, credit_function.function_name, 'EXECUTE')
       ) AS can_execute_credit_functions,
       has_table_privilege(current_user, 'auth_mail_settings', 'SELECT,INSERT,UPDATE,DELETE') AS can_manage_mail_settings,
       has_table_privilege(current_user, 'auth_mail_settings_audit', 'SELECT,INSERT') AS can_insert_mail_audit,
       has_table_privilege(current_user, 'auth_mail_settings_audit', 'UPDATE')
         OR has_table_privilege(current_user, 'auth_mail_settings_audit', 'DELETE')
         OR has_table_privilege(current_user, 'auth_mail_settings_audit', 'TRUNCATE') AS can_mutate_mail_audit
`,
    [creditTables, creditFunctions],
  )
  const privileges = result.rows[0]
  if (
    !privileges?.can_read_auth_users ||
    !privileges.can_execute_auth_settings ||
    privileges.can_read_system_configs ||
    privileges.can_write_credits ||
    privileges.can_execute_credit_functions ||
    (privileges.can_manage_mail_settings !== undefined &&
      (!privileges.can_manage_mail_settings ||
        !privileges.can_insert_mail_audit ||
        Boolean(privileges.can_mutate_mail_audit)))
  ) {
    throw new Error(
      `Unsafe Auth runtime database role ${JSON.stringify(privileges?.current_user || '')}: ` +
        `read_auth_users=${Boolean(privileges?.can_read_auth_users)} ` +
        `execute_auth_settings=${Boolean(privileges?.can_execute_auth_settings)} ` +
        `read_system_configs=${Boolean(privileges?.can_read_system_configs)} ` +
        `write_credits=${Boolean(privileges?.can_write_credits)} ` +
        `execute_credit_functions=${Boolean(privileges?.can_execute_credit_functions)} ` +
        `manage_mail_settings=${String(privileges?.can_manage_mail_settings)} ` +
        `insert_mail_audit=${String(privileges?.can_insert_mail_audit)} ` +
        `mutate_mail_audit=${String(privileges?.can_mutate_mail_audit)}`,
    )
  }
}

function quoteIdentifier(value: string) {
  return `"${value.replaceAll('"', '""')}"`
}
