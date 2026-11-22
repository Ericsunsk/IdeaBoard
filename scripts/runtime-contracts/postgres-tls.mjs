import assert from 'node:assert/strict'
import { createAuthPool } from '/app/dist/application/auth.js'
import { loadEnv } from '/app/dist/infrastructure/env.js'
const pool = createAuthPool(loadEnv(process.env).database)
try {
  const r = await pool.query('SELECT ssl, version FROM pg_stat_ssl WHERE pid = pg_backend_pid()')
  assert.equal(r.rows[0].ssl, true)
  const client = await pool.connect()
  assert.equal(client.connection.stream.authorized, true)
  await client.query('BEGIN')
  await client.query('SELECT 1')
  await client.query('ROLLBACK')
  client.release()
  if (process.env.EXPECT_REJECT === '1') throw new Error('UNSAFE_TLS_ACCEPTED')
  console.log('PG_TLS_VERIFIED', JSON.stringify(r.rows[0]))
} catch (e) {
  if (process.env.EXPECT_REJECT !== '1') throw e
  assert.match(String(e.code) + ' ' + e.message, /CERT_|certificate|altname|Hostname/)
  console.log('PG_TLS_REJECTED', e.code, e.message)
} finally {
  await pool.end()
}
