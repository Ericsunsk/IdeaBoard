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
  assert.equal(queries.at(-1), 'COMMIT')
  assert.equal(
    queries.some((sql) => sql.includes("credit_")),
    false,
  )
})

test("verifyAuthRuntimePrivileges rejects a role that can write Credits", async () => {
  const pool = {
    query: async () => ({
      rows: [
        {
          current_user: 'schema_owner',
          can_read_auth_users: true,
          can_write_credits: true,
          can_execute_credit_functions: true,
        },
      ],
    }),
  } as unknown as Pool

  await assert.rejects(
    () => verifyAuthRuntimePrivileges(pool),
    /Unsafe Auth runtime database role/,
  )
})
