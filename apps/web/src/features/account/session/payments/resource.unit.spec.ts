import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  parsePaymentEvent,
  paymentsQueryKeys,
  paymentsResource,
  subscribeToPaymentEvents,
} from './resource'
import { resetWorkspaceEventStreamsForTests } from '../../../../api/workspaceEventStream'

const transport = vi.hoisted(() => ({
  request: vi.fn(),
}))

vi.mock('../../../../api/client', () => ({
  api: { request: transport.request },
}))

const WORKSPACE_ID = 'ws_123'

function checkoutRequest() {
  return {
    idempotency_key: 'key-1',
    idea_quantity: 1000,
    terms_accepted: true as const,
    terms_version: 'v2026-01',
    refund_policy_version: 'v2026-01',
  }
}

describe('paymentsResource checkout', () => {
  beforeEach(() => vi.clearAllMocks())

  it('creates a checkout session through the v1 public plane', async () => {
    transport.request.mockResolvedValue({
      data: {
        session_id: 'b39a4c1e-0f2a-4b9e-9c1a-2b3c4d5e6f70',
        checkout_url: 'https://pay.example/checkout',
        expires_at: '2026-07-15T10:00:00Z',
        reused: false,
      },
    })

    const session = await paymentsResource.createCheckout(WORKSPACE_ID, checkoutRequest())

    expect(session).toMatchObject({
      checkout_url: 'https://pay.example/checkout',
      public_token: 'b39a4c1e-0f2a-4b9e-9c1a-2b3c4d5e6f70',
      reused: false,
    })
    expect(transport.request).toHaveBeenCalledWith(
      `/v1/workspaces/${WORKSPACE_ID}/checkout-sessions`,
      expect.objectContaining({
        method: 'POST',
        headers: expect.objectContaining({ 'Idempotency-Key': 'key-1' }),
      }),
    )
    const [, config] = transport.request.mock.calls[0] as [string, { body?: string }]
    expect(JSON.parse(config.body ?? '{}')).toMatchObject({
      data: { idea_quantity: 1000, terms_accepted: true },
    })
    expect(paymentsQueryKeys.returnStatus(WORKSPACE_ID, 'token')).toEqual([
      'workspace',
      'payments',
      'orders',
      'return',
      WORKSPACE_ID,
      'token',
    ])
  })

  it('reads the return status through the v1 checkout session', async () => {
    transport.request.mockResolvedValue({
      data: {
        session_id: 'token-1',
        status: 'credited',
        idea_quantity: 1000,
        expires_at: '2026-07-15T10:00:00Z',
      },
    })

    await expect(paymentsResource.getReturnStatus(WORKSPACE_ID, 'token-1')).resolves.toMatchObject({
      idea_quantity: 1000,
      status: 'credited',
    })
    expect(transport.request).toHaveBeenCalledWith(
      `/v1/workspaces/${WORKSPACE_ID}/checkout-sessions/token-1`,
      expect.objectContaining({ method: 'GET' }),
    )
  })

  it('reads purchase options through the v1 public plane', async () => {
    transport.request.mockResolvedValue({
      data: {
        purchases_available: true,
        quick_idea_quantities: [100, 500],
        min_idea_quantity: 100,
        max_idea_quantity: 100000,
        ideas_per_usd: 100,
        terms: { version: 'v2026-01', url: 'https://example/terms' },
        refund_policy: { version: 'v2026-01', url: 'https://example/refund' },
      },
    })

    await expect(paymentsResource.getPurchaseOptions(WORKSPACE_ID)).resolves.toMatchObject({
      purchases_available: true,
      min_idea_quantity: 100,
      terms: { version: 'v2026-01' },
    })
    expect(transport.request).toHaveBeenCalledWith(
      `/v1/workspaces/${WORKSPACE_ID}/purchase-options`,
      expect.objectContaining({ method: 'GET' }),
    )
  })

  it('lists pending orders through the v1 public plane', async () => {
    transport.request.mockResolvedValue({
      data: {
        items: [
          {
            public_token: 'tok-1',
            idea_quantity: 500,
            checkout_url: 'https://pay.example/c',
            status: 'checkout_open',
            expires_at: '2026-07-15T10:00:00Z',
            updated_at: '2026-07-15T09:00:00Z',
          },
        ],
      },
    })

    await expect(paymentsResource.listPendingOrders(WORKSPACE_ID)).resolves.toMatchObject({
      items: [{ public_token: 'tok-1', status: 'checkout_open' }],
    })
    expect(transport.request).toHaveBeenCalledWith(
      `/v1/workspaces/${WORKSPACE_ID}/pending-orders`,
      expect.objectContaining({ method: 'GET' }),
    )
  })

  it('continues and abandons pending orders with per-attempt idempotency keys', async () => {
    transport.request.mockResolvedValue({
      data: {
        public_token: 'tok-1',
        idea_quantity: 500,
        status: 'checkout_open',
        expires_at: '2026-07-15T10:00:00Z',
        updated_at: '2026-07-15T09:00:00Z',
      },
    })

    await expect(
      paymentsResource.continuePendingOrder(WORKSPACE_ID, 'tok-1'),
    ).resolves.toMatchObject({ public_token: 'tok-1', status: 'checkout_open' })
    expect(transport.request).toHaveBeenCalledWith(
      `/v1/workspaces/${WORKSPACE_ID}/pending-orders/tok-1/continue`,
      expect.objectContaining({
        method: 'POST',
        headers: expect.objectContaining({
          'Idempotency-Key': expect.stringMatching(/^billing:pending-continue:/),
        }),
      }),
    )

    await expect(
      paymentsResource.abandonPendingOrder(WORKSPACE_ID, 'tok-1'),
    ).resolves.toMatchObject({ public_token: 'tok-1' })
    expect(transport.request).toHaveBeenCalledWith(
      `/v1/workspaces/${WORKSPACE_ID}/pending-orders/tok-1/abandon`,
      expect.objectContaining({
        method: 'POST',
        headers: expect.objectContaining({
          'Idempotency-Key': expect.stringMatching(/^billing:pending-abandon:/),
        }),
      }),
    )
  })

  it('lists recharge history with cursor pagination through the v1 public plane', async () => {
    transport.request.mockResolvedValue({
      data: [
        {
          occurred_at: '2026-07-15T10:00:00Z',
          idea_quantity: 500,
          paid_amount_minor: 500,
          paid_currency: 'USD',
          status: 'credited',
        },
      ],
      meta: { next_cursor: 'cursor-next', has_more: true },
    })

    await expect(
      paymentsResource.listRecharges(WORKSPACE_ID, { cursor: 'cursor-prev', limit: 20 }),
    ).resolves.toMatchObject({
      items: [{ idea_quantity: 500, status: 'credited' }],
      next_cursor: 'cursor-next',
    })
    expect(transport.request).toHaveBeenCalledWith(
      `/v1/workspaces/${WORKSPACE_ID}/recharges?cursor=cursor-prev&page_size=20`,
      expect.objectContaining({ method: 'GET' }),
    )
  })
})

