// Typed env contract for the realtime presence sidecar. Mirrors the
// apps/auth infrastructure/env.ts shape: both entrypoints load through
// loadEnv(source) and never read process.env past the entrypoint.

export type EnvSource = Record<string, string | undefined>

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

function isProductionRuntime(source: EnvSource): boolean {
  return getEnv(source, 'NODE_ENV', '') === 'production'
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

export function loadEnv(source: EnvSource) {
  const productionRuntime = isProductionRuntime(source)

  const ticketSecret = getEnv(source, 'REALTIME_TICKET_SECRET', '')
  const internalKey = getEnv(source, 'REALTIME_INTERNAL_KEY', '')
  const apiInternalURL = getEnv(source, 'API_INTERNAL_URL', '') || 'http://127.0.0.1:6000'

  if (!ticketSecret) {
    throw new Error('REALTIME_TICKET_SECRET is required')
  }
  if (!internalKey) {
    throw new Error('REALTIME_INTERNAL_KEY is required')
  }

  if (productionRuntime) {
    if (isUnsafeProductionSecret(ticketSecret)) {
      throw new Error(
        'REALTIME_TICKET_SECRET must be set to a non-placeholder value when NODE_ENV=production',
      )
    }
    if (ticketSecret.length < 32) {
      throw new Error('REALTIME_TICKET_SECRET must be at least 32 characters long')
    }
    if (isUnsafeProductionSecret(internalKey)) {
      throw new Error(
        'REALTIME_INTERNAL_KEY must be set to a non-placeholder value when NODE_ENV=production',
      )
    }
    if (internalKey.length < 32) {
      throw new Error('REALTIME_INTERNAL_KEY must be at least 32 characters long')
    }
    try {
      const parsed = new URL(apiInternalURL)
      if (parsed.protocol !== 'http:' && parsed.protocol !== 'https:') {
        throw new Error('unsupported protocol')
      }
    } catch {
      throw new Error('API_INTERNAL_URL must be a valid http(s) URL when NODE_ENV=production')
    }
  }

  return {
    realtime: {
      host: getEnv(source, 'REALTIME_HOST', '127.0.0.1'),
      port: getEnvInt(source, 'REALTIME_PORT', 6002),
      ticketSecret,
      internalKey,
      apiInternalURL,
      yCompactEvery: getEnvInt(source, 'REALTIME_Y_COMPACT_EVERY', 50),
      yPersistRetries: getEnvInt(source, 'REALTIME_Y_PERSIST_RETRIES', 5),
    },
  }
}

export type RealtimeEnvironment = ReturnType<typeof loadEnv>
