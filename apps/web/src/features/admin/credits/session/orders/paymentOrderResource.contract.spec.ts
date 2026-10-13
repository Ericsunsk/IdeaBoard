import { describe, expect, it } from 'vitest'
import { HttpResponse, http, useMswHandlers } from '../../../../../test/msw'
import {
  adminPaymentOrderNumber as orderNumber,
  makePaymentOrderDetail as makeApiDetail,
  makePaymentOrderSummary as makeApiSummary,
} from '../../../../../test/factories'
import { createAdminPaymentOrderApiAdapter } from './paymentOrderApiAdapter'
import { createAdminPaymentOrderResource } from './paymentOrderResource'

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

  it('creates refunds through control with the idempotency key with idempotency headers', async () => {
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
        }
        return HttpResponse.json({ data: { status: 'pending' } })
      }),
    )

    const adapter = createAdminPaymentOrderApiAdapter()
    await expect(
      adapter.refund(orderNumber, {
        idea_quantity: 500,
        idempotency_key: 'refund-1',
      }),
    ).resolves.toMatchObject({ status: 'pending' })
    expect(seenBody).toEqual({ idea_quantity: 500 })
    expect(seenHeaders.idempotencyKey).toBe('refund-1')
  })
})
