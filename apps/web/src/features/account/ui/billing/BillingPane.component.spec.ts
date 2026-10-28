import { ref } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { renderWithUser, screen, within } from '../../../../test/render'
import { mockAppToast, expectToastFeedback, type AppToastMocks } from '../../../../test/toast'
import type { BillingRecord } from '../../session/billing/resource'
import BillingPane from './BillingPane.vue'

const useBillingRecordsMock = vi.hoisted(() => vi.fn())

vi.mock('../../session/billing/useBillingRecords', () => ({
  useBillingRecords: useBillingRecordsMock,
}))

vi.mock('../../../../core/composables/useAppToast')

function makeRecord(overrides: Partial<BillingRecord> = {}): BillingRecord {
  return {
    record_id: 'record-1',
    occurred_at: '2026-07-14T09:08:07',
    kind: 'ai_usage',
    title: '局部重绘',
    credit_delta: -20,
    available_credits_after: 980,
    ...overrides,
  }
}

function makeBillingState(overrides: Record<string, unknown> = {}) {
  return {
    billingAccountId: ref<string | undefined>(undefined),
    isShared: ref(false),
    consumptionScope: ref('account'),
    records: ref<BillingRecord[]>([]),
    isInitialPending: ref(false),
    initialError: ref<unknown>(null),
    loadMoreError: ref<unknown>(null),
    hasNextPage: ref(false),
    isFetchingNextPage: ref(false),
    retryInitial: vi.fn().mockResolvedValue(undefined),
    loadMore: vi.fn().mockResolvedValue(undefined),
    ...overrides,
  }
}

function renderPane() {
  return renderWithUser(BillingPane, {
    props: { active: true },
  })
}

