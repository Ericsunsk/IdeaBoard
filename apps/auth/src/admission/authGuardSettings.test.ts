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
    assert.match(settingsQuery, /get_auth_login_lockout_settings\(\)/)
    assert.doesNotMatch(settingsQuery, /system_configs/)
    assert.deepEqual(settings, {
      registrationEnabled: true,
      projectName: 'Design Studio',
      logoUrl: '/uploads/site/assets/logo.png',
      windowSeconds: DEFAULT_AUTH_RATE_LIMIT_WINDOW_SECONDS,
      maxRequests: DEFAULT_AUTH_RATE_LIMIT_MAX_REQUESTS,
      lockoutThreshold: 10,
      lockoutDurationSeconds: 900,
    })
  })

  it('loads configured login-lockout budgets within bounds', async () => {
    const { createAuthGuardSettingsReader } = await import('./authGuardSettings.js')
    const getAuthGuardSettings = createAuthGuardSettingsReader({
      query: async () => ({
        rows: [
          {
            registration_enabled: false,
            login_lockout_threshold: 5,
            login_lockout_duration_seconds: 300,
          },
        ],
      }),
    })

    const settings = await getAuthGuardSettings()

    assert.equal(settings.lockoutThreshold, 5)
    assert.equal(settings.lockoutDurationSeconds, 300)
  })

  it('falls back to login-lockout defaults on missing or out-of-range values', async () => {
    const { createAuthGuardSettingsReader } = await import('./authGuardSettings.js')
    for (const row of [
      {},
      { login_lockout_threshold: 2, login_lockout_duration_seconds: 59 },
      { login_lockout_threshold: 101, login_lockout_duration_seconds: 86401 },
      { login_lockout_threshold: 'ten', login_lockout_duration_seconds: 7.5 },
      { login_lockout_threshold: null, login_lockout_duration_seconds: undefined },
    ]) {
      const getAuthGuardSettings = createAuthGuardSettingsReader(
        {
          query: async () => ({ rows: [row] }),
        },
        { onError: () => {} },
      )
      const settings = await getAuthGuardSettings()
      assert.equal(settings.lockoutThreshold, 10)
      assert.equal(settings.lockoutDurationSeconds, 900)
    }
  })

  it('falls back to login-lockout defaults when the lockout seam is missing', async () => {
    const { createAuthGuardSettingsReader } = await import('./authGuardSettings.js')
    const getAuthGuardSettings = createAuthGuardSettingsReader(
      {
        query: async () => {
          throw new Error('function get_auth_login_lockout_settings() does not exist')
        },
      },
      { onError: () => {} },
    )

    const settings = await getAuthGuardSettings()

    assert.equal(settings.lockoutThreshold, 10)
    assert.equal(settings.lockoutDurationSeconds, 900)
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
