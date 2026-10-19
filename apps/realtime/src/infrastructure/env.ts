// Typed env contract for the realtime presence sidecar. Mirrors the
// apps/auth infrastructure/env.ts shape: both entrypoints load through
// loadEnv(source) and never read process.env past the entrypoint.
//
// Naming: RT_ owns sidecar config, AUTH_INTERNAL_KEY is the single shared
// internal key (validated by Go as AUTH_INTERNAL_KEY), and API_INTERNAL_URL
// is the single Go API origin shared with apps/auth.

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

// Canonical production switch, shared with apps/auth runtimeEnv and the Go
// APP_ENV gate: production, prod, or release via NODE_ENV or APP_ENV.
function isProductionRuntime(source: EnvSource): boolean {
  const value = String(source.NODE_ENV || source.APP_ENV || '')
    .trim()
    .toLowerCase()
  return value === 'production' || value === 'prod' || value === 'release'
}

// Shared placeholder blacklist, same rule as the Go API
// (isUnsafeProductionSecret) and apps/auth: empty, replace-with*/replace_with*
// prefixes, placeholder/change-before-production substrings, and the exact
// weak tokens changeme/change-me/password/secret.
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

  const ticketSecret = getEnv(source, 'RT_TICKET_SECRET', '')
  const internalKey = getEnv(source, 'AUTH_INTERNAL_KEY', '')
  const apiInternalURL = getEnv(source, 'API_INTERNAL_URL', '') || 'http://127.0.0.1:6000'

  // Collect every missing or weak value before throwing so one startup
  // attempt lists everything to fill instead of failing one key at a time.
  const problems: string[] = []
  if (!ticketSecret) {
    problems.push('RT_TICKET_SECRET is required')
  }
  if (!internalKey) {
    problems.push('AUTH_INTERNAL_KEY is required')
  }

  if (productionRuntime) {
    if (isUnsafeProductionSecret(ticketSecret)) {
      problems.push(
        'RT_TICKET_SECRET must be set to a non-placeholder value when NODE_ENV=production',
      )
    } else if (ticketSecret.length < 32) {
      problems.push('RT_TICKET_SECRET must be at least 32 characters long')
    }
    if (isUnsafeProductionSecret(internalKey)) {
      problems.push(
        'AUTH_INTERNAL_KEY must be set to a non-placeholder value when NODE_ENV=production',
      )
    } else if (internalKey.length < 32) {
      problems.push('AUTH_INTERNAL_KEY must be at least 32 characters long')
    }
    try {
      const parsed = new URL(apiInternalURL)
      if (parsed.protocol !== 'http:' && parsed.protocol !== 'https:') {
        throw new Error('unsupported protocol')
      }
    } catch {
      problems.push('API_INTERNAL_URL must be a valid http(s) URL when NODE_ENV=production')
    }
  }

  if (problems.length > 0) {
    throw new Error(`missing or invalid required env: ${problems.join('; ')}`)
  }

  return {
    realtime: {
      host: getEnv(source, 'RT_HOST', '127.0.0.1'),
      port: getEnvInt(source, 'RT_PORT', 6002),
      ticketSecret,
      internalKey,
      apiInternalURL,
      yCompactEvery: getEnvInt(source, 'RT_Y_COMPACT_EVERY', 50),
      yPersistRetries: getEnvInt(source, 'RT_Y_PERSIST_RETRIES', 5),
    },
  }
}

export type RealtimeEnvironment = ReturnType<typeof loadEnv>
