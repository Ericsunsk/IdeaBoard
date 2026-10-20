import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { renderWithUser, screen, waitFor, within } from '../../../../test/render'
import { mockAppToast, type AppToastMocks } from '../../../../test/toast'
import type { PaymentReturnContext } from '../../core/return'
import {
  paymentsContextKey,
  type PaymentPurchaseOptions,
  type PaymentsContext,
  type PaymentsResource,
} from '../../session/payments/resource'
import { creditsQueryKeys } from '../../session/credits/resource'
import RechargePane from './RechargePane.vue'
import { ref } from 'vue'

vi.mock('../../session/accountWorkspace', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../session/accountWorkspace')>()),
  useAccountWorkspace: () => ({
    workspaceId: ref('ws_1'),
    isSelectionSettled: ref(true),
  }),
}))

vi.mock('../../../../core/composables/useAppToast')

const purchaseOptions = {
  purchases_available: true,
  quick_idea_quantities: [1000, 2000, 3000, 5000, 10_000],
  min_idea_quantity: 500,
  max_idea_quantity: 500_000,
  ideas_per_usd: 100,
  terms: { version: '2026-08-01', url: '/terms' },
  refund_policy: { version: '2026-08-01', url: '/refund-policy' },
} as const

function makeResource(overrides: Partial<PaymentsResource> = {}): PaymentsResource {
  return {
    getPurchaseOptions: vi.fn().mockResolvedValue(purchaseOptions),
    createCheckout: vi.fn().mockResolvedValue({
      checkout_url: 'https://checkout.stripe.com/c/pay/cs_test_123',
      public_token: '0198cd29-f457-7e4b-b95e-6cfde41b7222',
      expires_at: '2026-08-01T01:00:00Z',
      reused: false,
    }),
    getReturnStatus: vi.fn().mockResolvedValue({
      idea_quantity: 1000,
      status: 'processing',
      updated_at: '2026-08-01T00:30:00Z',
    }),
    listPendingOrders: vi.fn().mockResolvedValue({ items: [] }),
    continuePendingOrder: vi.fn(),
    abandonPendingOrder: vi.fn(),
    listRecharges: vi.fn().mockResolvedValue({ items: [] }),
    subscribe: vi.fn(() => () => undefined),
    ...overrides,
  }
}

function renderPane(
  options: {
    resource?: PaymentsResource
    paymentReturn?: PaymentReturnContext | null
    navigate?: PaymentsContext['navigateToCheckout']
  } = {},
) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  const context: PaymentsContext = {
    resource: options.resource ?? makeResource(),
    navigateToCheckout: options.navigate ?? vi.fn(),
  }
  return {
    context,
    queryClient,
    ...renderWithUser(RechargePane, {
      props: { isActive: true, paymentReturn: options.paymentReturn ?? null },
      global: {
        plugins: [[VueQueryPlugin, { queryClient }]],
        provide: { [paymentsContextKey as symbol]: context },
      },
    }),
  }
}

