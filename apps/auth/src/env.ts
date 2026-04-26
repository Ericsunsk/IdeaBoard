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
  },
  database: {
    host: getEnv('DB_HOST', '127.0.0.1'),
    port: getEnvInt('DB_PORT', 3306),
    user: getEnv('DB_USER', 'ideaboard'),
    password: getEnv('DB_PASSWORD', ''),
    database: getEnv('DB_NAME', 'ideaboard'),
  },
}