describe('BillingPane', () => {
  let toast: AppToastMocks

  beforeEach(() => {
    useBillingRecordsMock.mockReset()
    toast = mockAppToast()
  })

  it('renders the four-column final billing record table with stable amount semantics', () => {
    useBillingRecordsMock.mockReturnValue(
      makeBillingState({
        records: ref([
          makeRecord(),
          makeRecord({
            record_id: 'record-2',
            kind: 'reward',
            title: '兑换奖励',
            credit_delta: 50,
            available_credits_after: 1030,
          }),
          makeRecord({
            record_id: 'record-3',
            kind: 'system_adjustment',
            title: '系统调整',
            credit_delta: -5,
            available_credits_after: 1025,
          }),
        ]),
      }),
    )

    renderPane()

    expect(screen.getByRole('heading', { name: 'Transactions' })).toBeVisible()
    expect(screen.getAllByRole('columnheader').map((header) => header.textContent)).toEqual([
      'Time',
      'Description',
      'Ideas change',
      'Balance after',
    ])
    expect(screen.getAllByRole('columnheader')[0]).toHaveClass('text-foreground')
    expect(screen.getByRole('region', { name: 'Transactions table' })).toHaveClass(
      'app-scrollbar',
      'overflow-x-auto',
    )
    expect(screen.getAllByText('07/14/2026, 09:08:07')).toHaveLength(3)
    expect(screen.getByText('-20')).toHaveClass('text-foreground')
    expect(screen.getByText('+50')).toHaveClass('text-success')
    expect(screen.getByText('-5')).toHaveClass('text-destructive')
    expect(screen.getByText('1,030')).toBeVisible()
  })

  it('keeps the table headers and stable rows visible during the first request', () => {
    useBillingRecordsMock.mockReturnValue(makeBillingState({ isInitialPending: ref(true) }))

    renderPane()

    expect(screen.getByRole('table', { name: 'Transactions table' })).toBeVisible()
    expect(screen.getAllByRole('columnheader')).toHaveLength(4)
    expect(screen.getByRole('status', { name: 'Loading billing records…' })).toBeInTheDocument()
    const skeletonRows = screen.getAllByTestId('billing-skeleton-row')
    expect(skeletonRows).toHaveLength(5)
    expect(skeletonRows[0]).toHaveAttribute('data-ui', 'skeleton')
  })

  it('renders the confirmed empty state inside the shared table frame', () => {
    useBillingRecordsMock.mockReturnValue(makeBillingState())

    renderPane()

    expect(screen.getByText('No billing records yet')).toBeVisible()
    expect(screen.getByText('Your Ideas activity will appear here.')).toBeVisible()
  })

  it('maps ledger kinds to English descriptions and keeps ai_usage pricing display names', () => {
    useBillingRecordsMock.mockReturnValue(
      makeBillingState({
        records: ref([
          makeRecord({ record_id: 'record-1', kind: 'usage', title: 'Idea 消费' }),
          makeRecord({ record_id: 'record-2', kind: 'top_up', title: 'Idea 充值' }),
          makeRecord({ record_id: 'record-3', kind: 'system_grant', title: '系统赠送' }),
          makeRecord({ record_id: 'record-4', kind: 'ai_usage', title: 'Inpaint' }),
        ]),
      }),
    )

    renderPane()

    expect(screen.getByText('Usage')).toBeVisible()
    expect(screen.getByText('Top up')).toBeVisible()
    expect(screen.getByText('Ideas grant')).toBeVisible()
    expect(screen.getByText('Inpaint')).toBeVisible()
    expect(screen.queryByText('Idea 消费')).not.toBeInTheDocument()
    expect(screen.queryByText('Idea 充值')).not.toBeInTheDocument()
  })

  it('toasts an initial failure with a neutral retry entry without removing the table header', async () => {
    const retryInitial = vi.fn().mockResolvedValue(undefined)
    useBillingRecordsMock.mockReturnValue(
      makeBillingState({ initialError: ref(new Error('账单服务暂不可用')), retryInitial }),
    )

    const { user } = renderPane()

    expect(screen.getByRole('table', { name: 'Transactions table' })).toBeVisible()
    // 初载失败只走 toast；中性空态 + 重新加载按钮即恢复入口。
    await expectToastFeedback(toast, 'Network error. Please try again.')
    expect(screen.getByText('No billing records yet')).toBeVisible()
    await user.click(screen.getByRole('button', { name: 'Reload' }))
    expect(retryInitial).toHaveBeenCalledOnce()
  })

  it('keeps loaded records when loading more fails and retries the cursor page', async () => {
    const loadMore = vi.fn().mockResolvedValue(undefined)
    useBillingRecordsMock.mockReturnValue(
      makeBillingState({
        records: ref([makeRecord()]),
        hasNextPage: ref(true),
        loadMoreError: ref(new Error('网络错误')),
        loadMore,
      }),
    )

    const { user } = renderPane()

    expect(screen.getByText('局部重绘')).toBeVisible()
    const alert = screen.getByRole('alert')
    expect(alert).toHaveTextContent('Network error. Please try again.')
    await user.click(within(alert).getByRole('button', { name: 'Retry load' }))
    expect(loadMore).toHaveBeenCalledOnce()
  })

  it('delegates cursor pagination from the single load-more command', async () => {
    const loadMore = vi.fn().mockResolvedValue(undefined)
    useBillingRecordsMock.mockReturnValue(
      makeBillingState({ records: ref([makeRecord()]), hasNextPage: ref(true), loadMore }),
    )

    const { user } = renderPane()
    const loadMoreButton = screen.getByRole('button', { name: 'Load more' })
    expect(loadMoreButton).toHaveClass('border-input')
    await user.click(loadMoreButton)

    expect(loadMore).toHaveBeenCalledOnce()
  })

  it('names the shared balance account behind the records', () => {
    useBillingRecordsMock.mockReturnValue(
      makeBillingState({
        billingAccountId: ref('ba_1'),
        isShared: ref(true),
        records: ref([makeRecord()]),
      }),
    )

    renderPane()

    expect(screen.getByRole('status')).toHaveTextContent('ba_1')
  })

  it('renders the restricted state for billing_account_forbidden instead of a retryable load error', async () => {
    const retryInitial = vi.fn().mockResolvedValue(undefined)
    const forbidden = new Error('forbidden') as Error & { code: string }
    forbidden.code = 'billing_account_forbidden'
    useBillingRecordsMock.mockReturnValue(
      makeBillingState({ initialError: ref(forbidden), retryInitial }),
    )

    const { user } = renderPane()

    expect(screen.getByText('Access restricted')).toBeVisible()
    expect(
      screen.getByText(
        'Your current role cannot view these records. Contact a workspace or billing admin.',
      ),
    ).toBeVisible()
    expect(toast.error).not.toHaveBeenCalled()
    await user.click(screen.getByRole('button', { name: 'Reload' }))
    expect(retryInitial).toHaveBeenCalledOnce()
  })
})

describe('scoped consumption records', () => {
  it('renders member consumption without exposing an account balance column', () => {
    const { available_credits_after: _balance, ...record } = makeRecord()
    useBillingRecordsMock.mockReturnValue(
      makeBillingState({
        consumptionScope: ref('member'),
        records: ref([record]),
      }),
    )
    renderPane()
    expect(screen.getByText('局部重绘')).toBeVisible()
    expect(screen.queryByRole('columnheader', { name: /余额/ })).not.toBeInTheDocument()
    expect(screen.queryByText('980')).not.toBeInTheDocument()
  })
})
