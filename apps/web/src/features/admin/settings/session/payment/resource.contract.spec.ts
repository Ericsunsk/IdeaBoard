import { describe, expect, it } from 'vitest'
import { HttpResponse, http, useMswHandlers } from '../../../../../test/msw'
import { createAdminStripeSettingsResource, type ApiStripeConfigurationSummary } from './resource'

const responseData: ApiStripeConfigurationSummary = {
  configured: true,
  api_key_configured: true,
  webhook_secret_configured: true,
  account_id: 'acct_123',
  mode: 'live',
  charges_enabled: true,
  payment_method_configuration_id: 'pmc_ideaboard',
  product_tax_code: 'txcd_10105001',
  product_ready: true,
  webhook_ready: true,
  purchases_enabled: false,
  updated_at: '2026-08-01T00:00:00Z',
}

describe('stripeSettingsResource', () => {
  it('maps the GET safe configuration summary without exposing either Secret', async () => {
    useMswHandlers(
      http.get('/api/control/v1/payments/providers/stripe', () =>
        HttpResponse.json({ data: responseData }),
      ),
    )

    await expect(createAdminStripeSettingsResource().get()).resolves.toEqual({
      isConfigured: true,
      isApiKeyConfigured: true,
      isWebhookSecretConfigured: true,
      accountId: 'acct_123',
      mode: 'live',
      isChargesEnabled: true,
      paymentMethodConfigurationId: 'pmc_ideaboard',
      productTaxCode: 'txcd_10105001',
      isProductReady: true,
      isWebhookReady: true,
      isPurchasesEnabled: false,
      updatedAt: '2026-08-01T00:00:00Z',
    })
  })

  it('omits an empty API Key while saving non-secret Stripe settings', async () => {
    let seenHeaders: Record<string, string | null> = {}
    useMswHandlers(
      http.put('/api/control/v1/payments/providers/stripe', async ({ request }) => {
        expect(await request.json()).toEqual({
          payment_method_configuration_id: 'pmc_ideaboard',
          product_tax_code: 'txcd_10105001',
          purchases_enabled: true,
        } satisfies Pick<ApiStripeConfigurationSummary, 'purchases_enabled'> &
          Record<string, unknown>)
        seenHeaders = {
          idempotencyKey: request.headers.get('Idempotency-Key'),
        }
        return HttpResponse.json({ data: responseData })
      }),
    )

    await expect(
      createAdminStripeSettingsResource().save({
        apiKey: '   ',
        paymentMethodConfigurationId: ' pmc_ideaboard ',
        productTaxCode: ' txcd_10105001 ',
        isPurchasesEnabled: true,
      }),
    ).resolves.toMatchObject({
      isApiKeyConfigured: true,
      isWebhookSecretConfigured: true,
      accountId: 'acct_123',
    })
    expect(seenHeaders.idempotencyKey).toMatch(/.+/)
  })
})
