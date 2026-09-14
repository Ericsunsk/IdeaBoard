import assert from 'node:assert/strict'
import { after, before, describe, it } from 'node:test'

type EnvSource = Record<string, string | undefined>
type LoadEnv = typeof import('./env.js')['loadEnv']

let loadEnv: LoadEnv

const moduleEnvKeys = [
  'APP_ENV',
  'AUTH_HOST',
  'AUTH_INTERNAL_KEY',
  'DB_SSLMODE',
  'NODE_ENV',
] as const
const originalModuleEnv = new Map<string, string | undefined>()

before(async () => {
  for (const key of moduleEnvKeys) {
    originalModuleEnv.set(key, process.env[key])
  }
  process.env.NODE_ENV = 'test'
  process.env.APP_ENV = ''
  process.env.AUTH_HOST = '127.0.0.1'
  process.env.AUTH_INTERNAL_KEY = 'test-internal-key'
  process.env.DB_SSLMODE = 'disable'

  const envModule = await import('./env.js')
  loadEnv = envModule.loadEnv
})

after(() => {
  for (const key of moduleEnvKeys) {
    const originalValue = originalModuleEnv.get(key)
    if (originalValue === undefined) {
      delete process.env[key]
    } else {
      process.env[key] = originalValue
    }
  }
})

const strongInternalKey = 'a'.repeat(32)
const strongAuthSecret = 'b'.repeat(32)
const strongSecretsEncryptionKey = Buffer.alloc(32, 9).toString('base64')

function productionSource(overrides: EnvSource = {}): EnvSource {
  return {
    NODE_ENV: 'production',
    AUTH_INTERNAL_KEY: strongInternalKey,
    BETTER_AUTH_SECRET: strongAuthSecret,
    BETTER_AUTH_URL: 'https://auth.example.com',
    AUTH_TRUSTED_ORIGINS: 'https://app.example.com',
    DB_PASSWORD: 'db-prod-value-123',
    DB_SSLMODE: 'require',
    APP_SECRETS_ENCRYPTION_KEY: strongSecretsEncryptionKey,
    ...overrides,
  }
}

describe('loadEnv production guard', () => {
  it('loads valid production values from an injected source', () => {
    const config = loadEnv(
      productionSource({
        AUTH_TRUSTED_ORIGINS: 'https://app.example.com, https://admin.example.com',
        DB_SSLMODE: 'verify-full',
      }),
    )

    assert.equal(config.auth.internalKey, strongInternalKey)
    assert.equal(config.auth.secret, strongAuthSecret)
    assert.equal(config.auth.baseURL, 'https://auth.example.com')
    assert.deepEqual(config.auth.trustedOrigins, [
      'https://app.example.com',
      'https://admin.example.com',
    ])
    assert.equal(config.database.sslMode, 'verify-full')
  })

  it('derives auth URL and trusted origins from PUBLIC_ORIGIN', () => {
    const config = loadEnv(
      productionSource({
        PUBLIC_ORIGIN: 'https://app.example.com',
        BETTER_AUTH_URL: undefined,
        AUTH_TRUSTED_ORIGINS: undefined,
      }),
    )

    assert.equal(config.auth.baseURL, 'https://app.example.com')
    assert.deepEqual(config.auth.trustedOrigins, ['https://app.example.com'])
  })

  it('uses BETTER_AUTH_URL as the final trusted-origin fallback', () => {
    const config = loadEnv(
      productionSource({
        PUBLIC_ORIGIN: undefined,
        AUTH_TRUSTED_ORIGINS: undefined,
      }),
    )

    assert.deepEqual(config.auth.trustedOrigins, ['https://auth.example.com'])
  })

  for (const field of ['AUTH_INTERNAL_KEY', 'BETTER_AUTH_SECRET', 'DB_PASSWORD'] as const) {
    for (const value of [
      '',
      'placeholder',
      'changeme',
      'password',
      'secret',
      'replace-with-real-value',
    ]) {
      it(`rejects unsafe production ${field} value ${value || '<empty>'}`, () => {
        assert.throws(() => loadEnv(productionSource({ [field]: value })), new RegExp(field))
      })
    }
  }

  it('rejects short production AUTH_INTERNAL_KEY values', () => {
    assert.throws(
      () => loadEnv(productionSource({ AUTH_INTERNAL_KEY: 'short-but-safe' })),
      /AUTH_INTERNAL_KEY must be at least 32 characters long/,
    )
  })

  it('rejects short production BETTER_AUTH_SECRET values', () => {
    assert.throws(
      () => loadEnv(productionSource({ BETTER_AUTH_SECRET: 'short-but-safe' })),
      /BETTER_AUTH_SECRET must be at least 32 characters long/,
    )
  })

  it('requires a valid production APP_SECRETS_ENCRYPTION_KEY', () => {
    for (const value of ['', 'placeholder', Buffer.alloc(31, 9).toString('base64')]) {
      assert.throws(
        () => loadEnv(productionSource({ APP_SECRETS_ENCRYPTION_KEY: value })),
        /APP_SECRETS_ENCRYPTION_KEY must be base64 that decodes to exactly 32 bytes/,
      )
    }
  })

  it('requires production auth origin to be a valid HTTPS URL', () => {
    for (const value of ['not-a-url', 'http://auth.example.com', 'ftp://auth.example.com']) {
      assert.throws(
        () => loadEnv(productionSource({ BETTER_AUTH_URL: value })),
        /BETTER_AUTH_URL must be a valid https URL when NODE_ENV=production/,
      )
    }
  })

  it('requires BETTER_AUTH_URL or PUBLIC_ORIGIN in production', () => {
    assert.throws(
      () => loadEnv(productionSource({ BETTER_AUTH_URL: undefined, PUBLIC_ORIGIN: undefined })),
      /BETTER_AUTH_URL or PUBLIC_ORIGIN is required when NODE_ENV=production/,
    )
  })

  it('rejects invalid production AUTH_TRUSTED_ORIGINS entries', () => {
    for (const value of [
      'https://app.example.com,not-a-url',
      'https://app.example.com/path',
      'https://user:pass@app.example.com',
      'https://app.example.com?query=1',
    ]) {
      assert.throws(
        () => loadEnv(productionSource({ AUTH_TRUSTED_ORIGINS: value })),
        /AUTH_TRUSTED_ORIGINS entry must be a valid https URL when NODE_ENV=production/,
      )
    }
  })

  it('requires HTTPS for production AUTH_TRUSTED_ORIGINS entries', () => {
    assert.throws(
      () => loadEnv(productionSource({ AUTH_TRUSTED_ORIGINS: 'http://app.example.com' })),
      /AUTH_TRUSTED_ORIGINS entry must be a valid https URL when NODE_ENV=production/,
    )
  })

  it('accepts only allowlisted DB_SSLMODE values', () => {
    for (const value of ['disable', 'require', 'verify-full']) {
      assert.equal(loadEnv(productionSource({ DB_SSLMODE: value })).database.sslMode, value)
    }

    assert.throws(
      () => loadEnv(productionSource({ DB_SSLMODE: 'prefer' })),
      /DB_SSLMODE must be one of disable, require, or verify-full/,
    )
  })
})

describe('loadEnv internal authentication guard', () => {
  it('requires AUTH_INTERNAL_KEY in development', () => {
    assert.throws(
      () => loadEnv({ NODE_ENV: 'development', AUTH_INTERNAL_KEY: '' }),
      /AUTH_INTERNAL_KEY is required/,
    )
  })
})
