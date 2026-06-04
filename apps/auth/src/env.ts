import dotenv from 'dotenv'
import { isIP } from 'node:net'
import { isProductionRuntime } from './runtime-env.js'

dotenv.config({ quiet: true })

function getEnv(name: string, fallback = ''): string {
  const raw = process.env[name]
  if (typeof raw !== 'string') return fallback
  return raw.trim()
}

function getEnvInt(name: string, fallback: number): number {
  const raw = getEnv(name, '')
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
  return normalized === 'changeme' || normalized === 'change-me' || normalized === 'password' || normalized === 'secret'
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

function validateDatabaseSSLMode(value: string): 'disable' | 'require' | 'verify-full' {
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

const productionRuntime = isProductionRuntime()

const trustedOrigins = getEnv('AUTH_TRUSTED_ORIGINS', '')
  .split(',')
  .map((item) => item.trim())
  .filter(Boolean)

const authInternalKey = getEnv('AUTH_INTERNAL_KEY', '')
const authHost = getEnv('AUTH_HOST', '127.0.0.1')
const allowInsecureInternalBypass =
  !productionRuntime && !authInternalKey && getEnv('AUTH_INSECURE_DEV', '') === '1' && isLoopbackHost(authHost)
const authSecret =
  getEnv('BETTER_AUTH_SECRET', '') ||
  (productionRuntime ? '' : 'ideaboard-local-development-secret-change-before-production')
const authBaseURL =
  getEnv('BETTER_AUTH_URL', '') ||
  (productionRuntime ? '' : 'http://localhost:5173')
const dbPassword = getEnv('DB_PASSWORD', '')
const dbSSLMode = validateDatabaseSSLMode(getEnv('DB_SSLMODE', 'disable'))

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

export const env = {
  auth: {
    port: getEnvInt('AUTH_PORT', 6001),
    host: authHost,
    trustProxy: getEnv('AUTH_TRUST_PROXY', 'loopback'),
    internalKey: authInternalKey,
    allowInsecureInternalBypass,
    secret: authSecret,
    baseURL: authBaseURL,
    trustedOrigins,
  },
  database: {
    host: getEnv('DB_HOST', '127.0.0.1'),
    port: getEnvInt('DB_PORT', 5432),
    user: getEnv('DB_USER', 'ideaboard'),
    password: dbPassword,
    database: getEnv('DB_NAME', 'ideaboard'),
    sslMode: dbSSLMode,
  },
}
