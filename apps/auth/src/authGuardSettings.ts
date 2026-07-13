import { authDB } from './auth.js'

export type AuthGuardSettings = {
  registrationEnabled: boolean
  windowSeconds: number
  maxRequests: number
}

type AuthGuardSettingsCache = {
  value: AuthGuardSettings
  expiresAtMs: number
}

type AuthGuardSettingsRow = {
  registration_enabled?: unknown
}

const AUTH_GUARD_SETTINGS_TTL_MS = 10_000
export const DEFAULT_AUTH_RATE_LIMIT_WINDOW_SECONDS = 60
export const DEFAULT_AUTH_RATE_LIMIT_MAX_REQUESTS = 30

// Auth 登录/注册限流由代码常量控制；后台 runtime settings 只调 Go API 全局限流。
const DEFAULT_AUTH_GUARD_SETTINGS: AuthGuardSettings = {
  registrationEnabled: false,
  windowSeconds: DEFAULT_AUTH_RATE_LIMIT_WINDOW_SECONDS,
  maxRequests: DEFAULT_AUTH_RATE_LIMIT_MAX_REQUESTS,
}

let authGuardSettingsCache: AuthGuardSettingsCache | null = null

export async function getAuthGuardSettings(): Promise<AuthGuardSettings> {
  const nowMs = Date.now()
  if (authGuardSettingsCache && nowMs < authGuardSettingsCache.expiresAtMs) {
    return authGuardSettingsCache.value
  }

  let nextSettings: AuthGuardSettings = {
    ...DEFAULT_AUTH_GUARD_SETTINGS,
  }
  try {
    const result = await authDB.query(
      'SELECT registration_enabled FROM get_auth_runtime_settings()',
    )
    const row = result.rows[0] as AuthGuardSettingsRow | undefined
    if (typeof row?.registration_enabled === 'boolean') {
      nextSettings.registrationEnabled = row.registration_enabled
    }
  } catch (error) {
    // If the typed settings seam is unavailable, fail closed and avoid hammering DB.
    console.error('[auth] Failed to query auth guard settings', error)
    nextSettings = { ...DEFAULT_AUTH_GUARD_SETTINGS }
  }

  authGuardSettingsCache = {
    value: nextSettings,
    expiresAtMs: nowMs + AUTH_GUARD_SETTINGS_TTL_MS,
  }
  return nextSettings
}
