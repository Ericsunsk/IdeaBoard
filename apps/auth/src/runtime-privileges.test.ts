import assert from 'node:assert/strict'
import test from 'node:test'
import type { Pool } from 'pg'
import {
  configureAuthRuntimePrivileges,
  verifyAuthRuntimePrivileges,
} from './runtime-privileges.js'

test("configureAuthRuntimePrivileges grants only Better Auth tables atomically", async () => {
  const queries: string[] = []
  const client = {
    async query(sql: string) {
      queries.push(sql)
      if (sql === 'SELECT current_schema()') {
        return { rows: [{ current_schema: 'public' }] }
      }
      return { rows: [] }
    },
    release() {},
  };
  const pool = { connect: async () => client } as unknown as Pool

  await configureAuthRuntimePrivileges(pool, 'ideaboard_auth')

  assert.equal(queries[0], 'BEGIN')
  assert.equal(queries[1], 'SELECT current_schema()')
  assert.match(queries[2] || "", /GRANT USAGE ON SCHEMA "public"/);
  assert.match(
    queries[3] || "",
    /"user", "session", "account", "verification"/,
  )
  assert.match(
    queries[4] || '',
    /REVOKE ALL ON TABLE "public"\.system_configs FROM "ideaboard_auth"/,
  )
  assert.match(
    queries[5] || '',
    /GRANT EXECUTE ON FUNCTION "public"\.get_auth_runtime_settings\(\) TO "ideaboard_auth"/,
  )
  assert.equal(queries.at(-1), 'COMMIT')
  assert.equal(
    queries.some((sql) => sql.includes("credit_")),
    false,
  )
})

test("verifyAuthRuntimePrivileges rejects a role that can write Credits", async () => {
  let inspectedFunctions: unknown
  const pool = {
    query: async (_sql: string, values: unknown[]) => {
      inspectedFunctions = values[1]
      return {
        rows: [
          {
            current_user: 'schema_owner',
            can_execute_auth_settings: true,
            can_read_system_configs: false,
            can_read_auth_users: true,
            can_write_credits: true,
            can_execute_credit_functions: true,
          },
        ],
      }
    },
  } as unknown as Pool

  await assert.rejects(
    () => verifyAuthRuntimePrivileges(pool),
    /Unsafe Auth runtime database role/,
  )
  assert.ok(Array.isArray(inspectedFunctions))
  assert.ok(
    inspectedFunctions.includes(
      'execute_credit_command_v2(text,bigint,bigint,text,text,text,text,text,text,text,text)',
    ),
  )
  assert.equal(
    inspectedFunctions.some((signature) => String(signature).startsWith('execute_credit_command(')),
    false,
  )
})

test('verifyAuthRuntimePrivileges rejects a role without the typed Auth settings seam', async () => {
  const pool = {
    query: async () => ({
      rows: [
        {
          current_user: 'ideaboard_auth',
          can_execute_auth_settings: false,
          can_read_system_configs: false,
          can_read_auth_users: true,
          can_write_credits: false,
          can_execute_credit_functions: false,
        },
      ],
    }),
  } as unknown as Pool

  await assert.rejects(
    () => verifyAuthRuntimePrivileges(pool),
    /execute_auth_settings=false/,
  )
})

test('verifyAuthRuntimePrivileges rejects direct access to raw system settings', async () => {
  const pool = {
    query: async () => ({
      rows: [
        {
          current_user: 'ideaboard_auth',
          can_execute_auth_settings: true,
          can_read_system_configs: true,
          can_read_auth_users: true,
          can_write_credits: false,
          can_execute_credit_functions: false,
        },
      ],
    }),
  } as unknown as Pool

  await assert.rejects(
    () => verifyAuthRuntimePrivileges(pool),
    /read_system_configs=true/,
  )
})

test('verifyAuthRuntimePrivileges accepts the limited Auth runtime role', async () => {
  const pool = {
    query: async () => ({
      rows: [
        {
          current_user: 'ideaboard_auth',
          can_execute_auth_settings: true,
          can_read_system_configs: false,
          can_read_auth_users: true,
          can_write_credits: false,
          can_execute_credit_functions: false,
        },
      ],
    }),
  } as unknown as Pool

  await verifyAuthRuntimePrivileges(pool)
})
