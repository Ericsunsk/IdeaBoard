import { api } from '../../../../shared/api/client'
import {
  resolveApiData,
  type ApiResponseWithData,
} from '../../../../shared/api/apiResponse'
import type {
  AdminStripeSettingsResource,
  SaveStripeConfigurationInput,
  StripeConfigurationSummary,
} from './types'

// Admin Stripe settings boundary (issue #809). The generated
// `admin-payments` client is gone but the backend admin plane still serves
// `/api/admin/payments/stripe-config` with the shared envelope, so this
// boundary transports through the central `api.request` with local wire
// types.
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

export function createAdminStripeSettingsResource(): AdminStripeSettingsResource {
  return {
    get: getStripeConfiguration,
    save: saveStripeConfiguration,
  }
}

export async function getStripeConfiguration(): Promise<StripeConfigurationSummary> {
  const data = await resolveApiData(
    api.request<ApiResponseWithData<ApiStripeConfigurationSummary>>(
      '/admin/payments/stripe-config',
      { method: 'GET' },
    ),
    '加载 Stripe 设置失败',
  )
  return normalizeStripeConfiguration(data)
}

export async function saveStripeConfiguration(
  input: SaveStripeConfigurationInput,
): Promise<StripeConfigurationSummary> {
  const body: UpdateStripeConfigurationRequest = {
    ...(input.apiKey?.trim() ? { api_key: input.apiKey.trim() } : {}),
    payment_method_configuration_id: input.paymentMethodConfigurationId.trim(),
    product_tax_code: input.productTaxCode.trim(),
    purchases_enabled: input.purchasesEnabled,
  }
  const data = await resolveApiData(
    api.request<ApiResponseWithData<ApiStripeConfigurationSummary>>(
      '/admin/payments/stripe-config',
      {
        method: 'PUT',
        body: body as unknown as Record<string, unknown>,
      },
    ),
    '保存 Stripe 设置失败',
  )
  return normalizeStripeConfiguration(data)
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
