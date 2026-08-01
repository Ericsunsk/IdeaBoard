export type StripeConfigurationMode = 'test' | 'live'

export type StripeConfigurationSummary = {
  configured: boolean
  apiKeyConfigured: boolean
  webhookSecretConfigured: boolean
  accountId?: string
  mode?: StripeConfigurationMode
  chargesEnabled: boolean
  productTaxCode: string
  productReady: boolean
  webhookReady: boolean
  purchasesEnabled: boolean
  updatedAt?: string
}

export type SaveStripeConfigurationInput = {
  apiKey?: string
  productTaxCode: string
  purchasesEnabled: boolean
}

export interface AdminStripeSettingsResource {
  get: () => Promise<StripeConfigurationSummary>
  save: (input: SaveStripeConfigurationInput) => Promise<StripeConfigurationSummary>
}
