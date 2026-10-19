import { describe, it } from 'node:test'
import assert from 'node:assert/strict'
import { loadEnv, type EnvSource } from './env.js'

const BASE_SECRETS: EnvSource = {
  REALTIME_TICKET_SECRET: 'test-ticket-secret-0123456789abcdef',
  REALTIME_INTERNAL_KEY: 'test-internal-key-0123456789abcdef',
}

function productionSource(overrides: EnvSource = {}): EnvSource {
  return { NODE_ENV: 'production', ...BASE_SECRETS, ...overrides }
}

describe('loadEnv', () => {
  it('loads defaults for host, port, and api url', () => {
    const config = loadEnv({ ...BASE_SECRETS })
    assert.equal(config.realtime.host, '127.0.0.1')
    assert.equal(config.realtime.port, 6002)
    assert.equal(config.realtime.apiInternalURL, 'http://127.0.0.1:6000')
  })

  it('requires both secrets', () => {
    assert.throws(
      () => loadEnv({ REALTIME_INTERNAL_KEY: 'x' }),
      /REALTIME_TICKET_SECRET is required/,
    )
    assert.throws(
      () => loadEnv({ REALTIME_TICKET_SECRET: 'x' }),
      /REALTIME_INTERNAL_KEY is required/,
    )
  })

  it('rejects placeholder or short production secrets', () => {
    assert.throws(
      () => loadEnv(productionSource({ REALTIME_TICKET_SECRET: 'replace-with-secret' })),
      /non-placeholder value/,
    )
    assert.throws(
      () => loadEnv(productionSource({ REALTIME_TICKET_SECRET: 'short-but-safe' })),
      /at least 32 characters long/,
    )
    assert.throws(
      () => loadEnv(productionSource({ REALTIME_INTERNAL_KEY: 'short-but-safe' })),
      /REALTIME_INTERNAL_KEY must be at least 32 characters long/,
    )
  })

  it('rejects an invalid production api url', () => {
    assert.throws(
      () => loadEnv(productionSource({ API_INTERNAL_URL: 'not-a-url' })),
      /API_INTERNAL_URL must be a valid http\(s\) URL/,
    )
  })

  it('defaults the Y collaboration knobs and parses overrides', () => {
    const defaults = loadEnv({ ...BASE_SECRETS })
    assert.equal(defaults.realtime.yCompactEvery, 50)
    assert.equal(defaults.realtime.yPersistRetries, 5)
    const tuned = loadEnv({
      ...BASE_SECRETS,
      REALTIME_Y_COMPACT_EVERY: '10',
      REALTIME_Y_PERSIST_RETRIES: '2',
    })
    assert.equal(tuned.realtime.yCompactEvery, 10)
    assert.equal(tuned.realtime.yPersistRetries, 2)
    const invalid = loadEnv({
      ...BASE_SECRETS,
      REALTIME_Y_COMPACT_EVERY: 'bogus',
    })
    assert.equal(invalid.realtime.yCompactEvery, 50)
  })
})
