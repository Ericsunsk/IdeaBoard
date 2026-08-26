import PrimeVue from 'primevue/config'
import { ref } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { userPrimeVueTestOptions } from '../../../test/primeVue'
import { renderWithUser, screen, within } from '../../../test/render'
import type { UserBillingRecord } from '../userBillingRecordsResource'
import UserBillingRecordsPane from './UserBillingRecordsPane.vue'

const useUserBillingRecordsMock = vi.hoisted(() => vi.fn())

vi.mock('../useUserBillingRecords', () => ({
  useUserBillingRecords: useUserBillingRecordsMock,
}))

function makeRecord(overrides: Partial<UserBillingRecord> = {}): UserBillingRecord {
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
    records: ref<UserBillingRecord[]>([]),
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
  return renderWithUser(UserBillingRecordsPane, {
    props: { active: true },
    global: { plugins: [[PrimeVue, userPrimeVueTestOptions]] },
  })
}

describe('UserBillingRecordsPane', () => {
  beforeEach(() => useUserBillingRecordsMock.mockReset())

  it('renders the four-column final billing record table with stable amount semantics', () => {
    useUserBillingRecordsMock.mockReturnValue(
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
    expect(screen.getAllByRole('columnheader')[0]).toHaveClass('p-datatable-header-cell')
    expect(screen.getByRole('region', { name: 'Transactions table' })).toHaveClass(
      'p-datatable-table-container',
      'app-scrollbar',
    )
    expect(screen.getAllByText('07/14/2026, 09:08:07')).toHaveLength(3)
    expect(screen.getByText('-20')).toHaveClass('text-ib-text')
    expect(screen.getByText('+50')).toHaveClass('text-ib-success')
    expect(screen.getByText('-5')).toHaveClass('text-ib-danger')
    expect(screen.getByText('1,030')).toBeVisible()
  })

  it('keeps the table headers and stable rows visible during the first request', () => {
    useUserBillingRecordsMock.mockReturnValue(makeBillingState({ isInitialPending: ref(true) }))

    renderPane()

    expect(screen.getByRole('table', { name: 'Transactions table' })).toBeVisible()
    expect(screen.getAllByRole('columnheader')).toHaveLength(4)
    expect(screen.getByRole('status', { name: 'Loading billing records…' })).toBeInTheDocument()
    const skeletonRows = screen.getAllByTestId('billing-skeleton-row')
    expect(skeletonRows).toHaveLength(5)
    expect(skeletonRows[0]).toHaveClass('p-skeleton')
  })

  it('renders the confirmed empty state inside the shared table frame', () => {
    useUserBillingRecordsMock.mockReturnValue(makeBillingState())

    renderPane()

    expect(screen.getByText('No billing records yet')).toBeVisible()
    expect(screen.getByText('Your Ideas activity will appear here.')).toBeVisible()
  })

  it('maps ledger kinds to English descriptions and keeps ai_usage pricing display names', () => {
    useUserBillingRecordsMock.mockReturnValue(
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

  it('recovers from an initial failure without removing the table header', async () => {
    const retryInitial = vi.fn().mockResolvedValue(undefined)
    useUserBillingRecordsMock.mockReturnValue(
      makeBillingState({ initialError: ref(new Error('账单服务暂不可用')), retryInitial }),
    )

    const { user } = renderPane()

    expect(screen.getByRole('table', { name: 'Transactions table' })).toBeVisible()
    const alert = screen.getByRole('alert')
    expect(alert).toHaveTextContent('Could not load billing records.')
    expect(alert).toHaveTextContent('Network error. Please try again.')
    await user.click(within(alert).getByRole('button', { name: 'Reload' }))
    expect(retryInitial).toHaveBeenCalledOnce()
  })

  it('keeps loaded records when loading more fails and retries the cursor page', async () => {
    const loadMore = vi.fn().mockResolvedValue(undefined)
    useUserBillingRecordsMock.mockReturnValue(
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
    useUserBillingRecordsMock.mockReturnValue(
      makeBillingState({ records: ref([makeRecord()]), hasNextPage: ref(true), loadMore }),
    )

    const { user } = renderPane()
    const loadMoreButton = screen.getByRole('button', { name: 'Load more' })
    expect(loadMoreButton).toHaveClass('p-button-outlined')
    await user.click(loadMoreButton)

    expect(loadMore).toHaveBeenCalledOnce()
  })
})
