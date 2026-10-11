import {
  getControlOAuthProvider,
  updateControlOAuthProvider,
} from '../../../../../api/generated-query/control/control-users/control-users'
import type { ControlOAuthProviderSummary } from '../../../../../api/generated-query/control/model/controlOAuthProviderSummary'
import type { ControlOAuthProviderUpdateRequest } from '../../../../../api/generated-query/control/model/controlOAuthProviderUpdateRequest'
import type { OAuthProviderCode } from '../../../../../core/auth/oauthProviders'
import { requireContractData } from '../../../../../api/contractKernel'
import { buildRoutineControlMutationHeaders } from '../../../shared/index'
import { loadControlSetting } from '../../../shared/index'
import { withControlCopy } from '../../../shared/index'

export type OAuthProviderSettingsSummary = {
  enabled: boolean
  configured: boolean
  clientId: string
  clientSecretConfigured: boolean
  callbackUrl: string
}

type SaveOAuthProviderSettingsInput = {
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
    save: saveOAuthProviderSettings,
  }
}

async function saveOAuthProviderSettings(
  provider: OAuthProviderCode,
  input: SaveOAuthProviderSettingsInput,
  operatorReason: string,
): Promise<OAuthProviderSettingsSummary> {
  // Routine config write (backend ReasonOptional): the reason may be empty
  // (audit records it as-is) while Idempotency-Key stays required.
  const reason = operatorReason.trim()
  try {
    const body = await updateControlOAuthProvider(
      provider,
      {
        enabled: input.enabled,
        client_id: input.clientId,
        client_secret: input.clientSecret,
        reason,
      } as ControlOAuthProviderUpdateRequest,
      {
        headers: buildRoutineControlMutationHeaders({ operatorReason: reason }),
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
