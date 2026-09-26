import { adminControlUnavailable } from '../shared/lib/controlGuidance'
import type { OAuthProviderCode } from '../../../shared/auth/oauthProviders'

// Admin OAuth provider boundary (W2 transports, fail-closed). The Control
// Plane has no OAuth provider operations, so get/save fail closed with
// guidance instead of calling the deleted
// `/api/admin/users/oauth-providers/:provider` routes. View types stay so
// the UI and specs keep their shapes. Backend gap: control OAuth providers
// (see the stream report).
export type OAuthProviderSettings = {
  provider: string
  enabled: boolean
  configured: boolean
  available: boolean
  client_id: string
  client_secret_configured: boolean
  scopes: string[]
  config_version: number
  callback_url: string
  updated_at: string | null
}

export type OAuthProviderSettingsSummary = {
  enabled: boolean
  configured: boolean
  clientId: string
  clientSecretConfigured: boolean
  callbackUrl: string
}

export type SaveOAuthProviderSettingsInput = {
  enabled: boolean
  clientId: string
  clientSecret: string
}

export type OAuthProviderSettingsResource = {
  get(provider: OAuthProviderCode): Promise<OAuthProviderSettingsSummary>
  save(
    provider: OAuthProviderCode,
    input: SaveOAuthProviderSettingsInput,
  ): Promise<OAuthProviderSettingsSummary>
}

export function createOAuthProviderSettingsResource(): OAuthProviderSettingsResource {
  return {
    async get() {
      throw adminControlUnavailable('oauth-providers')
    },
    async save() {
      throw adminControlUnavailable('oauth-providers')
    },
  }
}
