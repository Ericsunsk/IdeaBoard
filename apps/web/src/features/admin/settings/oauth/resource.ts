import {
  getControlOAuthProvider,
  updateControlOAuthProvider,
} from '../../../../shared/api/generated-query/control/control-users/control-users'
import type { ControlOAuthProviderSummary } from '../../../../shared/api/generated-query/control/model/controlOAuthProviderSummary'
import type { ControlOAuthProviderUpdateRequest } from '../../../../shared/api/generated-query/control/model/controlOAuthProviderUpdateRequest'
import type { OAuthProviderCode } from '../../../../shared/auth/oauthProviders'
import { loadControlSetting, storeControlSetting } from '../../shared/lib/controlResource'

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

export interface OAuthProviderSettingsResource {
  get(provider: OAuthProviderCode): Promise<OAuthProviderSettingsSummary>
  save(
    provider: OAuthProviderCode,
    input: SaveOAuthProviderSettingsInput,
    operatorReason: string,
  ): Promise<OAuthProviderSettingsSummary>
}

function toSummary(data: ControlOAuthProviderSummary): OAuthProviderSettingsSummary {
  return {
    enabled: data.enabled,
    configured: data.configured,
    clientId: data.client_id,
    clientSecretConfigured: data.client_secret_configured,
    callbackUrl: data.callback_url,
  }
}

export function createOAuthProviderSettingsResource(): OAuthProviderSettingsResource {
  return {
    get: (provider) =>
      loadControlSetting(() => getControlOAuthProvider(provider), {
        fallback: '加载 OAuth Provider 设置失败，请重试',
        map: toSummary,
      }),
    save: (provider, input, operatorReason) =>
      storeControlSetting(
        input,
        { operatorReason },
        {
          fallback: '保存 OAuth Provider 设置失败，请重试',
          map: toSummary,
          toRequest: (settings: SaveOAuthProviderSettingsInput, reason: string) => ({
            enabled: settings.enabled,
            client_id: settings.clientId,
            client_secret: settings.clientSecret,
            reason,
          }),
          persist: (body, headers) =>
            updateControlOAuthProvider(provider, body as ControlOAuthProviderUpdateRequest, {
              headers,
            }),
        },
      ),
  }
}
