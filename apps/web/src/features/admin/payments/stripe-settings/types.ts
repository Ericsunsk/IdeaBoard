export type StripeConfigurationMode = 'test' | 'live'

export type StripeConfigurationSummary = {
  configured: boolean
  apiKeyConfigured: boolean
  webhookSecretConfigured: boolean
  accountId?: string | undefined
  mode?: StripeConfigurationMode | undefined
  chargesEnabled: boolean
  paymentMethodConfigurationId: string
  productTaxCode: string
  productReady: boolean
  webhookReady: boolean
  purchasesEnabled: boolean
  updatedAt?: string | undefined
}

export type SaveStripeConfigurationInput = {
  apiKey?: string
  paymentMethodConfigurationId: string
  productTaxCode: string
  purchasesEnabled: boolean
}

export interface AdminStripeSettingsResource {
  get: () => Promise<StripeConfigurationSummary>
  save: (input: SaveStripeConfigurationInput) => Promise<StripeConfigurationSummary>
}
