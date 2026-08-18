import { resolveApiData } from '../../../shared/api/apiResponse'
import {
  getAdminOAuthCapabilities,
  getAdminOAuthProviderSettings,
  updateAdminOAuthProviderSettings,
} from '../../../shared/api/generated-query/admin-users/admin-users'
import type {
  ApiResponseOAuthCapabilities,
  ApiResponseOAuthProviderSettings,
  OAuthProviderSettings,
  OAuthProviderSettingsUpdate,
} from '../../../shared/api/generated-query/model'
import {
  getOAuthProviderLabel,
  type OAuthProviderCode,
} from '../../../shared/auth/oauthProviders'

export type OAuthProviderSettingsSummary = {
  provider: OAuthProviderCode
  enabled: boolean
  configured: boolean
  available: boolean
  clientId: string
  clientSecretConfigured: boolean
  scopes: string[]
  configVersion: number
  callbackUrl: string
  updatedAt: string | null
}

export type SaveOAuthProviderSettingsInput = {
  enabled: boolean
  clientId: string
  clientSecret: string
}

export type OAuthCapabilities = {
  social: Record<string, boolean>
}

export type OAuthProviderSettingsResource = {
  get(provider: OAuthProviderCode): Promise<OAuthProviderSettingsSummary>
  save(
    provider: OAuthProviderCode,
    input: SaveOAuthProviderSettingsInput,
  ): Promise<OAuthProviderSettingsSummary>
  getCapabilities(): Promise<OAuthCapabilities>
}

type OAuthProviderSettingsApi = {
  getSettings(provider: OAuthProviderCode): Promise<ApiResponseOAuthProviderSettings>
  updateSettings(
    provider: OAuthProviderCode,
    input: OAuthProviderSettingsUpdate,
  ): Promise<ApiResponseOAuthProviderSettings>
  getCapabilities(): Promise<ApiResponseOAuthCapabilities>
}

const generatedApi: OAuthProviderSettingsApi = {
  getSettings: getAdminOAuthProviderSettings,
  updateSettings: updateAdminOAuthProviderSettings,
  getCapabilities: getAdminOAuthCapabilities,
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
      return normalizeSettings(settings, provider)
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
      return normalizeSettings(settings, provider)
    },
    async getCapabilities() {
      const capabilities = await resolveApiData(
        api.getCapabilities(),
        '加载 OAuth capabilities 失败',
      )
      return {
        social: {
          google: capabilities.social.google === true,
          discord: capabilities.social.discord === true,
        },
      }
    },
  }
}

function normalizeSettings(
  settings: OAuthProviderSettings,
  requestedProvider: OAuthProviderCode,
): OAuthProviderSettingsSummary {
  return {
    provider: requestedProvider,
    enabled: settings.enabled,
    configured: settings.configured,
    available: settings.available,
    clientId: settings.client_id,
    clientSecretConfigured: settings.client_secret_configured,
    scopes: [...settings.scopes],
    configVersion: settings.config_version,
    callbackUrl: settings.callback_url,
    updatedAt: settings.updated_at,
  }
}

function providerLabel(provider: OAuthProviderCode): string {
  return getOAuthProviderLabel(provider)
}
