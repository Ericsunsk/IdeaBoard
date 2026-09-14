import { isProductionRuntime } from './runtimeEnv.js'
import { isValidSecretEncryptionKey } from './secretCipher.js'
import { normalizeOrigin } from './origin.js'

export type EnvSource = Record<string, string | undefined>
type DatabaseSSLMode = 'disable' | 'require' | 'verify-full'

function getEnv(source: EnvSource, name: string, fallback = ''): string {
  const raw = source[name]
  if (typeof raw !== 'string') return fallback
  return raw.trim()
}

function getEnvInt(source: EnvSource, name: string, fallback: number): number {
  const raw = getEnv(source, name, '')
  if (!raw) return fallback
  const value = Number.parseInt(raw, 10)
  if (!Number.isFinite(value) || value <= 0) return fallback
  return value
}

function isUnsafeProductionSecret(value: string): boolean {
  const normalized = value.trim().toLowerCase()
  if (!normalized) return true
  if (normalized.startsWith('replace-with')) return true
  if (normalized.startsWith('replace_with')) return true
  if (normalized.includes('change-before-production')) return true
  if (normalized.includes('placeholder')) return true
  return (
    normalized === 'changeme' ||
    normalized === 'change-me' ||
    normalized === 'password' ||
    normalized === 'secret'
  )
}

function validateProductionURL(name: string, value: string, options: { requireHTTPS?: boolean } = {}) {
  const normalized = normalizeOrigin(value)
  if (!normalized || (options.requireHTTPS && !normalized.startsWith('https://'))) {
    if (options.requireHTTPS) {
      throw new Error(`${name} must be a valid https URL when NODE_ENV=production`)
    }
    throw new Error(`${name} must be a valid http(s) URL when NODE_ENV=production`)
  }
}

function validateDatabaseSSLMode(value: string): DatabaseSSLMode {
  const normalized = value.toLowerCase()
  if (normalized === 'disable' || normalized === 'require' || normalized === 'verify-full') {
    return normalized
  }
  throw new Error('DB_SSLMODE must be one of disable, require, or verify-full')
}

export function loadEnv(source: EnvSource) {
  const productionRuntime = isProductionRuntime(source)

  const publicOrigin = getEnv(source, 'PUBLIC_ORIGIN', '')
  const authInternalKey = getEnv(source, 'AUTH_INTERNAL_KEY', '')
  const authHost = getEnv(source, 'AUTH_HOST', '127.0.0.1')
  const authSecret =
    getEnv(source, 'BETTER_AUTH_SECRET', '') ||
    (productionRuntime ? '' : 'ideaboard-local-development-secret-change-before-production')
  const authBaseURL = publicOrigin || (productionRuntime ? '' : 'http://localhost:5173')
  const trustedOrigins = [publicOrigin || authBaseURL]
    .map((item) => item.trim())
    .filter(Boolean)
    .filter((item, index, all) => all.indexOf(item) === index)
  const apiInternalURL =
    getEnv(source, 'API_INTERNAL_URL', '') || 'http://127.0.0.1:6000'
  const dbPassword = getEnv(source, 'DB_PASSWORD', '')
  const secretsEncryptionKey = getEnv(source, 'APP_SECRETS_ENCRYPTION_KEY', '')
  const dbSSLMode = validateDatabaseSSLMode(getEnv(source, 'DB_SSLMODE', 'disable'))

  if (!authInternalKey) {
    throw new Error('AUTH_INTERNAL_KEY is required')
  }

  if (productionRuntime) {
    if (isUnsafeProductionSecret(authInternalKey)) {
      throw new Error('AUTH_INTERNAL_KEY must be set to a non-placeholder value when NODE_ENV=production')
    }
    if (authInternalKey.length < 32) {
      throw new Error('AUTH_INTERNAL_KEY must be at least 32 characters long')
    }
    if (isUnsafeProductionSecret(authSecret)) {
      throw new Error('BETTER_AUTH_SECRET must be set to a non-placeholder value when NODE_ENV=production')
    }
    if (authSecret.length < 32) {
      throw new Error('BETTER_AUTH_SECRET must be at least 32 characters long')
    }
    if (isUnsafeProductionSecret(dbPassword)) {
      throw new Error('DB_PASSWORD must be set to a non-placeholder value when NODE_ENV=production')
    }
    if (!isValidSecretEncryptionKey(secretsEncryptionKey)) {
      throw new Error(
        'APP_SECRETS_ENCRYPTION_KEY must be base64 that decodes to exactly 32 bytes when NODE_ENV=production',
      )
    }
    if (!authBaseURL) {
      throw new Error('PUBLIC_ORIGIN is required when NODE_ENV=production')
    }
    validateProductionURL('PUBLIC_ORIGIN', authBaseURL, { requireHTTPS: true })
    validateProductionURL('PUBLIC_ORIGIN', trustedOrigins[0] ?? authBaseURL, { requireHTTPS: true })
  }

  return {
    auth: {
      port: getEnvInt(source, 'AUTH_PORT', 6001),
      host: authHost,
      trustProxy: getEnv(source, 'AUTH_TRUST_PROXY', 'loopback'),
      internalKey: authInternalKey,
      secret: authSecret,
      baseURL: authBaseURL,
      trustedOrigins,
      apiInternalURL,
    },
    database: {
      host: getEnv(source, 'DB_HOST', '127.0.0.1'),
      port: getEnvInt(source, 'DB_PORT', 5432),
      user: getEnv(source, 'DB_USER', 'ideaboard'),
      password: dbPassword,
      database: getEnv(source, 'DB_NAME', 'ideaboard'),
      sslMode: dbSSLMode,
    },
    secrets: {
      encryptionKey: secretsEncryptionKey,
    },
  }
}

export type AuthEnvironment = ReturnType<typeof loadEnv>
