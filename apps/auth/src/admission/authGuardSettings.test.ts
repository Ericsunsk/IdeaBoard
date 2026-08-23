import assert from 'node:assert/strict'
import { describe, it } from 'node:test'

describe('getAuthGuardSettings', () => {
  it('loads only registration settings and keeps auth rate-limit defaults', async () => {
    const {
      createAuthGuardSettingsReader,
      DEFAULT_AUTH_RATE_LIMIT_MAX_REQUESTS,
      DEFAULT_AUTH_RATE_LIMIT_WINDOW_SECONDS,
    } = await import('./authGuardSettings.js')
    let settingsQuery = ''
    const getAuthGuardSettings = createAuthGuardSettingsReader({
      query: async (queryText) => {
        settingsQuery = queryText
        return {
          rows: [
            {
              registration_enabled: true,
              branding_project_name: 'Design Studio',
              branding_logo_url: '/uploads/site/assets/logo.png',
            },
          ],
        }
      },
    })

    const settings = await getAuthGuardSettings()

    assert.match(settingsQuery, /get_auth_runtime_settings\(\)/)
    assert.doesNotMatch(settingsQuery, /system_configs/)
    assert.deepEqual(settings, {
      registrationEnabled: true,
      projectName: 'Design Studio',
      logoUrl: '/uploads/site/assets/logo.png',
      windowSeconds: DEFAULT_AUTH_RATE_LIMIT_WINDOW_SECONDS,
      maxRequests: DEFAULT_AUTH_RATE_LIMIT_MAX_REQUESTS,
    })
  })

  it('keeps fail-closed cache state isolated per Runtime reader', async () => {
    const { createAuthGuardSettingsReader } = await import('./authGuardSettings.js')
    let firstQueries = 0
    const first = createAuthGuardSettingsReader(
      {
        query: async () => {
          firstQueries += 1
          throw new Error('database unavailable')
        },
      },
      { now: () => 1_000, onError: () => {} },
    )
    let secondQueries = 0
    const second = createAuthGuardSettingsReader(
      {
        query: async () => {
          secondQueries += 1
          return { rows: [{ registration_enabled: true }] }
        },
      },
      { now: () => 1_000 },
    )

    assert.equal((await first()).registrationEnabled, false)
    assert.equal((await first()).registrationEnabled, false)
    assert.equal((await second()).registrationEnabled, true)
    assert.equal(firstQueries, 1)
    assert.equal(secondQueries, 1)
  })
})
