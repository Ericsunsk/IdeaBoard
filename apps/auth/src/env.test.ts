import assert from 'node:assert/strict'
import { after, before, describe, it } from 'node:test'

type EnvSource = Record<string, string | undefined>
type LoadEnv = typeof import('./env.js')['loadEnv']

let loadEnv: LoadEnv

const moduleEnvKeys = [
  'APP_ENV',
  'AUTH_HOST',
  'AUTH_INSECURE_DEV',
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
  process.env.AUTH_INSECURE_DEV = '1'
  process.env.AUTH_HOST = '127.0.0.1'
  process.env.AUTH_INTERNAL_KEY = ''
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

function productionSource(overrides: EnvSource = {}): EnvSource {
  return {
    NODE_ENV: 'production',
    AUTH_INTERNAL_KEY: strongInternalKey,
    BETTER_AUTH_SECRET: strongAuthSecret,
    BETTER_AUTH_URL: 'https://auth.example.com',
    AUTH_TRUSTED_ORIGINS: 'https://app.example.com',
    DB_PASSWORD: 'db-prod-value-123',
    DB_SSLMODE: 'require',
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

  it('requires production BETTER_AUTH_URL to be a valid HTTPS URL', () => {
    for (const value of ['not-a-url', 'http://auth.example.com', 'ftp://auth.example.com']) {
      assert.throws(
        () => loadEnv(productionSource({ BETTER_AUTH_URL: value })),
        /BETTER_AUTH_URL must be a valid https URL when NODE_ENV=production/,
      )
    }
  })

  it('rejects invalid production AUTH_TRUSTED_ORIGINS entries', () => {
    assert.throws(
      () =>
        loadEnv(productionSource({
          AUTH_TRUSTED_ORIGINS: 'https://app.example.com,not-a-url',
        })),
      /AUTH_TRUSTED_ORIGINS entry must be a valid http\(s\) URL when NODE_ENV=production/,
    )
  })

  it('does not force HTTPS for production AUTH_TRUSTED_ORIGINS entries', () => {
    const config = loadEnv(
      productionSource({
        AUTH_TRUSTED_ORIGINS: 'http://app.example.com, https://admin.example.com',
      }),
    )

    assert.deepEqual(config.auth.trustedOrigins, [
      'http://app.example.com',
      'https://admin.example.com',
    ])
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

describe('loadEnv AUTH_INSECURE_DEV guard', () => {
  it('allows empty AUTH_INTERNAL_KEY only in non-production loopback mode', () => {
    const config = loadEnv({
      NODE_ENV: 'development',
      AUTH_INSECURE_DEV: '1',
      AUTH_HOST: 'localhost',
      AUTH_INTERNAL_KEY: '',
    })

    assert.equal(config.auth.allowInsecureInternalBypass, true)
    assert.equal(config.auth.internalKey, '')
  })

  it('rejects AUTH_INSECURE_DEV bypass in production', () => {
    assert.throws(
      () =>
        loadEnv(productionSource({
          AUTH_INSECURE_DEV: '1',
          AUTH_HOST: '127.0.0.1',
          AUTH_INTERNAL_KEY: '',
        })),
      /AUTH_INTERNAL_KEY/,
    )
  })

  it('rejects AUTH_INSECURE_DEV bypass on non-loopback hosts', () => {
    assert.throws(
      () => loadEnv({
        NODE_ENV: 'development',
        AUTH_INSECURE_DEV: '1',
        AUTH_HOST: '0.0.0.0',
        AUTH_INTERNAL_KEY: '',
      }),
      /AUTH_INTERNAL_KEY is required unless AUTH_INSECURE_DEV=1 and AUTH_HOST is loopback/,
    )
  })
})
