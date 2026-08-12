import { describe, expect, it } from 'vitest'
import { HttpResponse, http, useMswHandlers } from '../../../../test/msw'
import { getStripeConfiguration, saveStripeConfiguration } from './stripeSettingsResource'

const responseData = {
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
      http.get('/api/admin/payments/stripe-config', () =>
        HttpResponse.json({ code: 0, msg: 'ok', data: responseData }),
      ),
    )

    await expect(getStripeConfiguration()).resolves.toEqual({
      configured: true,
      apiKeyConfigured: true,
      webhookSecretConfigured: true,
      accountId: 'acct_123',
      mode: 'live',
      chargesEnabled: true,
      paymentMethodConfigurationId: 'pmc_ideaboard',
      productTaxCode: 'txcd_10105001',
      productReady: true,
      webhookReady: true,
      purchasesEnabled: false,
      updatedAt: '2026-08-01T00:00:00Z',
    })
  })

  it('omits an empty API Key while saving the non-secret Stripe settings', async () => {
    useMswHandlers(
      http.put('/api/admin/payments/stripe-config', async ({ request }) => {
        expect(await request.json()).toEqual({
          payment_method_configuration_id: 'pmc_ideaboard',
          product_tax_code: 'txcd_10105001',
          purchases_enabled: true,
        })
        return HttpResponse.json({ code: 0, msg: 'ok', data: responseData })
      }),
    )

    await expect(
      saveStripeConfiguration({
        apiKey: '   ',
        paymentMethodConfigurationId: ' pmc_ideaboard ',
        productTaxCode: ' txcd_10105001 ',
        purchasesEnabled: true,
      }),
    ).resolves.toMatchObject({
      apiKeyConfigured: true,
      webhookSecretConfigured: true,
      accountId: 'acct_123',
    })
  })
})
