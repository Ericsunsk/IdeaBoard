import dotenv from 'dotenv'

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

function getEnvBool(name: string, fallback: boolean): boolean {
  const raw = getEnv(name, '')
  if (!raw) return fallback
  switch (raw.toLowerCase()) {
    case '1':
    case 'true':
    case 'yes':
    case 'on':
      return true
    case '0':
    case 'false':
    case 'no':
    case 'off':
      return false
    default:
      return fallback
  }
}

function isProductionRuntime(): boolean {
  const value = getEnv('NODE_ENV', getEnv('APP_ENV', '')).toLowerCase()
  return value === 'production' || value === 'prod' || value === 'release'
}

const trustedOrigins = getEnv('AUTH_TRUSTED_ORIGINS', '')
  .split(',')
  .map((item) => item.trim())
  .filter(Boolean)

export const env = {
  auth: {
    port: getEnvInt('AUTH_PORT', 6001),
    host: getEnv('AUTH_HOST', '127.0.0.1'),
    internalKey: getEnv('AUTH_INTERNAL_KEY', ''),
    trustedOrigins,
    registrationEnabled: getEnvBool('AUTH_REGISTRATION_ENABLED', !isProductionRuntime()),
    rateLimitWindowSeconds: getEnvInt('AUTH_RATE_LIMIT_WINDOW_SECONDS', 60),
    rateLimitMaxRequests: getEnvInt('AUTH_RATE_LIMIT_MAX_REQUESTS', 30),
  },
  database: {
    host: getEnv('DB_HOST', '127.0.0.1'),
    port: getEnvInt('DB_PORT', 3306),
    user: getEnv('DB_USER', 'ideaboard'),
    password: getEnv('DB_PASSWORD', ''),
    database: getEnv('DB_NAME', 'ideaboard'),
  },
}
