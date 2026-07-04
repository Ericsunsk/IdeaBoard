import dotenv from 'dotenv'
import { isIP } from 'node:net'
import { isProductionRuntime } from './runtimeEnv.js'

dotenv.config({ quiet: true })

type EnvSource = Record<string, string | undefined>
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
  try {
    const parsed = new URL(value)
    if (parsed.protocol !== 'http:' && parsed.protocol !== 'https:') {
      throw new Error('invalid protocol')
    }
    if (options.requireHTTPS && parsed.protocol !== 'https:') {
      throw new Error('insecure protocol')
    }
  } catch {
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

function isLoopbackHost(value: string): boolean {
  const normalized = value.trim().toLowerCase().replace(/^\[(.*)\]$/, '$1')
  if (normalized === 'localhost') return true

  const ipVersion = isIP(normalized)
  if (ipVersion === 4) return normalized.split('.')[0] === '127'
  if (ipVersion !== 6) return false

  return (
    normalized === '::1' ||
    normalized === '0:0:0:0:0:0:0:1' ||
    normalized.startsWith('::ffff:127.')
  )
}

export function loadEnv(source: EnvSource) {
  const productionRuntime = isProductionRuntime(source)

  const trustedOrigins = getEnv(source, 'AUTH_TRUSTED_ORIGINS', '')
    .split(',')
    .map((item) => item.trim())
    .filter(Boolean)

  const authInternalKey = getEnv(source, 'AUTH_INTERNAL_KEY', '')
  const authHost = getEnv(source, 'AUTH_HOST', '127.0.0.1')
  const allowInsecureInternalBypass =
    !productionRuntime &&
    !authInternalKey &&
    getEnv(source, 'AUTH_INSECURE_DEV', '') === '1' &&
    isLoopbackHost(authHost)
  const authSecret =
    getEnv(source, 'BETTER_AUTH_SECRET', '') ||
    (productionRuntime ? '' : 'ideaboard-local-development-secret-change-before-production')
  const authBaseURL =
    getEnv(source, 'BETTER_AUTH_URL', '') ||
    (productionRuntime ? '' : 'http://localhost:5173')
  const dbPassword = getEnv(source, 'DB_PASSWORD', '')
  const dbSSLMode = validateDatabaseSSLMode(getEnv(source, 'DB_SSLMODE', 'disable'))

  if (!authInternalKey && !allowInsecureInternalBypass) {
    throw new Error(
      'AUTH_INTERNAL_KEY is required unless AUTH_INSECURE_DEV=1 and AUTH_HOST is loopback',
    )
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
    if (!authBaseURL) {
      throw new Error('BETTER_AUTH_URL is required when NODE_ENV=production')
    }
    validateProductionURL('BETTER_AUTH_URL', authBaseURL, { requireHTTPS: true })
    for (const origin of trustedOrigins) {
      validateProductionURL('AUTH_TRUSTED_ORIGINS entry', origin)
    }
  }

  return {
    auth: {
      port: getEnvInt(source, 'AUTH_PORT', 6001),
      host: authHost,
      trustProxy: getEnv(source, 'AUTH_TRUST_PROXY', 'loopback'),
      internalKey: authInternalKey,
      allowInsecureInternalBypass,
      secret: authSecret,
      baseURL: authBaseURL,
      trustedOrigins,
    },
    database: {
      host: getEnv(source, 'DB_HOST', '127.0.0.1'),
      port: getEnvInt(source, 'DB_PORT', 5432),
      user: getEnv(source, 'DB_USER', 'ideaboard'),
      password: dbPassword,
      database: getEnv(source, 'DB_NAME', 'ideaboard'),
      sslMode: dbSSLMode,
    },
  }
}

export const env = loadEnv(process.env)
