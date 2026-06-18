import { authDB } from './auth.js'
import { clampInteger } from './authUtils.js'

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
  config_key?: unknown
  value?: unknown
}

const AUTH_GUARD_SETTINGS_TTL_MS = 10_000
const DEFAULT_AUTH_GUARD_SETTINGS: AuthGuardSettings = {
  registrationEnabled: false,
  windowSeconds: 60,
  maxRequests: 30,
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
      'SELECT config_key, value FROM system_configs WHERE config_key = ANY($1::text[])',
      [
        [
          'auth_registration_enabled',
          'auth_rate_limit_window',
          'auth_rate_limit_max_requests',
        ],
      ],
    )
    for (const row of result.rows as AuthGuardSettingsRow[]) {
      applyAuthGuardSettingsRow(nextSettings, row)
    }
  } catch (error) {
    // If query fails (including missing table), apply safe defaults and avoid hammering DB.
    console.error('[auth] Failed to query system_configs for auth guard settings', error)
    nextSettings = { ...DEFAULT_AUTH_GUARD_SETTINGS }
  }

  authGuardSettingsCache = {
    value: nextSettings,
    expiresAtMs: nowMs + AUTH_GUARD_SETTINGS_TTL_MS,
  }
  return nextSettings
}

function applyAuthGuardSettingsRow(settings: AuthGuardSettings, row: AuthGuardSettingsRow) {
  if (row?.config_key === 'auth_registration_enabled' && typeof row?.value === 'string') {
    settings.registrationEnabled = row.value === 'true'
  }
  if (row?.config_key === 'auth_rate_limit_window' && row?.value) {
    settings.windowSeconds = clampInteger(
      row.value,
      1,
      3600,
      DEFAULT_AUTH_GUARD_SETTINGS.windowSeconds,
    )
  }
  if (row?.config_key === 'auth_rate_limit_max_requests' && row?.value) {
    settings.maxRequests = clampInteger(
      row.value,
      1,
      10000,
      DEFAULT_AUTH_GUARD_SETTINGS.maxRequests,
    )
  }
}
