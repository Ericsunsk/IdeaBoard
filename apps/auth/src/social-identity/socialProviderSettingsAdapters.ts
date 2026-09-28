import type { Pool } from 'pg'
import { createSecretCipher } from '../infrastructure/secretCipher.js'
import {
  SocialProviderSettingsService,
  type RuntimeSocialProviderApply,
  type SocialProviderSettingsRepository,
  type StoredSocialProviderSettings,
  type SupportedSocialProvider,
} from './socialProviderSettings.js'

export function createRuntimeSocialProviderSettingsService(
  pool: Pool,
  baseURL: string,
  encryptionKey: string,
  applyRuntimeConfig?: RuntimeSocialProviderApply,
): SocialProviderSettingsService {
  return new SocialProviderSettingsService(
    createPgSocialProviderSettingsRepository(pool),
    createSecretCipher(encryptionKey),
    baseURL,
    applyRuntimeConfig,
  )
}

export function createPgSocialProviderSettingsRepository(
  pool: Pool,
): SocialProviderSettingsRepository {
  return {
    async get(provider) {
      const result = await pool.query<SocialProviderSettingsRow>(
        'SELECT provider, enabled, client_id, encrypted_client_secret, scopes, config_version, updated_at FROM social_provider_settings WHERE provider = $1',
        [provider],
      )
      return result.rows[0] ? fromRow(result.rows[0]) : null
    },
    async put(value) {
      const result = await pool.query<SocialProviderSettingsRow>(
        'INSERT INTO social_provider_settings (provider, enabled, client_id, encrypted_client_secret, scopes, config_version, updated_at) VALUES ($1, $2, $3, $4, $5::jsonb, 1, clock_timestamp()) ON CONFLICT (provider) DO UPDATE SET enabled = EXCLUDED.enabled, client_id = EXCLUDED.client_id, encrypted_client_secret = EXCLUDED.encrypted_client_secret, scopes = EXCLUDED.scopes, config_version = social_provider_settings.config_version + 1, updated_at = clock_timestamp() RETURNING provider, enabled, client_id, encrypted_client_secret, scopes, config_version, updated_at',
        [
          value.provider,
          value.enabled,
          value.clientId,
          value.encryptedClientSecret,
          JSON.stringify(value.scopes),
        ],
      )
      return fromRow(result.rows[0])
    },
  }
}

type SocialProviderSettingsRow = {
  provider: SupportedSocialProvider
  enabled: boolean
  client_id: string
  encrypted_client_secret: string
  scopes: string[]
  config_version: number | string
  updated_at: Date
}

function fromRow(row: SocialProviderSettingsRow): StoredSocialProviderSettings {
  return {
    provider: row.provider,
    enabled: row.enabled,
    clientId: row.client_id,
    encryptedClientSecret: row.encrypted_client_secret,
    scopes: Array.isArray(row.scopes) ? row.scopes.map(String) : [],
    configVersion: Number(row.config_version),
    updatedAt: row.updated_at,
  }
}
