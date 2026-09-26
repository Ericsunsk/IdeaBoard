import { describe, expect, it } from 'vitest'
import { HttpResponse, http, useMswHandlers } from '../../../../test/msw'
import { createAdminPaymentOrderApiAdapter } from './paymentOrderApiAdapter'
import { createAdminPaymentOrderResource } from './paymentOrderResource'

const orderNumber = 'IBP-20260803-ABCDEF123456'

describe('paymentOrderResource', () => {
  it('searches through the control plane with bounded filters', async () => {
    const seen: Array<{ url: string; headers: Record<string, string | null> }> = []
    useMswHandlers(
      http.get('/api/control/v1/payments/orders', ({ request }) => {
        const url = new URL(request.url)
        expect(url.searchParams.get('order_number')).toBe(orderNumber)
        expect(url.searchParams.get('user')).toBe('42')
        expect(url.searchParams.get('status')).toBe('paid_pending_credit')
        expect(url.searchParams.get('anomaly_only')).toBe('true')
        expect(url.searchParams.get('cursor')).toBe('cursor-2')
        expect(url.searchParams.get('limit')).toBe('20')
        seen.push({ url: request.url, headers: {} })
        return HttpResponse.json({
          data: {
            orders: [makeApiSummary()],
            total: 37,
            next_cursor: 'cursor-3',
          },
        })
      }),
    )

    const page = await createAdminPaymentOrderResource().search({
      orderNumber: ` ${orderNumber.toLowerCase()} `,
      user: ' 42 ',
      status: 'paid_pending_credit',
      anomalyOnly: true,
      cursor: 'cursor-2',
    })

    expect(seen).toHaveLength(1)
    expect(page).toEqual({
      orders: [
        expect.objectContaining({
          publicOrderNumber: orderNumber,
          stripeStatus: 'paid',
          creditStatus: 'pending',
          paidAmountMinor: 12_750,
          anomalousAt: '2026-08-02T01:00:00Z',
        }),
      ],
      total: 37,
      nextCursor: 'cursor-3',
    })
  })

  it('whitelists the safe detail DTO and omits unexpected Secret/provider payload fields', async () => {
    useMswHandlers(
      http.get('/api/control/v1/payments/orders/:orderNumber', ({ params }) => {
        expect(params.orderNumber).toBe(orderNumber)
        return HttpResponse.json({
          data: {
            ...makeApiDetail(),
            api_key: 'rk_live_secret',
            webhook_secret: 'whsec_secret',
            provider_response: { card_number: '4242424242424242' },
          },
        })
      }),
    )

    const detail = await createAdminPaymentOrderResource().get(orderNumber.toLowerCase())
    const serialized = JSON.stringify(detail)

    expect(detail.stripe).toEqual({
      accountId: 'acct…7890',
      customerId: 'cus_…7890',
      checkoutSessionId: 'cs_t…7890',
      paymentIntentId: 'pi_1…7890',
      chargeId: 'ch_1…7890',
    })
    expect(detail.reconciliation).toMatchObject({
      anomalous: true,
      lastErrorSummary: 'Stripe 暂时不可用',
    })
    expect(detail.refunds).toEqual([])
    expect(detail.disputes).toEqual([])
    expect(serialized).not.toContain('rk_live_secret')
    expect(serialized).not.toContain('whsec_secret')
    expect(serialized).not.toContain('4242424242424242')
  })

  it('creates refunds through control with the idempotency key and operator reason', async () => {
    let seenBody: unknown
    let seenHeaders: Record<string, string | null> = {}
    useMswHandlers(
      http.get('/api/control/v1/payments/orders/:orderNumber', () =>
        HttpResponse.json({ data: makeApiDetail() }),
      ),
      http.post('/api/control/v1/payments/orders/:orderNumber/refunds', async ({ request }) => {
        seenBody = await request.json()
        seenHeaders = {
          idempotencyKey: request.headers.get('Idempotency-Key'),
          operatorReason: request.headers.get('X-Operator-Reason'),
        }
        return HttpResponse.json({ data: { status: 'pending' } })
      }),
    )

    const adapter = createAdminPaymentOrderApiAdapter()
    await expect(
      adapter.refund(orderNumber, {
        idea_quantity: 500,
        idempotency_key: 'refund-1',
        operatorReason: 'duplicate charge',
      }),
    ).resolves.toMatchObject({ status: 'pending' })
    expect(seenBody).toEqual({ idea_quantity: 500, reason: 'duplicate charge' })
    expect(seenHeaders.idempotencyKey).toBe('refund-1')
    expect(seenHeaders.operatorReason).toBe('duplicate charge')
  })
})

function makeApiSummary() {
  return {
    public_order_number: orderNumber,
    user: {
      public_id: '00000000-0000-4000-8000-000000000042',
      display_name: 'Sunshi',
    },
    idea_quantity: 12_000,
    base_amount_minor: 12_000,
    base_currency: 'usd',
    paid_amount_minor: 12_750,
    paid_currency: 'hkd',
    status: 'paid_pending_credit',
    stripe_status: 'paid',
    credit_status: 'pending',
    created_at: '2026-08-01T01:00:00Z',
    paid_at: '2026-08-01T01:03:00Z',
    last_checked_at: '2026-08-03T01:00:00Z',
    anomalous_at: '2026-08-02T01:00:00Z',
  }
}

function makeApiDetail() {
  return {
    ...makeApiSummary(),
    paid_subtotal_minor: 12_000,
    paid_tax_minor: 750,
    paid_total_minor: 12_750,
    stripe: {
      account_id: 'acct…7890',
      customer_id: 'cus_…7890',
      checkout_session_id: 'cs_t…7890',
      payment_intent_id: 'pi_1…7890',
      charge_id: 'ch_1…7890',
    },
    terms_version: '2026-08-01',
    refund_policy_version: '2026-08-01',
    receipt_url: 'https://pay.stripe.com/receipts/example',
    expires_at: '2026-08-01T01:30:00Z',
    updated_at: '2026-08-03T01:00:00Z',
    reconciliation: {
      anomalous: true,
      anomalous_at: '2026-08-02T01:00:00Z',
      last_checked_at: '2026-08-03T01:00:00Z',
      next_attempt_at: '2026-08-03T01:05:00Z',
      last_error_code: 'stripe_unavailable',
      last_error_summary: 'Stripe 暂时不可用',
    },
  }
}