describe('RechargePane', () => {
  let toast: AppToastMocks

  beforeEach(() => {
    vi.clearAllMocks()
    toast = mockAppToast()
  })

  it('renders the canonical quick packages with exact two-decimal USD prices', async () => {
    renderPane()

    expect(await screen.findByRole('heading', { name: 'Top up' })).toBeVisible()
    expect(await screen.findByText('$1.00 USD = 100 Ideas')).toBeVisible()
    const packages = await screen.findAllByRole('button', { name: /Ideas.*USD/ })
    expect(packages).toHaveLength(5)
    expect(packages[0]).toHaveAccessibleName('1,000 Ideas $10.00 USD')
    expect(packages[1]).toHaveAccessibleName('2,000 Ideas $20.00 USD')
    expect(packages[2]).toHaveAccessibleName('3,000 Ideas $30.00 USD')
    expect(packages[3]).toHaveAccessibleName('5,000 Ideas $50.00 USD')
    expect(packages[4]).toHaveAccessibleName('10,000 Ideas $100.00 USD')
    expect(packages[0]).toHaveAttribute('aria-pressed', 'true')
  })

  it('announces purchase-option loading to assistive technology', () => {
    const resource = makeResource({
      getPurchaseOptions: vi.fn(() => new Promise<PaymentPurchaseOptions>(() => undefined)),
    })
    renderPane({ resource })

    expect(screen.getByRole('status', { name: 'Loading top-up options…' })).toBeVisible()
  })

  it('uses the payment action as versioned legal acceptance and redirects once', async () => {
    const createCheckout = vi.fn().mockResolvedValue({
      checkout_url: 'https://checkout.stripe.com/c/pay/cs_test_123',
      public_token: '0198cd29-f457-7e4b-b95e-6cfde41b7222',
      expires_at: '2026-08-01T01:00:00Z',
      reused: false,
    })
    const navigate = vi.fn()
    const { user } = renderPane({ resource: makeResource({ createCheckout }), navigate })

    const checkout = await screen.findByRole('button', { name: 'Pay now' })
    expect(checkout).toBeEnabled()
    expect(screen.queryByRole('checkbox')).not.toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Terms of Service' })).toHaveAttribute('href', '/terms')
    expect(screen.getByRole('link', { name: 'Refund Policy' })).toHaveAttribute(
      'href',
      '/refund-policy',
    )
    await user.click(checkout)

    await waitFor(() => {
      expect(createCheckout).toHaveBeenCalledWith('ws_1', {
        idea_quantity: 1000,
        idempotency_key: expect.stringMatching(
          /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i,
        ),
        refund_policy_version: '2026-08-01',
        terms_accepted: true,
        terms_version: '2026-08-01',
      })
    })
    expect(navigate).toHaveBeenCalledOnce()
    expect(navigate).toHaveBeenCalledWith('https://checkout.stripe.com/c/pay/cs_test_123')
  })

  it('accepts any in-range custom integer and keeps exact cents', async () => {
    const createCheckout = vi.fn().mockResolvedValue({
      checkout_url: 'https://checkout.stripe.com/c/pay/cs_test_custom',
      public_token: '0198cd29-f457-7e4b-b95e-6cfde41b7222',
      expires_at: '2026-08-01T01:00:00Z',
      reused: false,
    })
    const { user } = renderPane({ resource: makeResource({ createCheckout }) })
    const custom = await screen.findByRole('textbox', { name: 'Custom amount' })

    await user.type(custom, '501')
    expect(screen.getByText('$5.01 USD')).toBeVisible()
    await user.click(screen.getByRole('button', { name: 'Pay now' }))

    expect(createCheckout).toHaveBeenCalledWith('ws_1', {
      idea_quantity: 501,
      idempotency_key: expect.any(String),
      refund_policy_version: '2026-08-01',
      terms_accepted: true,
      terms_version: '2026-08-01',
    })
  })

  it('normalizes custom quantities without inline validation errors', async () => {
    const { user } = renderPane()
    const custom = await screen.findByRole('textbox', { name: 'Custom amount' })

    await user.type(custom, '499.9')
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
    expect(custom).toHaveValue('499.9')
    expect(screen.getByText('$5.00 USD')).toBeVisible()
    await user.tab()

    expect(custom).toHaveValue('500')
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
    await user.clear(custom)
    await user.type(custom, '501.9')
    await user.tab()
    expect(custom).toHaveValue('501')
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Terms of Service' })).toHaveAttribute('href', '/terms')
    expect(screen.getByRole('link', { name: 'Refund Policy' })).toHaveAttribute(
      'href',
      '/refund-policy',
    )
  })

  it('shows only necessary recharge fields and a safe Stripe receipt link', async () => {
    const resource = makeResource({
      listRecharges: vi.fn().mockResolvedValue({
        items: [
          {
            occurred_at: '2026-08-01T09:08:00Z',
            idea_quantity: 2000,
            paid_amount_minor: 15_650,
            paid_currency: 'hkd',
            status: 'credited',
            receipt_url: 'https://pay.stripe.com/receipts/demo',
          },
          {
            occurred_at: '2026-08-01T08:08:00Z',
            idea_quantity: 1000,
            paid_amount_minor: 1000,
            paid_currency: 'usd',
            status: 'credited',
            refunded_idea_quantity: 250,
            refunded_amount_minor: 250,
          },
          {
            occurred_at: '2026-08-01T07:08:00Z',
            idea_quantity: 500,
            paid_amount_minor: 500,
            paid_currency: 'usd',
            status: 'refunded',
            refunded_idea_quantity: 500,
            refunded_amount_minor: 500,
          },
          {
            occurred_at: '2026-08-01T06:08:00Z',
            idea_quantity: 750,
            status: 'cancelled',
          },
        ],
      }),
    })
    renderPane({ resource })

    const table = await screen.findByRole('table', { name: 'Top-up history table' })
    expect(
      within(table)
        .getAllByRole('columnheader')
        .map((item) => item.textContent),
    ).toEqual(['Time', 'Ideas', 'Amount paid', 'Status', 'Receipt'])
    expect(await within(table).findByText('2,000')).toBeVisible()
    expect(within(table).getByText('HK$156.50 HKD')).toBeVisible()
    expect(within(table).getByText('Added')).toBeVisible()
    expect(within(table).getByText('Added · $2.50 USD refunded')).toBeVisible()
    expect(within(table).getByText('Refunded')).toBeVisible()
    expect(within(table).getByText('Unpaid')).toBeVisible()
    expect(within(table).getByText('Cancelled')).toBeVisible()
    expect(within(table).getByRole('link', { name: 'View receipt' })).toHaveAttribute(
      'href',
      'https://pay.stripe.com/receipts/demo',
    )
    expect(screen.queryByText(/customer|session|email/i)).not.toBeInTheDocument()
  })

  it('moves an abandoned Checkout order from pending orders into recharge history', async () => {
    const continuePendingOrder = vi.fn().mockResolvedValue({
      public_token: 'open-order',
      idea_quantity: 1000,
      status: 'checkout_open',
      checkout_url: 'https://checkout.stripe.com/c/pay/open-order',
      expires_at: '2026-08-01T01:00:00Z',
      updated_at: '2026-08-01T00:00:00Z',
    })
    const abandonPendingOrder = vi.fn().mockResolvedValue({
      public_token: 'open-order',
      idea_quantity: 1000,
      status: 'cancelled',
      expires_at: '2026-08-01T01:00:00Z',
      updated_at: '2026-08-01T00:00:00Z',
    })
    const listRecharges = vi
      .fn()
      .mockResolvedValueOnce({ items: [] })
      .mockResolvedValueOnce({ items: [] })
      .mockResolvedValue({
        items: [
          {
            occurred_at: '2026-08-01T00:10:00Z',
            idea_quantity: 1000,
            status: 'cancelled',
          },
        ],
      })
    const navigate = vi.fn()
    const { user } = renderPane({
      navigate,
      resource: makeResource({
        continuePendingOrder,
        abandonPendingOrder,
        listRecharges,
        // Server truth: the order stays listed until abandon, then leaves the
        // pending list, so the post-action invalidation refetches resolve so.
        listPendingOrders: vi
          .fn()
          .mockResolvedValueOnce({
            items: [
              {
                public_token: 'open-order',
                idea_quantity: 1000,
                status: 'checkout_open',
                checkout_url: 'https://checkout.stripe.com/c/pay/open-order',
                expires_at: '2026-08-01T01:00:00Z',
                updated_at: '2026-08-01T00:00:00Z',
              },
            ],
          })
          .mockResolvedValueOnce({
            items: [
              {
                public_token: 'open-order',
                idea_quantity: 1000,
                status: 'checkout_open',
                checkout_url: 'https://checkout.stripe.com/c/pay/open-order',
                expires_at: '2026-08-01T01:00:00Z',
                updated_at: '2026-08-01T00:00:00Z',
              },
            ],
          })
          .mockResolvedValue({ items: [] }),
      }),
    })

    const pendingOrder = await screen.findByTestId('pending-payment-order-open-order')
    expect(within(pendingOrder).getByText('1,000 Ideas')).toBeVisible()
    expect(within(pendingOrder).getByText('Awaiting payment')).toBeVisible()
    expect(within(pendingOrder).getByText(/Valid until/)).toBeVisible()
    expect(screen.getByRole('heading', { name: 'Pending orders' })).toBeVisible()
    const historyTable = await screen.findByRole('table', { name: 'Top-up history table' })
    expect(within(historyTable).queryByText('1,000')).not.toBeInTheDocument()

    await user.click(within(pendingOrder).getByRole('button', { name: 'Continue payment' }))
    await waitFor(() => expect(continuePendingOrder).toHaveBeenCalledWith('ws_1', 'open-order'))
    expect(navigate).toHaveBeenCalledWith('https://checkout.stripe.com/c/pay/open-order')

    await user.click(within(pendingOrder).getByRole('button', { name: 'Abandon order' }))
    await waitFor(() => expect(abandonPendingOrder).toHaveBeenCalledWith('ws_1', 'open-order'))
    await waitFor(() =>
      expect(screen.queryByTestId('pending-payment-order-open-order')).not.toBeInTheDocument(),
    )
    expect(screen.queryByRole('button', { name: '确认放弃' })).not.toBeInTheDocument()
    expect(await within(historyTable).findByText('Cancelled')).toBeVisible()
    expect(within(historyTable).getByText('Unpaid')).toBeVisible()
  })

  it('refetches the pending list after a pending-order action even with no cached entry', async () => {
    // Guards the mutation/SSE parity: `pendingActionMutation.onSuccess` must
    // invalidate the pending root like the SSE path does. Surgical
    // `setQueryData` alone never refetches (and is a no-op when the list was
    // never cached), so the list would go stale without this invalidation.
    const abandonPendingOrder = vi.fn().mockResolvedValue({
      public_token: 'open-order',
      idea_quantity: 1000,
      status: 'cancelled',
      expires_at: '2026-08-01T01:00:00Z',
      updated_at: '2026-08-01T00:00:00Z',
    })
    const listPendingOrders = vi.fn().mockResolvedValue({
      items: [
        {
          public_token: 'open-order',
          idea_quantity: 1000,
          status: 'checkout_open',
          checkout_url: 'https://checkout.stripe.com/c/pay/open-order',
          expires_at: '2026-08-01T01:00:00Z',
          updated_at: '2026-08-01T00:00:00Z',
        },
      ],
    })
    const listRecharges = vi.fn().mockResolvedValue({ items: [] })
    const { user } = renderPane({
      resource: makeResource({ abandonPendingOrder, listPendingOrders, listRecharges }),
    })

    const pendingOrder = await screen.findByTestId('pending-payment-order-open-order')
    await user.click(within(pendingOrder).getByRole('button', { name: 'Abandon order' }))

    await waitFor(() => expect(abandonPendingOrder).toHaveBeenCalledWith('ws_1', 'open-order'))
    await waitFor(() => expect(listPendingOrders).toHaveBeenCalledTimes(2))
    await waitFor(() => expect(listRecharges).toHaveBeenCalledTimes(2))
  })

  it('renders waiting, delayed credit processing, and expired pending order states', async () => {
    renderPane({
      resource: makeResource({
        listPendingOrders: vi.fn().mockResolvedValue({
          items: [
            {
              public_token: 'async-order',
              idea_quantity: 2000,
              status: 'async_payment_pending',
              expires_at: '2026-08-01T01:00:00Z',
              updated_at: '2026-08-01T00:00:00Z',
            },
            {
              public_token: 'credit-order',
              idea_quantity: 3000,
              status: 'paid_pending_credit',
              expires_at: '2026-08-01T01:00:00Z',
              updated_at: new Date(Date.now() - 61_000).toISOString(),
            },
            {
              public_token: 'expired-order',
              idea_quantity: 5000,
              status: 'expired',
              expires_at: '2026-07-31T01:00:00Z',
              updated_at: '2026-07-31T01:00:00Z',
            },
          ],
        }),
      }),
    })

    expect(await screen.findByText('Waiting for payment confirmation')).toBeVisible()
    expect(screen.getByText('Processing Ideas')).toBeVisible()
    expect(screen.getByText('Payment link expired')).toBeVisible()
    expect(screen.getByText(/Expired at .*/)).toBeVisible()
  })

  it('makes pending order failures visible and retryable', async () => {
    const continuePendingOrder = vi
      .fn()
      .mockRejectedValueOnce(new Error('待支付订单已达到 3 笔上限，请先完成或放弃现有订单'))
      .mockResolvedValueOnce({
        public_token: 'open-order',
        idea_quantity: 1000,
        status: 'checkout_open',
        checkout_url: 'https://checkout.stripe.com/c/pay/open-order',
        expires_at: '2026-08-01T01:00:00Z',
        updated_at: '2026-08-01T00:00:00Z',
      })
    const { user } = renderPane({
      resource: makeResource({
        continuePendingOrder,
        listPendingOrders: vi.fn().mockResolvedValue({
          items: [
            {
              public_token: 'open-order',
              idea_quantity: 1000,
              status: 'checkout_open',
              expires_at: '2026-08-01T01:00:00Z',
              updated_at: '2026-08-01T00:00:00Z',
            },
          ],
        }),
      }),
    })

    const pendingOrder = await screen.findByTestId('pending-payment-order-open-order')
    await user.click(within(pendingOrder).getByRole('button', { name: 'Continue payment' }))
    expect(await within(pendingOrder).findByRole('alert')).toHaveTextContent(
      'Network error. Please try again.',
    )
    // Dual: inline alert stays as position anchor, toast mirrors it transiently.
    await waitFor(() =>
      expect(toast.error).toHaveBeenCalledWith('Could not continue this payment.'),
    )
    await user.click(within(pendingOrder).getByRole('button', { name: 'Retry continue payment' }))
    await waitFor(() => expect(continuePendingOrder).toHaveBeenCalledTimes(2))
  })

  it('mirrors abandon-order failures in a toast while keeping the inline retry', async () => {
    const abandonPendingOrder = vi.fn().mockRejectedValueOnce(new Error('abandon failed'))
    const { user } = renderPane({
      resource: makeResource({
        abandonPendingOrder,
        listPendingOrders: vi.fn().mockResolvedValue({
          items: [
            {
              public_token: 'open-order',
              idea_quantity: 1000,
              status: 'checkout_open',
              expires_at: '2026-08-01T01:00:00Z',
              updated_at: '2026-08-01T00:00:00Z',
            },
          ],
        }),
      }),
    })

    const pendingOrder = await screen.findByTestId('pending-payment-order-open-order')
    await user.click(within(pendingOrder).getByRole('button', { name: 'Abandon order' }))
    expect(await within(pendingOrder).findByRole('alert')).toHaveTextContent(
      'Network error. Please try again.',
    )
    await waitFor(() => expect(toast.error).toHaveBeenCalledWith('Could not abandon this order.'))
  })

  it('shows the backend-owned processing result after a Checkout return', async () => {
    renderPane({
      paymentReturn: {
        publicToken: '0198cd29-f457-7e4b-b95e-6cfde41b7222',
        source: 'success',
      },
    })

    const status = (await screen.findByText('Confirming your payment result')).closest(
      '[role="status"]',
    )!
    expect(status).toHaveTextContent('The final result depends on payment provider confirmation')
    await waitFor(() => expect(status).toHaveFocus())
  })

  it('shows confirmed credit success and does not infer it from the return URL', async () => {
    renderPane({
      paymentReturn: {
        publicToken: '0198cd29-f457-7e4b-b95e-6cfde41b7222',
        source: 'cancel',
      },
      resource: makeResource({
        getReturnStatus: vi.fn().mockResolvedValue({
          idea_quantity: 3000,
          status: 'credited',
          updated_at: '2026-08-01T00:30:00Z',
        }),
      }),
    })

    const status = (await screen.findByText('3,000 Ideas added')).closest('[role="status"]')!
    expect(status).toHaveTextContent('3,000 Ideas added')
    expect(status).toHaveTextContent('Your balance and top-up history are up to date')
  })

  it('loads opaque cursor pages in batches of 20', async () => {
    const listRecharges = vi
      .fn()
      .mockResolvedValueOnce({
        items: [
          {
            occurred_at: '2026-08-01T09:08:00Z',
            idea_quantity: 1000,
            paid_amount_minor: 1000,
            paid_currency: 'usd',
            status: 'credited',
          },
        ],
        next_cursor: 'cursor_opaque_page_2',
      })
      .mockResolvedValueOnce({
        items: [
          {
            occurred_at: '2026-07-01T09:08:00Z',
            idea_quantity: 2000,
            paid_amount_minor: 2000,
            paid_currency: 'usd',
            status: 'credited',
          },
        ],
      })
    const { user } = renderPane({ resource: makeResource({ listRecharges }) })

    const table = await screen.findByRole('table', { name: 'Top-up history table' })
    expect(await within(table).findByText('$10.00 USD')).toBeVisible()
    await user.click(screen.getByRole('button', { name: 'Load more' }))
    expect(await within(table).findByText('$20.00 USD')).toBeVisible()
    expect(listRecharges).toHaveBeenNthCalledWith(1, 'ws_1', { limit: 20 }, expect.any(AbortSignal))
    expect(listRecharges).toHaveBeenNthCalledWith(
      2,
      'ws_1',
      { cursor: 'cursor_opaque_page_2', limit: 20 },
      expect.any(AbortSignal),
    )
  })

  it('uses the user payment event only to invalidate visible account queries', async () => {
    let paymentListener: Parameters<PaymentsResource['subscribe']>[1] | undefined
    const unsubscribe = vi.fn()
    const listPendingOrders = vi.fn().mockResolvedValue({ items: [] })
    const listRecharges = vi
      .fn()
      .mockResolvedValueOnce({ items: [] })
      .mockResolvedValueOnce({
        items: [
          {
            occurred_at: '2026-08-01T00:30:00Z',
            idea_quantity: 1000,
            paid_amount_minor: 1000,
            paid_currency: 'usd',
            status: 'credited',
          },
        ],
      })
    const subscribe = vi.fn(
      (_workspaceId: string, listener: Parameters<PaymentsResource['subscribe']>[1]) => {
        paymentListener = listener
        return unsubscribe
      },
    )
    const { unmount } = renderPane({
      resource: makeResource({ listPendingOrders, listRecharges, subscribe }),
    })
    expect(await screen.findByText('No top-ups yet')).toBeVisible()
    expect(listRecharges).toHaveBeenCalledOnce()

    paymentListener?.({ status: 'credited' })

    await waitFor(() => expect(listRecharges).toHaveBeenCalledTimes(2))
    await waitFor(() => expect(listPendingOrders).toHaveBeenCalledTimes(2))
    const historyTable = await screen.findByRole('table', { name: 'Top-up history table' })
    expect(await within(historyTable).findByText('$10.00 USD')).toBeVisible()
    unmount()
    expect(unsubscribe).toHaveBeenCalledOnce()
  })

  it('keeps reconciliation processing visible, then refreshes the wallet and history after credit', async () => {
    let paymentListener: Parameters<PaymentsResource['subscribe']>[1] | undefined
    const processingOrder = {
      public_token: 'reconciling-order',
      idea_quantity: 3000,
      status: 'paid_pending_credit' as const,
      expires_at: '2026-08-01T01:00:00Z',
      updated_at: new Date(Date.now() - 61_000).toISOString(),
    }
    const listPendingOrders = vi
      .fn()
      .mockResolvedValueOnce({ items: [processingOrder] })
      .mockResolvedValueOnce({ items: [processingOrder] })
      .mockResolvedValueOnce({ items: [] })
    const listRecharges = vi
      .fn()
      .mockResolvedValueOnce({ items: [] })
      .mockResolvedValueOnce({ items: [] })
      .mockResolvedValueOnce({
        items: [
          {
            occurred_at: '2026-08-01T00:30:00Z',
            idea_quantity: 3000,
            paid_amount_minor: 3000,
            paid_currency: 'usd',
            status: 'credited',
          },
        ],
      })
    const subscribe = vi.fn(
      (_workspaceId: string, listener: Parameters<PaymentsResource['subscribe']>[1]) => {
        paymentListener = listener
        return () => undefined
      },
    )
    const { queryClient } = renderPane({
      resource: makeResource({ listPendingOrders, listRecharges, subscribe }),
    })
    const invalidateQueries = vi.spyOn(queryClient, 'invalidateQueries')

    expect(await screen.findByText('Processing Ideas')).toBeVisible()

    paymentListener?.({ public_token: 'reconciling-order', status: 'processing' })

    await waitFor(() => expect(listPendingOrders).toHaveBeenCalledTimes(2))
    expect(screen.getByText('Processing Ideas')).toBeVisible()

    paymentListener?.({ public_token: 'reconciling-order', status: 'credited' })

    await waitFor(() => expect(listPendingOrders).toHaveBeenCalledTimes(3))
    const historyTable = await screen.findByRole('table', { name: 'Top-up history table' })
    expect(await within(historyTable).findByText('$30.00 USD')).toBeVisible()
    expect(screen.queryByText('Processing Ideas')).not.toBeInTheDocument()
    expect(invalidateQueries).toHaveBeenCalledWith({ queryKey: creditsQueryKeys.all })
  })

  it('keeps a final reconciliation failure visible while allowing a replacement purchase', async () => {
    let paymentListener: Parameters<PaymentsResource['subscribe']>[1] | undefined
    const createCheckout = vi.fn().mockResolvedValue({
      checkout_url: 'https://checkout.stripe.com/c/pay/replacement-order',
      public_token: 'replacement-order',
      expires_at: '2026-08-01T01:00:00Z',
      reused: false,
    })
    const subscribe = vi.fn(
      (_workspaceId: string, listener: Parameters<PaymentsResource['subscribe']>[1]) => {
        paymentListener = listener
        return () => undefined
      },
    )
    const { user } = renderPane({
      resource: makeResource({
        createCheckout,
        subscribe,
        listPendingOrders: vi.fn().mockResolvedValue({
          items: [
            {
              public_token: 'failed-reconciliation-order',
              idea_quantity: 2000,
              status: 'paid_pending_credit',
              expires_at: '2026-08-01T01:00:00Z',
              updated_at: new Date(Date.now() - 61_000).toISOString(),
            },
          ],
        }),
      }),
    })

    const pendingOrder = await screen.findByTestId(
      'pending-payment-order-failed-reconciliation-order',
    )
    expect(within(pendingOrder).getByText('Processing Ideas')).toBeVisible()

    paymentListener?.({ public_token: 'failed-reconciliation-order', status: 'failed' })

    expect(await within(pendingOrder).findByText('Payment incomplete')).toBeVisible()
    await user.click(screen.getByRole('button', { name: 'Pay now' }))
    await waitFor(() => expect(createCheckout).toHaveBeenCalledOnce())
  })

  it('retains a failed pending Checkout in the visible cache after its SSE event', async () => {
    let paymentListener: Parameters<PaymentsResource['subscribe']>[1] | undefined
    const listPendingOrders = vi.fn().mockResolvedValue({
      items: [
        {
          public_token: 'failed-order',
          idea_quantity: 1000,
          status: 'checkout_open',
          checkout_url: 'https://checkout.stripe.com/c/pay/failed-order',
          expires_at: '2026-08-01T01:00:00Z',
          updated_at: '2026-08-01T00:00:00Z',
        },
      ],
    })
    const listRecharges = vi.fn().mockResolvedValue({ items: [] })
    const subscribe = vi.fn(
      (_workspaceId: string, listener: Parameters<PaymentsResource['subscribe']>[1]) => {
        paymentListener = listener
        return () => undefined
      },
    )
    renderPane({ resource: makeResource({ listPendingOrders, listRecharges, subscribe }) })

    const pendingOrder = await screen.findByTestId('pending-payment-order-failed-order')
    expect(within(pendingOrder).getByRole('button', { name: 'Continue payment' })).toBeVisible()

    paymentListener?.({ public_token: 'failed-order', status: 'failed' })

    expect(await within(pendingOrder).findByText('Payment incomplete')).toBeVisible()
    expect(
      within(pendingOrder).queryByRole('button', { name: 'Continue payment' }),
    ).not.toBeInTheDocument()
    expect(listPendingOrders).toHaveBeenCalledOnce()
    await waitFor(() => expect(listRecharges).toHaveBeenCalledTimes(2))
  })

  it('keeps the form retryable when Checkout creation fails', async () => {
    const createCheckout = vi.fn().mockRejectedValue(new Error('暂时无法创建支付页面'))
    const navigate = vi.fn()
    const { user } = renderPane({ resource: makeResource({ createCheckout }), navigate })

    await user.click(await screen.findByRole('button', { name: 'Pay now' }))

    expect(await screen.findByRole('alert')).toHaveTextContent('Network error. Please try again.')
    // Dual: PurchaseCard inline alert stays as position anchor, toast mirrors it.
    await waitFor(() =>
      expect(toast.error).toHaveBeenCalledWith('Could not open the payment page.'),
    )
    expect(screen.getByRole('button', { name: 'Pay now' })).toBeEnabled()
    expect(navigate).not.toHaveBeenCalled()
  })

  it('reuses the idempotency key for a retry and rotates it when the purchase intent changes', async () => {
    const createCheckout = vi.fn().mockRejectedValue(new Error('暂时无法创建支付页面'))
    const { user } = renderPane({ resource: makeResource({ createCheckout }) })

    const checkout = await screen.findByRole('button', { name: 'Pay now' })

    await user.click(checkout)
    await waitFor(() => expect(createCheckout).toHaveBeenCalledTimes(1))
    await user.click(checkout)
    await waitFor(() => expect(createCheckout).toHaveBeenCalledTimes(2))

    const firstKey = createCheckout.mock.calls[0]![1].idempotency_key
    expect(createCheckout.mock.calls[1]![1].idempotency_key).toBe(firstKey)

    await user.type(screen.getByRole('textbox', { name: 'Custom amount' }), '501')
    await user.click(checkout)
    await waitFor(() => expect(createCheckout).toHaveBeenCalledTimes(3))

    expect(createCheckout.mock.calls[2]![1].idempotency_key).not.toBe(firstKey)
  })

  it('rotates the idempotency key after Checkout creation succeeds', async () => {
    const createCheckout = vi.fn().mockResolvedValue({
      checkout_url: 'https://checkout.stripe.com/c/pay/cs_test_success',
      public_token: '0198cd29-f457-7e4b-b95e-6cfde41b7222',
      expires_at: '2026-08-01T01:00:00Z',
      reused: false,
    })
    const navigate = vi.fn()
    const { user } = renderPane({ resource: makeResource({ createCheckout }), navigate })

    const checkout = await screen.findByRole('button', { name: 'Pay now' })

    await user.click(checkout)
    await waitFor(() => expect(createCheckout).toHaveBeenCalledTimes(1))
    await user.click(checkout)
    await waitFor(() => expect(createCheckout).toHaveBeenCalledTimes(2))

    expect(createCheckout.mock.calls[1]![1].idempotency_key).not.toBe(
      createCheckout.mock.calls[0]![1].idempotency_key,
    )
    expect(navigate).toHaveBeenCalledTimes(2)
  })

  it.each([
    ['checkout_request_rejected', '无法创建支付页面，请重试'],
    ['checkout_session_expired', '支付页面已过期，请重新发起充值'],
    ['payment_already_completed', '该充值已完成，请勿重复支付'],
  ])('rotates the idempotency key after the server returns %s', async (code, message) => {
    const rejectedError = Object.assign(new Error(message), { code })
    const createCheckout = vi.fn().mockRejectedValue(rejectedError)
    const { user } = renderPane({ resource: makeResource({ createCheckout }) })

    const checkout = await screen.findByRole('button', { name: 'Pay now' })

    await user.click(checkout)
    await waitFor(() => expect(createCheckout).toHaveBeenCalledTimes(1))
    await user.click(checkout)
    await waitFor(() => expect(createCheckout).toHaveBeenCalledTimes(2))

    expect(createCheckout.mock.calls[1]![1].idempotency_key).not.toBe(
      createCheckout.mock.calls[0]![1].idempotency_key,
    )
  })

  it('keeps the narrow-screen history surface keyboard scrollable', async () => {
    renderPane()

    await screen.findByLabelText('Quick top-up packages')
    expect(screen.getByRole('region', { name: 'Top-up history table' })).toHaveAttribute(
      'tabindex',
      '0',
    )
  })
})
