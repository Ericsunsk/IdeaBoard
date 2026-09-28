import assert from 'node:assert/strict'
import test from 'node:test'
import type { Pool } from 'pg'
import { cleanupUnverifiedUsers } from './authCleanup.js'

test('cleanupUnverifiedUsers deletes only the aged unverified user set transactionally', async () => {
  const queries: Array<{ sql: string; values?: unknown[] }> = []
  const client = {
    async query(sql: string, values?: unknown[]) {
      queries.push({ sql, values })
      if (sql.includes('RETURNING id')) return { rows: [{ id: 'user-1' }, { id: 'user-2' }] }
      return { rows: [] }
    },
    release() {},
  }

  const deleted = await cleanupUnverifiedUsers({
    connect: async () => client,
  } as unknown as Pool)

  assert.equal(deleted, 2)
  assert.equal(queries[0]?.sql, 'BEGIN')
  assert.match(queries[1]?.sql ?? '', /"emailVerified" = FALSE/)
  assert.match(queries[1]?.sql ?? '', /"createdAt" < clock_timestamp\(\)/)
  assert.deepEqual(queries[1]?.values, [24])
  assert.equal(queries[2]?.sql, 'COMMIT')
})
