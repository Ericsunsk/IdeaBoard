import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { renderWithUser, screen, waitFor, within } from '../../../test/render'
import type { PaymentReturnContext } from '../paymentReturnRoute'
import {
  userPaymentsContextKey,
  type UserPaymentPurchaseOptions,
  type UserPaymentsContext,
  type UserPaymentsResource,
} from '../userPaymentsResource'
import UserRechargePane from './UserRechargePane.vue'

const purchaseOptions = {
  purchases_available: true,
  quick_idea_quantities: [1000, 2000, 3000, 5000, 10_000],
  min_idea_quantity: 500,
  max_idea_quantity: 500_000,
  ideas_per_usd: 100,
  terms: { version: '2026-08-01', url: '/terms' },
  refund_policy: { version: '2026-08-01', url: '/refund-policy' },
} as const

function makeResource(overrides: Partial<UserPaymentsResource> = {}): UserPaymentsResource {
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
    listRecharges: vi.fn().mockResolvedValue({ items: [] }),
    subscribe: vi.fn(() => () => undefined),
    ...overrides,
  }
}

function renderPane(
  options: {
    resource?: UserPaymentsResource
    paymentReturn?: PaymentReturnContext | null
    navigate?: UserPaymentsContext['navigateToCheckout']
  } = {},
) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  const context: UserPaymentsContext = {
    resource: options.resource ?? makeResource(),
    navigateToCheckout: options.navigate ?? vi.fn(),
  }
  return {
    context,
    ...renderWithUser(UserRechargePane, {
      props: { active: true, paymentReturn: options.paymentReturn ?? null },
      global: {
        plugins: [[VueQueryPlugin, { queryClient }]],
        provide: { [userPaymentsContextKey as symbol]: context },
      },
    }),
  }
}

