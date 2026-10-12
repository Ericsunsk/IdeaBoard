type StripeConfigurationMode = 'test' | 'live'

export type StripeConfigurationSummary = {
  isConfigured: boolean
  isApiKeyConfigured: boolean
  isWebhookSecretConfigured: boolean
  accountId?: string | undefined
  mode?: StripeConfigurationMode | undefined
  isChargesEnabled: boolean
  paymentMethodConfigurationId: string
  productTaxCode: string
  isProductReady: boolean
  isWebhookReady: boolean
  isPurchasesEnabled: boolean
  updatedAt?: string | undefined
}

type SaveStripeConfigurationInput = {
  apiKey?: string
  paymentMethodConfigurationId: string
  productTaxCode: string
  isPurchasesEnabled: boolean
}

export interface AdminStripeSettingsResource {
  get: () => Promise<StripeConfigurationSummary>
  save: (
    input: SaveStripeConfigurationInput,
    options: { operatorReason: string; idempotencyKey?: string | undefined },
  ) => Promise<StripeConfigurationSummary>
}
