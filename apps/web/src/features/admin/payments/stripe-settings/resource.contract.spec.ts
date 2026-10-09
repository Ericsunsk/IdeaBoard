import { describe, expect, it } from 'vitest'
import { HttpResponse, http, useMswHandlers } from '../../../../test/msw'
import { createAdminStripeSettingsResource } from './resource'

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
      http.get('/api/control/v1/payments/providers/stripe', () =>
        HttpResponse.json({ data: responseData }),
      ),
    )

    await expect(createAdminStripeSettingsResource().get()).resolves.toEqual({
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

  it('omits an empty API Key while saving the non-secret Stripe settings with a reason', async () => {
    let seenHeaders: Record<string, string | null> = {}
    useMswHandlers(
      http.put('/api/control/v1/payments/providers/stripe', async ({ request }) => {
        expect(await request.json()).toEqual({
          payment_method_configuration_id: 'pmc_ideaboard',
          product_tax_code: 'txcd_10105001',
          purchases_enabled: true,
          reason: 'rotate test key',
        })
        seenHeaders = {
          idempotencyKey: request.headers.get('Idempotency-Key'),
          operatorReason: request.headers.get('X-Operator-Reason'),
        }
        return HttpResponse.json({ data: responseData })
      }),
    )

    await expect(
      createAdminStripeSettingsResource().save(
        {
          apiKey: '   ',
          paymentMethodConfigurationId: ' pmc_ideaboard ',
          productTaxCode: ' txcd_10105001 ',
          purchasesEnabled: true,
        },
        { operatorReason: 'rotate test key' },
      ),
    ).resolves.toMatchObject({
      apiKeyConfigured: true,
      webhookSecretConfigured: true,
      accountId: 'acct_123',
    })
    expect(seenHeaders.idempotencyKey).toMatch(/.+/)
    expect(seenHeaders.operatorReason).toBe('rotate test key')
  })

  it('requires an operator reason before saving', async () => {
    await expect(
      createAdminStripeSettingsResource().save(
        {
          paymentMethodConfigurationId: 'pmc_ideaboard',
          productTaxCode: 'txcd_10105001',
          purchasesEnabled: true,
        },
        { operatorReason: '   ' },
      ),
    ).rejects.toMatchObject({ code: 'control_reason_required' })
  })
})
