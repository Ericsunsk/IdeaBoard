import type { Pool } from 'pg'

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
  'execute_credit_command(text,bigint,bigint,text,text,text,text,text,text,text,text)',
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
}

export async function verifyAuthRuntimePrivileges(pool: Pool) {
  const result = await pool.query<{
    can_execute_credit_functions: boolean;
    can_write_credits: boolean;
    can_read_auth_users: boolean;
    current_user: string;
  }>(
    `
SELECT current_user,
       has_table_privilege(current_user, '"user"', 'SELECT') AS can_read_auth_users,
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
       ) AS can_execute_credit_functions
`,
    [creditTables, creditFunctions],
  )
  const privileges = result.rows[0]
  if (
    !privileges?.can_read_auth_users ||
    privileges.can_write_credits ||
    privileges.can_execute_credit_functions
  ) {
    throw new Error(
      `Unsafe Auth runtime database role ${JSON.stringify(privileges?.current_user || '')}: ` +
        `read_auth_users=${Boolean(privileges?.can_read_auth_users)} ` +
        `write_credits=${Boolean(privileges?.can_write_credits)} ` +
        `execute_credit_functions=${Boolean(privileges?.can_execute_credit_functions)}`,
    )
  }
}

function quoteIdentifier(value: string) {
  return `"${value.replaceAll('"', '""')}"`
}
