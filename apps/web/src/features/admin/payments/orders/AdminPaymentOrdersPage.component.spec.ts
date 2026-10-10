import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { renderWithUser, screen, waitFor } from '../../../../test/render'
import { mockAppToast, type AppToastMocks } from '../../../../test/toast'
import AdminPaymentOrdersPage from './AdminPaymentOrdersPage.vue'

const resource = vi.hoisted(() => ({
  search: vi.fn(),
  get: vi.fn(),
  refund: vi.fn(),
}))
const confirm = vi.hoisted(() => ({
  confirm: vi.fn().mockResolvedValue(true),
  readReason: vi.fn().mockReturnValue('refund review'),
}))

let toast: AppToastMocks

vi.mock('./paymentOrderResource', () => ({
  createAdminPaymentOrderResource: () => resource,
}))

vi.mock('../../shared/composables/useAdminConfirm', () => ({
  useAdminConfirm: () => confirm,
}))

vi.mock('../../../../core/composables/useAppToast')

describe('AdminPaymentOrdersPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    toast = mockAppToast()
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
    expect(screen.getByText('Sunshi')).toBeVisible()
    expect(screen.queryByText('00000000-0000-4000-8000-000000000042')).not.toBeInTheDocument()
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

  it('clears the status filter back to all from inside the select', async () => {
    const { user } = renderPage()
    await screen.findByText(orderNumber)

    await user.click(screen.getByRole('button', { name: '筛选支付订单' }))
    await user.click(screen.getByRole('combobox'))
    await user.click(await screen.findByRole('option', { name: '已支付待到账' }))
    await waitFor(() =>
      expect(resource.search).toHaveBeenLastCalledWith(
        expect.objectContaining({ status: 'paid_pending_credit' }),
      ),
    )

    await user.click(screen.getByRole('combobox'))
    await user.click(await screen.findByRole('option', { name: '全部状态' }))
    await waitFor(() =>
      expect(resource.search).toHaveBeenLastCalledWith(
        expect.objectContaining({ status: undefined, cursor: undefined }),
      ),
    )
  })

  it('opens the safe detail dialog with receipt, reconciliation, refund, and Dispute states', async () => {
    resource.get.mockResolvedValueOnce({
      ...makeDetail(),
      disputes: [
        {
          id: 'dp_i…7890',
          status: 'lost',
          providerStatus: 'lost',
          amountMinor: 3_750,
          currency: 'usd',
          ratioBasisPoints: 2_941,
          targetIdeaQuantity: 294,
          debitedIdeaQuantity: 250,
          unrecoveredCredits: 44,
          stripeDashboardUrl: 'https://dashboard.stripe.com/test/disputes/dp_idea',
          openedAt: '2026-08-02T02:00:00Z',
          closedAt: '2026-08-03T02:00:00Z',
        },
      ],
    })
    const { user } = renderPage()
    await screen.findByText(orderNumber)

    await user.click(screen.getAllByRole('button', { name: `查看订单 ${orderNumber}` })[0]!)

    expect(await screen.findByRole('dialog', { name: `支付订单 · ${orderNumber}` })).toBeVisible()
    expect(screen.queryByText('00000000-0000-4000-8000-000000000042')).not.toBeInTheDocument()
    expect(
      screen.queryByText('仅显示服务端脱敏 identity，不包含 API Key、Webhook Payload 或账单地址。'),
    ).not.toBeInTheDocument()
    expect(screen.getByText('商品金额')).toBeVisible()
    expect(screen.getByText('税费')).toBeVisible()
    expect(screen.getByText('条款版本')).toBeVisible()
    expect(screen.getByText('退款政策版本')).toBeVisible()
    expect(screen.getByText('过期时间')).toBeVisible()
    expect(screen.getByText('最近更新')).toBeVisible()
    expect(screen.getByText('基准金额')).toBeVisible()
    expect(screen.getByText('acct…7890')).toBeVisible()
    expect(screen.getByText('Stripe 暂时不可用')).toBeVisible()
    expect(screen.getByText('暂无退款记录')).toBeVisible()
    expect(screen.getByText('lost')).toBeVisible()
    expect(screen.getByText(/目标 294 Ideas \/ 已扣回 250 Ideas \/ 未追回 44 Ideas/)).toBeVisible()
    expect(screen.getByText(/开启 .*关闭/)).toBeVisible()
    expect(screen.getByRole('link', { name: 'Stripe Dashboard' })).toHaveAttribute(
      'href',
      'https://dashboard.stripe.com/test/disputes/dp_idea',
    )
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

  it('confirms an eligible partial refund and refreshes the payment read model', async () => {
    const eligibleDetail = {
      ...makeDetail(),
      status: 'credited',
      creditStatus: 'credited',
      paidAt: new Date().toISOString(),
      creditedAt: new Date().toISOString(),
      refundableIdeaQuantity: 500,
    }
    resource.get.mockResolvedValueOnce(eligibleDetail)
    resource.refund.mockResolvedValueOnce({
      ...eligibleDetail,
      refundableIdeaQuantity: 300,
      refunds: [
        {
          id: 'refu…1234',
          status: 'pending',
          ideaQuantity: 200,
          amountMinor: 200,
          currency: 'hkd',
          updatedAt: new Date().toISOString(),
        },
      ],
    })
    const { user } = renderPage()
    await screen.findByText(orderNumber)
    await user.click(screen.getAllByRole('button', { name: `查看订单 ${orderNumber}` })[0]!)

    expect(await screen.findByText('发起退款')).toBeVisible()
    const input = screen.getByLabelText('退款 Idea 数量')
    await user.type(input, '200')
    await user.click(screen.getByRole('button', { name: '确认退款' }))

    expect(confirm.confirm).toHaveBeenCalledWith(expect.objectContaining({ title: '确认退款' }))
    await waitFor(() =>
      expect(resource.refund).toHaveBeenCalledWith(
        orderNumber,
        expect.objectContaining({
          ideaQuantity: 200,
          idempotencyKey: expect.any(String),
          operatorReason: 'refund review',
        }),
      ),
    )
    expect(toast.success).toHaveBeenCalledWith('退款已提交，等待 Stripe 最终处理')
    expect(resource.search).toHaveBeenCalledTimes(2)
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

  it('keeps the table frame mounted while the initial collection request is pending', async () => {
    resource.search.mockReturnValueOnce(new Promise(() => {}))
    renderPage()

    expect(await screen.findByRole('table', { name: '支付订单列表' })).toBeVisible()
  })

  it('keeps the table headers mounted for the empty state', async () => {
    resource.search.mockResolvedValueOnce({ orders: [], total: 0 })
    renderPage()

    expect(await screen.findByText('暂无支付订单')).toBeVisible()
    expect(screen.getByRole('table', { name: '支付订单列表' })).toBeVisible()
    expect(screen.getByRole('columnheader', { name: '公开订单号' })).toBeVisible()
  })
})

const orderNumber = 'IBP-20260803-ABCDEF123456'

function renderPage() {
  // Fresh client per render: the list query caches by key, so sharing one
  // across tests would serve stale pages and hide `resource.search` calls.
  const queryClient = new QueryClient()
  return renderWithUser(AdminPaymentOrdersPage, {
    global: {
      plugins: [[VueQueryPlugin, { queryClient }]],
    },
  })
}

function makeOrder() {
  return {
    publicOrderNumber: orderNumber,
    user: {
      publicId: '00000000-0000-4000-8000-000000000042',
      displayName: 'Sunshi',
    },
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
