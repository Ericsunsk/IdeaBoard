import { resolveApiData } from '../../../../shared/api/apiResponse'
import {
  getAdminStripeConfiguration,
  updateAdminStripeConfiguration,
} from '../../../../shared/api/generated-query/admin-payments/admin-payments'
import type {
  StripeConfigurationSummary as ApiStripeConfigurationSummary,
  UpdateStripeConfigurationRequest,
} from '../../../../shared/api/generated-query/model'
import type {
  AdminStripeSettingsResource,
  SaveStripeConfigurationInput,
  StripeConfigurationSummary,
} from './stripeSettingsTypes'

export function createAdminStripeSettingsResource(): AdminStripeSettingsResource {
  return {
    get: getStripeConfiguration,
    save: saveStripeConfiguration,
  }
}

export async function getStripeConfiguration(): Promise<StripeConfigurationSummary> {
  const data = await resolveApiData(getAdminStripeConfiguration(), '加载 Stripe 设置失败')
  return normalizeStripeConfiguration(data)
}

export async function saveStripeConfiguration(
  input: SaveStripeConfigurationInput,
): Promise<StripeConfigurationSummary> {
  const body: UpdateStripeConfigurationRequest = {
    ...(input.apiKey?.trim() ? { api_key: input.apiKey.trim() } : {}),
    product_tax_code: input.productTaxCode.trim(),
    purchases_enabled: input.purchasesEnabled,
  }
  const data = await resolveApiData(updateAdminStripeConfiguration(body), '保存 Stripe 设置失败')
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
    productTaxCode: configuration.product_tax_code,
    productReady: configuration.product_ready,
    webhookReady: configuration.webhook_ready,
    purchasesEnabled: configuration.purchases_enabled,
    updatedAt: configuration.updated_at,
  }
}