describe('UserRechargePane', () => {
  beforeEach(() => vi.clearAllMocks())

  it('renders the canonical quick packages with exact two-decimal USD prices', async () => {
    renderPane()

    expect(await screen.findByRole('heading', { name: '充值积分' })).toBeVisible()
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
      getPurchaseOptions: vi.fn(() => new Promise<UserPaymentPurchaseOptions>(() => undefined)),
    })
    renderPane({ resource })

    expect(screen.getByRole('status', { name: '正在加载充值选项' })).toBeVisible()
  })

  it('requires explicit legal agreement and redirects once with the integer quantity', async () => {
    const createCheckout = vi.fn().mockResolvedValue({
      checkout_url: 'https://checkout.stripe.com/c/pay/cs_test_123',
      public_token: '0198cd29-f457-7e4b-b95e-6cfde41b7222',
      expires_at: '2026-08-01T01:00:00Z',
      reused: false,
    })
    const navigate = vi.fn()
    const { user } = renderPane({ resource: makeResource({ createCheckout }), navigate })

    const checkout = await screen.findByRole('button', { name: '前往 Stripe 支付' })
    expect(checkout).toBeDisabled()
    await user.click(screen.getByRole('checkbox', { name: /服务条款.*退款政策/ }))
    expect(checkout).toBeEnabled()
    await user.click(checkout)

    await waitFor(() => {
      expect(createCheckout).toHaveBeenCalledWith({
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
    const custom = await screen.findByRole('textbox', { name: '自定义数量' })

    await user.type(custom, '501')
    expect(screen.getByText('$5.01 USD')).toBeVisible()
    await user.click(screen.getByRole('checkbox', { name: /服务条款.*退款政策/ }))
    await user.click(screen.getByRole('button', { name: '前往 Stripe 支付' }))

    expect(createCheckout).toHaveBeenCalledWith({
      idea_quantity: 501,
      idempotency_key: expect.any(String),
      refund_policy_version: '2026-08-01',
      terms_accepted: true,
      terms_version: '2026-08-01',
    })
  })

  it('shows precise custom validation and legal document links', async () => {
    const { user } = renderPane()
    const custom = await screen.findByRole('textbox', { name: '自定义数量' })

    await user.type(custom, '500.5')

    expect(screen.getByRole('alert')).toHaveTextContent('请输入 500–500,000 之间的整数')
    expect(custom).toHaveAttribute('aria-invalid', 'true')
    expect(screen.getByRole('link', { name: '服务条款' })).toHaveAttribute('href', '/terms')
    expect(screen.getByRole('link', { name: '退款政策' })).toHaveAttribute('href', '/refund-policy')
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
        ],
      }),
    })
    renderPane({ resource })

    const table = await screen.findByRole('table', { name: '充值记录表格' })
    expect(
      within(table)
        .getAllByRole('columnheader')
        .map((item) => item.textContent),
    ).toEqual(['时间', 'Ideas', '实付金额', '状态', '收据'])
    expect(await within(table).findByText('2,000')).toBeVisible()
    expect(within(table).getByText('HK$156.50 HKD')).toBeVisible()
    expect(within(table).getByText('已到账')).toBeVisible()
    expect(within(table).getByRole('link', { name: '查看收据' })).toHaveAttribute(
      'href',
      'https://pay.stripe.com/receipts/demo',
    )
    expect(screen.queryByText(/customer|session|email/i)).not.toBeInTheDocument()
  })

  it('shows the backend-owned processing result after a Checkout return', async () => {
    renderPane({
      paymentReturn: {
        publicToken: '0198cd29-f457-7e4b-b95e-6cfde41b7222',
        source: 'success',
      },
    })

    const status = (await screen.findByText('正在确认支付结果')).closest('[role="status"]')!
    expect(status).toHaveTextContent('支付结果以 Stripe 确认状态为准')
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

    const status = (await screen.findByText('3,000 Ideas 已到账')).closest('[role="status"]')!
    expect(status).toHaveTextContent('3,000 Ideas 已到账')
    expect(status).toHaveTextContent('余额与充值记录已更新')
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

    const table = await screen.findByRole('table', { name: '充值记录表格' })
    expect(await within(table).findByText('$10.00 USD')).toBeVisible()
    await user.click(screen.getByRole('button', { name: '加载更多' }))
    expect(await within(table).findByText('$20.00 USD')).toBeVisible()
    expect(listRecharges).toHaveBeenNthCalledWith(1, { limit: 20 }, expect.any(AbortSignal))
    expect(listRecharges).toHaveBeenNthCalledWith(
      2,
      { cursor: 'cursor_opaque_page_2', limit: 20 },
      expect.any(AbortSignal),
    )
  })

  it('uses the user payment event only to invalidate visible account queries', async () => {
    let paymentListener: Parameters<UserPaymentsResource['subscribe']>[0] | undefined
    const unsubscribe = vi.fn()
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
    const subscribe = vi.fn((listener: Parameters<UserPaymentsResource['subscribe']>[0]) => {
      paymentListener = listener
      return unsubscribe
    })
    const { unmount } = renderPane({ resource: makeResource({ listRecharges, subscribe }) })
    expect(await screen.findByText('暂无充值记录')).toBeVisible()
    expect(listRecharges).toHaveBeenCalledOnce()

    paymentListener?.({ status: 'credited' })

    await waitFor(() => expect(listRecharges).toHaveBeenCalledTimes(2))
    const historyTable = await screen.findByRole('table', { name: '充值记录表格' })
    expect(await within(historyTable).findByText('$10.00 USD')).toBeVisible()
    unmount()
    expect(unsubscribe).toHaveBeenCalledOnce()
  })

  it('keeps the form retryable when Checkout creation fails', async () => {
    const createCheckout = vi.fn().mockRejectedValue(new Error('暂时无法创建支付页面'))
    const navigate = vi.fn()
    const { user } = renderPane({ resource: makeResource({ createCheckout }), navigate })

    await user.click(await screen.findByRole('checkbox', { name: /服务条款.*退款政策/ }))
    await user.click(screen.getByRole('button', { name: '前往 Stripe 支付' }))

    expect(await screen.findByRole('alert')).toHaveTextContent('暂时无法创建支付页面')
    expect(screen.getByRole('button', { name: '前往 Stripe 支付' })).toBeEnabled()
    expect(navigate).not.toHaveBeenCalled()
  })

  it('reuses the idempotency key for a retry and rotates it when the purchase intent changes', async () => {
    const createCheckout = vi.fn().mockRejectedValue(new Error('暂时无法创建支付页面'))
    const { user } = renderPane({ resource: makeResource({ createCheckout }) })

    await user.click(await screen.findByRole('checkbox', { name: /服务条款.*退款政策/ }))
    const checkout = screen.getByRole('button', { name: '前往 Stripe 支付' })

    await user.click(checkout)
    await waitFor(() => expect(createCheckout).toHaveBeenCalledTimes(1))
    await user.click(checkout)
    await waitFor(() => expect(createCheckout).toHaveBeenCalledTimes(2))

    const firstKey = createCheckout.mock.calls[0]![0].idempotency_key
    expect(createCheckout.mock.calls[1]![0].idempotency_key).toBe(firstKey)

    await user.type(screen.getByRole('textbox', { name: '自定义数量' }), '501')
    await user.click(checkout)
    await waitFor(() => expect(createCheckout).toHaveBeenCalledTimes(3))

    expect(createCheckout.mock.calls[2]![0].idempotency_key).not.toBe(firstKey)
  })

  it('keeps the narrow-screen history surface keyboard scrollable', async () => {
    renderPane()

    await screen.findByLabelText('快捷充值套餐')
    expect(screen.getByRole('region', { name: '充值记录表格' })).toHaveAttribute('tabindex', '0')
  })
})
