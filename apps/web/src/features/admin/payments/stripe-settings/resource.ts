import {
  getControlStripeConfig,
  updateControlStripeConfig,
} from '../../../../shared/api/generated-query/control/control-payments/control-payments'
import type { ControlStripeConfigRequest } from '../../../../shared/api/generated-query/control/model/controlStripeConfigRequest'
import { requireContractData } from '../../../../shared/api/contractKernel'
import { buildRoutineControlMutationHeaders } from '../../shared/lib/controlMutation'
import { loadControlSetting } from '../../shared/lib/controlResource'
import { withControlCopy } from '../../shared/lib/controlCopy'
import type { AdminStripeSettingsResource, StripeConfigurationSummary } from './types'

// Admin Stripe settings boundary (W2 transports). Reads and writes go
// through the Control Plane payment operations with the contract envelope
// (`{data}` over the same configuration summary). Writes are routine config
// writes (backend ReasonOptional): they skip the operator-reason ceremony
// and audit an empty reason (mirrored into the body `reason` field) while
// keeping a header-only Idempotency-Key. The deleted
// `/api/admin/payments/stripe-config` route is gone; this module is the
// single transport.
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
      // Routine config write (backend ReasonOptional): the reason may be
      // empty (audit records it as-is) while Idempotency-Key stays required.
      const reason = mutation.operatorReason.trim()
      try {
        const data = requireContractData<ApiStripeConfigurationSummary>(
          await updateControlStripeConfig(
            'stripe',
            {
              ...(input.apiKey?.trim() ? { api_key: input.apiKey.trim() } : {}),
              payment_method_configuration_id: input.paymentMethodConfigurationId.trim(),
              product_tax_code: input.productTaxCode.trim(),
              purchases_enabled: input.purchasesEnabled,
              reason,
            } as ControlStripeConfigRequest,
            {
              headers: buildRoutineControlMutationHeaders(mutation),
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
    configured: configuration.configured,
    apiKeyConfigured: configuration.api_key_configured,
    webhookSecretConfigured: configuration.webhook_secret_configured,
    accountId: configuration.account_id,
    mode: configuration.mode,
    chargesEnabled: configuration.charges_enabled,
    paymentMethodConfigurationId: configuration.payment_method_configuration_id ?? '',
    productTaxCode: configuration.product_tax_code,
    productReady: configuration.product_ready,
    webhookReady: configuration.webhook_ready,
    purchasesEnabled: configuration.purchases_enabled,
    updatedAt: configuration.updated_at,
  }
}
