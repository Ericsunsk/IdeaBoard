import { resolveApiData } from '../../../shared/api/apiResponse'
import {
  getAdminOAuthProviderSettings,
  updateAdminOAuthProviderSettings,
} from '../../../shared/api/generated-query/admin-users/admin-users'
import type {
  ApiResponseOAuthProviderSettings,
  OAuthProviderSettings,
  OAuthProviderSettingsUpdate,
} from '../../../shared/api/generated-query/model'
import {
  getOAuthProviderLabel,
  type OAuthProviderCode,
} from '../../../shared/auth/oauthProviders'

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

const generatedApi: OAuthProviderSettingsApi = {
  getSettings: getAdminOAuthProviderSettings,
  updateSettings: updateAdminOAuthProviderSettings,
}

export function createOAuthProviderSettingsResource(
  api: OAuthProviderSettingsApi = generatedApi,
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
