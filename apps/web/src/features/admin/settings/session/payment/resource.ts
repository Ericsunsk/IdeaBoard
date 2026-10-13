import {
  getControlStripeConfig,
  updateControlStripeConfig,
} from '../../../../../api/generated-query/control/control-payments/control-payments'
import type { ControlStripeConfigRequest } from '../../../../../api/generated-query/control/model/controlStripeConfigRequest'
import { requireContractData } from '../../../../../api/contractKernel'
import { buildControlMutationHeaders } from '../../../shared/index'
import { loadControlSetting } from '../../../shared/index'
import { withControlCopy } from '../../../shared/index'
import type { AdminStripeSettingsResource, StripeConfigurationSummary } from './types'

/** @externalContract Control Plane Stripe configuration response. */
export type ApiStripeConfigurationSummary = {
  configured: boolean
  api_key_configured: boolean
  webhook_secret_configured: boolean
  account_id?: string
  mode?: 'test' | 'live'
  charges_enabled: boolean
  payment_method_configuration_id?: string
  product_tax_code: string
  product_ready: boolean
  webhook_ready: boolean
  purchases_enabled: boolean
  updated_at?: string
}

export function createAdminStripeSettingsResource(): AdminStripeSettingsResource {
  return {
    get: () =>
      loadControlSetting(() => getControlStripeConfig('stripe'), {
        fallback: '加载 Stripe 设置失败',
        map: normalizeStripeConfiguration,
      }),
    save: async (input, mutation) => {
      try {
        const data = requireContractData<ApiStripeConfigurationSummary>(
          await updateControlStripeConfig(
            'stripe',
            {
              ...(input.apiKey?.trim() ? { api_key: input.apiKey.trim() } : {}),
              payment_method_configuration_id: input.paymentMethodConfigurationId.trim(),
              product_tax_code: input.productTaxCode.trim(),
              purchases_enabled: input.isPurchasesEnabled,
            } as ControlStripeConfigRequest,
            {
              headers: buildControlMutationHeaders(mutation),
            },
          ),
          '保存 Stripe 设置失败',
        )
        return normalizeStripeConfiguration(data)
      } catch (error) {
        throw withControlCopy(error, '保存 Stripe 设置失败')
      }
    },
  }
}

function normalizeStripeConfiguration(
  configuration: ApiStripeConfigurationSummary,
): StripeConfigurationSummary {
  return {
    isConfigured: configuration.configured,
    isApiKeyConfigured: configuration.api_key_configured,
    isWebhookSecretConfigured: configuration.webhook_secret_configured,
    accountId: configuration.account_id,
    mode: configuration.mode,
    isChargesEnabled: configuration.charges_enabled,
    paymentMethodConfigurationId: configuration.payment_method_configuration_id ?? '',
    productTaxCode: configuration.product_tax_code,
    isProductReady: configuration.product_ready,
    isWebhookReady: configuration.webhook_ready,
    isPurchasesEnabled: configuration.purchases_enabled,
    updatedAt: configuration.updated_at,
  }
}
