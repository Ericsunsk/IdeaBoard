import assert from 'node:assert/strict'
import { describe, it } from 'node:test'

describe('getAuthGuardSettings', () => {
  it('loads only registration settings and keeps auth rate-limit defaults', async (t) => {
    const originalAuthInsecureDev = process.env.AUTH_INSECURE_DEV
    const originalAuthHost = process.env.AUTH_HOST
    process.env.AUTH_INSECURE_DEV = '1'
    process.env.AUTH_HOST = '127.0.0.1'

    const { authDB } = await import('./auth.js')
    const {
      DEFAULT_AUTH_RATE_LIMIT_MAX_REQUESTS,
      DEFAULT_AUTH_RATE_LIMIT_WINDOW_SECONDS,
      getAuthGuardSettings,
    } = await import('./authGuardSettings.js')
    const originalQuery = authDB.query
    let selectedConfigKeys: unknown

    authDB.query = (async (_queryText: unknown, values?: unknown[]) => {
      selectedConfigKeys = values?.[0]
      return {
        rows: [
          { config_key: 'auth_registration_enabled', value: 'true' },
          { config_key: 'auth_rate_limit_window', value: '1' },
          { config_key: 'auth_rate_limit_max_requests', value: '1' },
          { config_key: 'api_rate_limit_window_seconds', value: '1' },
          { config_key: 'api_rate_limit_max_requests', value: '1' },
        ],
      }
    }) as typeof authDB.query

    t.after(() => {
      authDB.query = originalQuery
      if (originalAuthInsecureDev === undefined) {
        delete process.env.AUTH_INSECURE_DEV
      } else {
        process.env.AUTH_INSECURE_DEV = originalAuthInsecureDev
      }
      if (originalAuthHost === undefined) {
        delete process.env.AUTH_HOST
      } else {
        process.env.AUTH_HOST = originalAuthHost
      }
    })

    const settings = await getAuthGuardSettings()

    assert.deepEqual(selectedConfigKeys, ['auth_registration_enabled'])
    assert.deepEqual(settings, {
      registrationEnabled: true,
      windowSeconds: DEFAULT_AUTH_RATE_LIMIT_WINDOW_SECONDS,
      maxRequests: DEFAULT_AUTH_RATE_LIMIT_MAX_REQUESTS,
    })
  })
})
