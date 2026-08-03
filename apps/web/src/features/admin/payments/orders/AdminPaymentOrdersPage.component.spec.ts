import PrimeVue from 'primevue/config'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { primeVueTestOptions } from '../../../../test/primeVue'
import { renderWithUser, screen, waitFor } from '../../../../test/render'
import AdminPaymentOrdersPage from './AdminPaymentOrdersPage.vue'

const resource = vi.hoisted(() => ({
  search: vi.fn(),
  get: vi.fn(),
}))

vi.mock('./paymentOrderResource', () => ({
  createAdminPaymentOrderResource: () => resource,
}))

describe('AdminPaymentOrdersPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    resource.search.mockResolvedValue({
      orders: [makeOrder()],
      total: 27,
      nextCursor: 'cursor-2',
    })
    resource.get.mockResolvedValue(makeDetail())
  })

  it('renders the read model, searches both identities, filters anomalies, and pages by cursor', async () => {
    const { user } = renderPage()

    expect(await screen.findByText(orderNumber)).toBeVisible()
    expect(screen.getByText('筛选结果 27 笔')).toBeVisible()
    expect(screen.getByText('已支付')).toBeVisible()
    expect(screen.getByText('待到账')).toBeVisible()
    expect(screen.getByText('24h 未收敛')).toBeVisible()
    expect(screen.getByRole('region', { name: '支付订单列表横向滚动区域' })).toHaveAttribute(
      'tabindex',
      '0',
    )

    await user.type(screen.getByRole('textbox', { name: '搜索公开订单号' }), orderNumber)
    await user.type(screen.getByRole('textbox', { name: '搜索支付用户' }), '42')
    await user.keyboard('{Enter}')
    await waitFor(() =>
      expect(resource.search).toHaveBeenLastCalledWith(
        expect.objectContaining({ orderNumber, user: '42', cursor: undefined }),
      ),
    )

    await user.click(screen.getByRole('button', { name: '筛选支付订单' }))
    await user.click(screen.getByRole('combobox'))
    await user.click(await screen.findByRole('option', { name: '已支付待到账' }))
    await user.click(screen.getByRole('switch', { name: '仅看 24 小时异常' }))
    await waitFor(() =>
      expect(resource.search).toHaveBeenLastCalledWith(
        expect.objectContaining({
          status: 'paid_pending_credit',
          anomalyOnly: true,
          cursor: undefined,
        }),
      ),
    )

    await user.click(screen.getByRole('button', { name: '下一页' }))
    await waitFor(() =>
      expect(resource.search).toHaveBeenLastCalledWith(
        expect.objectContaining({ cursor: 'cursor-2' }),
      ),
    )
  })

  it('opens the safe detail dialog with receipt, reconciliation, refund, and Dispute states', async () => {
    const { user } = renderPage()
    await screen.findByText(orderNumber)

    await user.click(screen.getAllByRole('button', { name: `查看订单 ${orderNumber}` })[0]!)

    expect(await screen.findByRole('dialog', { name: `支付订单 · ${orderNumber}` })).toBeVisible()
    expect(screen.getByText('acct…7890')).toBeVisible()
    expect(screen.getByText('Stripe 暂时不可用')).toBeVisible()
    expect(screen.getByText('暂无退款记录')).toBeVisible()
    expect(screen.getByText('暂无 Dispute')).toBeVisible()
    expect(screen.getByRole('link', { name: '打开 Stripe 收据' })).toHaveAttribute(
      'rel',
      'noopener noreferrer',
    )
    expect(resource.get).toHaveBeenCalledWith(orderNumber, expect.any(AbortSignal))
  })

  it('recovers a failed detail request inside the focused dialog', async () => {
    resource.get.mockRejectedValueOnce(new Error('network unavailable'))
    const { user } = renderPage()
    await screen.findByText(orderNumber)

    await user.click(screen.getAllByRole('button', { name: `查看订单 ${orderNumber}` })[0]!)
    expect(await screen.findByText('支付订单详情加载失败，请重试。')).toBeVisible()

    await user.click(screen.getByRole('button', { name: '重试' }))
    expect(await screen.findByText('acct…7890')).toBeVisible()
    expect(resource.get).toHaveBeenCalledTimes(2)
  })

  it('distinguishes initial and background refresh errors, then recovers without dropping data', async () => {
    resource.search.mockRejectedValueOnce(new Error('network unavailable'))
    const { user } = renderPage()

    expect(await screen.findByText('支付订单加载失败，请重试。')).toBeVisible()
    await user.click(screen.getByRole('button', { name: '重试' }))
    expect(await screen.findByText(orderNumber)).toBeVisible()

    resource.search.mockRejectedValueOnce(new Error('network unavailable'))
    await user.click(screen.getByRole('button', { name: '刷新支付订单' }))
    expect(await screen.findByText('刷新支付订单失败，正在显示上次成功的数据。')).toBeVisible()
    expect(screen.getByText(orderNumber)).toBeVisible()

    await user.click(screen.getByRole('button', { name: '重试' }))
    await waitFor(() =>
      expect(
        screen.queryByText('刷新支付订单失败，正在显示上次成功的数据。'),
      ).not.toBeInTheDocument(),
    )
    expect(screen.getByText(orderNumber)).toBeVisible()
  })

  it('renders the compact filtered empty state without a table shell', async () => {
    resource.search.mockResolvedValueOnce({ orders: [], total: 0 })
    renderPage()

    expect(await screen.findByText('暂无支付订单')).toBeVisible()
    expect(screen.queryByRole('table', { name: '支付订单列表' })).not.toBeInTheDocument()
  })
})

const orderNumber = 'IBP-20260803-ABCDEF123456'

function renderPage() {
  return renderWithUser(AdminPaymentOrdersPage, {
    global: { plugins: [[PrimeVue, primeVueTestOptions]] },
  })
}

function makeOrder() {
  return {
    publicOrderNumber: orderNumber,
    user: { id: 42, username: 'sunshi' },
    ideaQuantity: 12_000,
    baseAmountMinor: 12_000,
    baseCurrency: 'usd',
    paidAmountMinor: 12_750,
    paidCurrency: 'hkd',
    status: 'paid_pending_credit',
    stripeStatus: 'paid',
    creditStatus: 'pending',
    createdAt: '2026-08-01T01:00:00Z',
    paidAt: '2026-08-01T01:03:00Z',
    lastCheckedAt: '2026-08-03T01:00:00Z',
    anomalousAt: '2026-08-02T01:00:00Z',
  }
}

function makeDetail() {
  return {
    ...makeOrder(),
    paidSubtotalMinor: 12_000,
    paidTaxMinor: 750,
    paidTotalMinor: 12_750,
    stripe: {
      accountId: 'acct…7890',
      customerId: 'cus_…7890',
      checkoutSessionId: 'cs_t…7890',
      paymentIntentId: 'pi_1…7890',
      chargeId: 'ch_1…7890',
    },
    termsVersion: '2026-08-01',
    refundPolicyVersion: '2026-08-01',
    receiptUrl: 'https://pay.stripe.com/receipts/example',
    expiresAt: '2026-08-01T01:30:00Z',
    updatedAt: '2026-08-03T01:00:00Z',
    creditedAt: undefined,
    reconciliation: {
      anomalous: true,
      anomalousAt: '2026-08-02T01:00:00Z',
      lastCheckedAt: '2026-08-03T01:00:00Z',
      nextAttemptAt: '2026-08-03T01:05:00Z',
      lastErrorCode: 'stripe_unavailable',
      lastErrorSummary: 'Stripe 暂时不可用',
    },
    refunds: [],
    disputes: [],
  }
}
