import { api } from '../../../shared/api/client'
import {
  resolveApiData,
  type ApiResponseWithData,
} from '../../../shared/api/apiResponse'
import { getOAuthProviderLabel, type OAuthProviderCode } from '../../../shared/auth/oauthProviders'

// Admin OAuth provider boundary (issue #809). The generated `admin-users`
// client is gone but the backend admin plane still serves
// `/api/admin/users/oauth-providers/:provider` (GET/PUT) with the shared
// envelope, so this boundary transports through the central `api.request`
// with local wire types.
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

export type OAuthProviderSettingsUpdate = {
  enabled: boolean
  client_id: string
  client_secret?: string
}

export type ApiResponseOAuthProviderSettings = ApiResponseWithData<OAuthProviderSettings>

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

type OAuthProviderSettingsApi = {
  getSettings(provider: OAuthProviderCode): Promise<ApiResponseOAuthProviderSettings>
  updateSettings(
    provider: OAuthProviderCode,
    input: OAuthProviderSettingsUpdate,
  ): Promise<ApiResponseOAuthProviderSettings>
}

const userOAuthProviderApi: OAuthProviderSettingsApi = {
  getSettings: (provider) =>
    api.request<ApiResponseOAuthProviderSettings>(
      `/admin/users/oauth-providers/${encodeURIComponent(provider)}`,
      { method: 'GET' },
    ),
  updateSettings: (provider, input) =>
    api.request<ApiResponseOAuthProviderSettings>(
      `/admin/users/oauth-providers/${encodeURIComponent(provider)}`,
      {
        method: 'PUT',
        body: input as unknown as Record<string, unknown>,
      },
    ),
}

export function createOAuthProviderSettingsResource(
  api: OAuthProviderSettingsApi = userOAuthProviderApi,
): OAuthProviderSettingsResource {
  return {
    async get(provider) {
      const settings = await resolveApiData(
        api.getSettings(provider),
        `加载 ${providerLabel(provider)} OAuth 设置失败`,
      )
      return normalizeSettings(settings)
    },
    async save(provider, input) {
      const body: OAuthProviderSettingsUpdate = {
        enabled: input.enabled,
        client_id: input.clientId.trim(),
        ...(input.clientSecret.trim() ? { client_secret: input.clientSecret } : {}),
      }
      const settings = await resolveApiData(
        api.updateSettings(provider, body),
        `保存 ${providerLabel(provider)} OAuth 设置失败`,
      )
      return normalizeSettings(settings)
    },
  }
}

function normalizeSettings(settings: OAuthProviderSettings): OAuthProviderSettingsSummary {
  return {
    enabled: settings.enabled,
    configured: settings.configured,
    clientId: settings.client_id,
    clientSecretConfigured: settings.client_secret_configured,
    callbackUrl: settings.callback_url,
  }
}

function providerLabel(provider: OAuthProviderCode): string {
  return getOAuthProviderLabel(provider)
}
