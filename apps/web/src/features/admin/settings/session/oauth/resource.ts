import {
  getControlOAuthProvider,
  updateControlOAuthProvider,
} from '@/api/generated-query/control/control-users/control-users'
import type { ControlOAuthProviderSummary } from '@/api/generated-query/control/model/controlOAuthProviderSummary'
import type { ControlOAuthProviderUpdateRequest } from '@/api/generated-query/control/model/controlOAuthProviderUpdateRequest'
import type { OAuthProviderCode } from '@/core/auth/oauthProviders'
import { requireContractData } from '@/api/contractKernel'
import { buildControlMutationHeaders } from '../../../shared/index'
import { loadControlSetting } from '../../../shared/index'
import { withControlCopy } from '../../../shared/index'

export type OAuthProviderSettingsSummary = {
  isEnabled: boolean
  configured: boolean
  clientId: string
  clientSecretConfigured: boolean
  callbackUrl: string
}

type SaveOAuthProviderSettingsInput = {
  isEnabled: boolean
  clientId: string
  clientSecret: string
}

export interface OAuthProviderSettingsResource {
  get(provider: OAuthProviderCode): Promise<OAuthProviderSettingsSummary>
  save(
    provider: OAuthProviderCode,
    input: SaveOAuthProviderSettingsInput,
  ): Promise<OAuthProviderSettingsSummary>
}

function toSummary(data: ControlOAuthProviderSummary): OAuthProviderSettingsSummary {
  return {
    isEnabled: data.enabled,
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
    save: saveOAuthProviderSettings,
  }
}

async function saveOAuthProviderSettings(
  provider: OAuthProviderCode,
  input: SaveOAuthProviderSettingsInput,
): Promise<OAuthProviderSettingsSummary> {
  try {
    const body = await updateControlOAuthProvider(
      provider,
      {
        enabled: input.isEnabled,
        client_id: input.clientId,
        client_secret: input.clientSecret,
      } as ControlOAuthProviderUpdateRequest,
      {
        headers: buildControlMutationHeaders(),
      },
    )
    return toSummary(
      requireContractData<ControlOAuthProviderSummary>(
        body,
        '保存 OAuth Provider 设置失败，请重试',
      ),
    )
  } catch (error) {
    throw withControlCopy(error, '保存 OAuth Provider 设置失败，请重试')
  }
}
