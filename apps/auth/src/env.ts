import dotenv from 'dotenv'
import { isProductionRuntime } from './runtime-env.js'

dotenv.config()

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
  if (normalized.includes('change-before-production')) return true
  return normalized === 'changeme' || normalized === 'change-me' || normalized === 'password' || normalized === 'secret'
}

function validateProductionURL(name: string, value: string) {
  try {
    const parsed = new URL(value)
    if (parsed.protocol !== 'http:' && parsed.protocol !== 'https:') {
      throw new Error('invalid protocol')
    }
  } catch {
    throw new Error(`${name} must be a valid http(s) URL when NODE_ENV=production`)
  }
}

const productionRuntime = isProductionRuntime()

const trustedOrigins = getEnv('AUTH_TRUSTED_ORIGINS', '')
  .split(',')
  .map((item) => item.trim())
  .filter(Boolean)

const authInternalKey = getEnv('AUTH_INTERNAL_KEY', '')
const authSecret =
  getEnv('BETTER_AUTH_SECRET', getEnv('AUTH_SECRET', '')) ||
  (productionRuntime ? '' : 'ideaboard-local-development-secret-change-before-production')
const authBaseURL =
  getEnv('BETTER_AUTH_URL', getEnv('AUTH_BASE_URL', '')) ||
  (productionRuntime ? '' : 'http://localhost:5173')

if (productionRuntime) {
  if (isUnsafeProductionSecret(authInternalKey)) {
    throw new Error('AUTH_INTERNAL_KEY must be set to a non-placeholder value when NODE_ENV=production')
  }
  if (isUnsafeProductionSecret(authSecret)) {
    throw new Error('BETTER_AUTH_SECRET must be set to a non-placeholder value when NODE_ENV=production')
  }
  if (authSecret.length < 32) {
    throw new Error('BETTER_AUTH_SECRET must be at least 32 characters long')
  }
  if (!authBaseURL) {
    throw new Error('BETTER_AUTH_URL is required when NODE_ENV=production')
  }
  validateProductionURL('BETTER_AUTH_URL', authBaseURL)
}

export const env = {
  auth: {
    port: getEnvInt('AUTH_PORT', 6001),
    host: getEnv('AUTH_HOST', '127.0.0.1'),
    trustProxy: getEnv('AUTH_TRUST_PROXY', 'loopback'),
    internalKey: authInternalKey,
    secret: authSecret,
    baseURL: authBaseURL,
    trustedOrigins,
  },
  database: {
    host: getEnv('DB_HOST', '127.0.0.1'),
    port: getEnvInt('DB_PORT', 5432),
    user: getEnv('DB_USER', 'ideaboard'),
    password: getEnv('DB_PASSWORD', ''),
    database: getEnv('DB_NAME', 'ideaboard'),
    sslMode: getEnv('DB_SSLMODE', 'disable'),
  },
}