describe('paymentsResource payment events', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
    resetWorkspaceEventStreamsForTests()
  })

  it('accepts only the minimal typed payment event contract', () => {
    expect(parsePaymentEvent('{"public_token":"token","status":"credited"}')).toEqual({
      public_token: 'token',
      status: 'credited',
    })
    expect(parsePaymentEvent('{"status":"refund_updated"}')).toEqual({
      status: 'refund_updated',
    })
    expect(parsePaymentEvent('{"status":"dispute_updated"}')).toEqual({
      status: 'dispute_updated',
    })
    expect(parsePaymentEvent('{"status":"unknown"}')).toBeNull()
    expect(parsePaymentEvent('{"public_token":42,"status":"failed"}')).toBeNull()
    expect(parsePaymentEvent('not-json')).toBeNull()
  })

  it('maps workspace billing settlement to the payment event contract', () => {
    const listener = vi.fn()
    const handlers = new Map<string, EventListener>()
    const source = {
      addEventListener: vi.fn((event: string, handler: EventListener) => {
        handlers.set(event, handler)
      }),
      removeEventListener: vi.fn(),
      close: vi.fn(),
    }
    vi.stubGlobal(
      'EventSource',
      vi.fn(function MockEventSource() {
        return source
      }),
    )

    const unsubscribe = subscribeToPaymentEvents(WORKSPACE_ID, listener)
    const settled = handlers.get('billing.settled')
    if (!settled) throw new Error('billing.settled listener was not registered')
    settled(
      new MessageEvent('billing.settled', {
        data: JSON.stringify({
          id: 'evt_abc123',
          type: 'billing.settled',
          version: 1,
          occurred_at: '2026-07-15T10:00:00Z',
          actor: { type: 'user', id: 'usr_1' },
          resource: { type: 'checkout_session', id: 'sess_9' },
          workspace_id: WORKSPACE_ID,
          data: {},
        }),
      }),
    )
    expect(listener).toHaveBeenCalledWith({ status: 'credited', public_token: 'sess_9' })

    unsubscribe()
    expect(source.close).toHaveBeenCalledOnce()
  })

  it('stays unsubscribed without a workspace selection', () => {
    const createEventSource = vi.fn()
    vi.stubGlobal('EventSource', createEventSource)
    const listener = vi.fn()

    const unsubscribe = subscribeToPaymentEvents('', listener)

    expect(createEventSource).not.toHaveBeenCalled()
    expect(listener).not.toHaveBeenCalled()
    unsubscribe()
  })
})
