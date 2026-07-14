import PrimeVue from 'primevue/config'
import { ref } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { primeVueTestOptions } from '../../../test/primeVue'
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
    global: { plugins: [[PrimeVue, primeVueTestOptions]] },
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

    expect(screen.getByRole('heading', { name: '账单记录' })).toBeVisible()
    expect(screen.getAllByRole('columnheader').map((header) => header.textContent)).toEqual([
      '交易时间',
      '记录内容',
      'Idea 变动',
      '可用余额',
    ])
    expect(screen.getAllByRole('columnheader')[0]).toHaveClass('p-datatable-header-cell')
    expect(screen.getByRole('region', { name: '账单记录表格' })).toHaveClass(
      'p-datatable-table-container',
      'app-scrollbar',
    )
    expect(screen.getAllByText('2026-07-14 09:08:07')).toHaveLength(3)
    expect(screen.getByText('-20')).toHaveClass('text-ib-text')
    expect(screen.getByText('+50')).toHaveClass('text-ib-success')
    expect(screen.getByText('-5')).toHaveClass('text-ib-danger')
    expect(screen.getByText('1,030')).toBeVisible()
    expect(screen.queryByText(/Ideas/)).not.toBeInTheDocument()
  })

  it('keeps the table headers and stable rows visible during the first request', () => {
    useUserBillingRecordsMock.mockReturnValue(makeBillingState({ isInitialPending: ref(true) }))

    renderPane()

    expect(screen.getByRole('table', { name: '账单记录表格' })).toBeVisible()
    expect(screen.getAllByRole('columnheader')).toHaveLength(4)
    expect(screen.getByRole('status', { name: '正在加载账单记录' })).toBeInTheDocument()
    expect(screen.getAllByTestId('billing-skeleton-row')).toHaveLength(5)
  })

  it('renders the confirmed empty state inside the shared table frame', () => {
    useUserBillingRecordsMock.mockReturnValue(makeBillingState())

    renderPane()

    expect(screen.getByText('暂无账单记录')).toBeVisible()
    expect(screen.getByText('Idea 收支记录会显示在这里')).toBeVisible()
  })

  it('recovers from an initial failure without removing the table header', async () => {
    const retryInitial = vi.fn().mockResolvedValue(undefined)
    useUserBillingRecordsMock.mockReturnValue(
      makeBillingState({ initialError: ref(new Error('账单服务暂不可用')), retryInitial }),
    )

    const { user } = renderPane()

    expect(screen.getByRole('table', { name: '账单记录表格' })).toBeVisible()
    const alert = screen.getByRole('alert')
    expect(alert).toHaveTextContent('账单加载失败')
    expect(alert).toHaveTextContent('账单服务暂不可用')
    await user.click(within(alert).getByRole('button', { name: '重新加载' }))
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
    expect(alert).toHaveTextContent('网络错误')
    await user.click(within(alert).getByRole('button', { name: '重试加载' }))
    expect(loadMore).toHaveBeenCalledOnce()
  })

  it('delegates cursor pagination from the single load-more command', async () => {
    const loadMore = vi.fn().mockResolvedValue(undefined)
    useUserBillingRecordsMock.mockReturnValue(
      makeBillingState({ records: ref([makeRecord()]), hasNextPage: ref(true), loadMore }),
    )

    const { user } = renderPane()
    await user.click(screen.getByRole('button', { name: '加载更多' }))

    expect(loadMore).toHaveBeenCalledOnce()
  })
})
