import {
  buildControlMutationHeaders,
  requireOperatorReason,
} from '../../shared/lib/controlMutation'
import {
  getControlStripeConfig,
  updateControlStripeConfig,
} from '../../../../shared/api/generated-query/control/control-payments/control-payments'
import { requireContractData } from '../../../../shared/api/contractKernel'
import { withSimpleControlCopy as withControlCopy } from '../../shared/lib/controlCopy'
import type {
  AdminStripeSettingsResource,
  SaveStripeConfigurationInput,
  StripeConfigurationSummary,
} from './types'

// Admin Stripe settings boundary (W2 transports). Reads and writes go
// through the Control Plane payment operations with the contract envelope
// (`{data}` over the same configuration summary); writes carry a
// header-only Idempotency-Key plus an operator reason (mirrored into the
// body `reason` field) and emit a control audit record. The deleted
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

export type UpdateStripeConfigurationRequest = {
  api_key?: string
  payment_method_configuration_id: string
  product_tax_code: string
  purchases_enabled: boolean
}

export type StripeSettingsMutationOptions = {
  operatorReason: string
  idempotencyKey?: string | undefined
}

export function createAdminStripeSettingsResource(): AdminStripeSettingsResource {
  return {
    get: getStripeConfiguration,
    save: saveStripeConfiguration,
  }
}

export async function getStripeConfiguration(): Promise<StripeConfigurationSummary> {
  try {
    const body = await getControlStripeConfig()
    return normalizeStripeConfiguration(
      requireContractData<ApiStripeConfigurationSummary>(body, '加载 Stripe 设置失败'),
    )
  } catch (error) {
    throw withControlCopy(error, '加载 Stripe 设置失败')
  }
}

export async function saveStripeConfiguration(
  input: SaveStripeConfigurationInput,
  options: StripeSettingsMutationOptions,
): Promise<StripeConfigurationSummary> {
  const operatorReason = requireOperatorReason(options.operatorReason)
  const body = {
    ...(input.apiKey?.trim() ? { api_key: input.apiKey.trim() } : {}),
    payment_method_configuration_id: input.paymentMethodConfigurationId.trim(),
    product_tax_code: input.productTaxCode.trim(),
    purchases_enabled: input.purchasesEnabled,
    reason: operatorReason,
  }
  try {
    const response = await updateControlStripeConfig(body, {
      headers: buildControlMutationHeaders(options),
    })
    return normalizeStripeConfiguration(
      requireContractData<ApiStripeConfigurationSummary>(response, '保存 Stripe 设置失败'),
    )
  } catch (error) {
    throw withControlCopy(error, '保存 Stripe 设置失败')
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

// withControlCopy is shared — see admin/shared/lib/controlCopy.ts.
