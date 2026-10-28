import assert from 'node:assert/strict'
import { test } from 'node:test'
import { ensureAccountIssuerRemoved } from './authAccountSchema.js'

test('drops the compound unique index before removing the issuer column', async () => {
  const queries: string[] = []
  const pool = {
    async query(text: string) {
      queries.push(text)
      return { rows: [] }
    },
  }
  await ensureAccountIssuerRemoved(pool as never)

  assert.deepEqual(queries, [
    'DROP INDEX IF EXISTS "account_issuer_accountId_uidx"',
    'ALTER TABLE account DROP COLUMN IF EXISTS issuer',
  ])
})
