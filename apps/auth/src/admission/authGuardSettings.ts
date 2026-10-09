export type AuthGuardSettings = {
  registrationEnabled: boolean
  projectName?: string
  logoUrl?: string
  windowSeconds: number
  maxRequests: number
  lockoutThreshold: number
  lockoutDurationSeconds: number
}

type AuthGuardSettingsCache = {
  value: AuthGuardSettings
  expiresAtMs: number
}

type AuthGuardSettingsRow = {
  registration_enabled?: unknown
  branding_project_name?: unknown
  branding_logo_url?: unknown
  login_lockout_threshold?: unknown
  login_lockout_duration_seconds?: unknown
}

type AuthGuardSettingsDatabase = {
  query(queryText: string): Promise<{ rows: unknown[] }>
}

const AUTH_GUARD_SETTINGS_TTL_MS = 10_000
export const DEFAULT_AUTH_RATE_LIMIT_WINDOW_SECONDS = 60
export const DEFAULT_AUTH_RATE_LIMIT_MAX_REQUESTS = 30
export const DEFAULT_LOGIN_LOCKOUT_THRESHOLD = 10
export const MIN_LOGIN_LOCKOUT_THRESHOLD = 3
export const MAX_LOGIN_LOCKOUT_THRESHOLD = 100
export const DEFAULT_LOGIN_LOCKOUT_DURATION_SECONDS = 900
export const MIN_LOGIN_LOCKOUT_DURATION_SECONDS = 60
export const MAX_LOGIN_LOCKOUT_DURATION_SECONDS = 86400

const DEFAULT_AUTH_GUARD_SETTINGS: AuthGuardSettings = {
  registrationEnabled: false,
  projectName: 'IdeaBoard',
  logoUrl: '',
  windowSeconds: DEFAULT_AUTH_RATE_LIMIT_WINDOW_SECONDS,
  maxRequests: DEFAULT_AUTH_RATE_LIMIT_MAX_REQUESTS,
  lockoutThreshold: DEFAULT_LOGIN_LOCKOUT_THRESHOLD,
  lockoutDurationSeconds: DEFAULT_LOGIN_LOCKOUT_DURATION_SECONDS,
}

export type AuthGuardSettingsReaderOptions = {
  now?: () => number
  ttlMs?: number
  onError?: (error: unknown) => void
}

export function createAuthGuardSettingsReader(
  database: AuthGuardSettingsDatabase,
  options: AuthGuardSettingsReaderOptions = {},
): () => Promise<AuthGuardSettings> {
  let cache: AuthGuardSettingsCache | null = null
  const now = options.now ?? Date.now
  const ttlMs = options.ttlMs ?? AUTH_GUARD_SETTINGS_TTL_MS
  const onError =
    options.onError ??
    ((error: unknown) => {
      console.error('[auth] Failed to query auth guard settings', error)
    })

  return async () => {
    const nowMs = now()
    if (cache && nowMs < cache.expiresAtMs) {
      return cache.value
    }

    let nextSettings: AuthGuardSettings = {
      ...DEFAULT_AUTH_GUARD_SETTINGS,
    }
    try {
      const result = await database.query(
        'SELECT registration_enabled, branding_project_name, branding_logo_url, login_lockout_threshold, login_lockout_duration_seconds FROM get_auth_runtime_settings() CROSS JOIN get_auth_login_lockout_settings()',
      )
      const row = result.rows[0] as AuthGuardSettingsRow | undefined
      if (typeof row?.registration_enabled === 'boolean') {
        nextSettings.registrationEnabled = row.registration_enabled
      }
      if (typeof row?.branding_project_name === 'string' && row.branding_project_name.trim()) {
        nextSettings.projectName = row.branding_project_name.trim()
      }
      if (typeof row?.branding_logo_url === 'string') {
        nextSettings.logoUrl = row.branding_logo_url.trim()
      }
      const threshold = normalizeBoundedInteger(
        row?.login_lockout_threshold,
        MIN_LOGIN_LOCKOUT_THRESHOLD,
        MAX_LOGIN_LOCKOUT_THRESHOLD,
      )
      if (threshold !== null) {
        nextSettings.lockoutThreshold = threshold
      }
      const durationSeconds = normalizeBoundedInteger(
        row?.login_lockout_duration_seconds,
        MIN_LOGIN_LOCKOUT_DURATION_SECONDS,
        MAX_LOGIN_LOCKOUT_DURATION_SECONDS,
      )
      if (durationSeconds !== null) {
        nextSettings.lockoutDurationSeconds = durationSeconds
      }
    } catch (error) {
      // The cached default keeps registration closed without repeatedly querying a failed dependency.
      onError(error)
      nextSettings = { ...DEFAULT_AUTH_GUARD_SETTINGS }
    }

    cache = {
      value: nextSettings,
      expiresAtMs: nowMs + ttlMs,
    }
    return nextSettings
  }
}

function normalizeBoundedInteger(raw: unknown, min: number, max: number): number | null {
  const value =
    typeof raw === 'number' ? raw : typeof raw === 'string' && raw.trim() ? Number(raw) : NaN
  if (!Number.isInteger(value) || value < min || value > max) {
    return null
  }
  return value
}
