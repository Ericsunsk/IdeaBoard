import assert from 'node:assert/strict'
import { describe, it } from 'node:test'

describe('getAuthGuardSettings', () => {
  it('loads only registration settings and keeps auth rate-limit defaults', async (t) => {
    const originalAuthInternalKey = process.env.AUTH_INTERNAL_KEY
    const originalAuthHost = process.env.AUTH_HOST
    process.env.AUTH_INTERNAL_KEY = 'test-internal-key'
    process.env.AUTH_HOST = '127.0.0.1'

    const { authDB } = await import('./auth.js')
    const {
      DEFAULT_AUTH_RATE_LIMIT_MAX_REQUESTS,
      DEFAULT_AUTH_RATE_LIMIT_WINDOW_SECONDS,
      getAuthGuardSettings,
    } = await import('./authGuardSettings.js')
    const originalQuery = authDB.query
    let settingsQuery = ''

    authDB.query = (async (queryText: unknown) => {
      settingsQuery = String(queryText)
      return {
        rows: [{ registration_enabled: true }],
      }
    }) as typeof authDB.query

    t.after(() => {
      authDB.query = originalQuery
      if (originalAuthInternalKey === undefined) {
        delete process.env.AUTH_INTERNAL_KEY
      } else {
        process.env.AUTH_INTERNAL_KEY = originalAuthInternalKey
      }
      if (originalAuthHost === undefined) {
        delete process.env.AUTH_HOST
      } else {
        process.env.AUTH_HOST = originalAuthHost
      }
    })

    const settings = await getAuthGuardSettings()

    assert.match(settingsQuery, /get_auth_runtime_settings\(\)/)
    assert.doesNotMatch(settingsQuery, /system_configs/)
    assert.deepEqual(settings, {
      registrationEnabled: true,
      windowSeconds: DEFAULT_AUTH_RATE_LIMIT_WINDOW_SECONDS,
      maxRequests: DEFAULT_AUTH_RATE_LIMIT_MAX_REQUESTS,
    })
  })
})
