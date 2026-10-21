import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { screen, waitFor } from '@testing-library/vue'
import { defineComponent, reactive, ref } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { notifyUserCreditsInvalidation } from '../../../../core/query/userCreditsInvalidation'
import { renderWithUser } from '../../../../test/render'
import { billingAccountQueryKeys } from '../billing-account/resource'
import type { BillingRecordPage } from './resource'
import { useBillingRecords } from './useBillingRecords'

const fetchBillingRecordsMock = vi.hoisted(() => vi.fn())
const workspaceId = ref('ws_1')
const fetchAccountMock = vi.hoisted(() => vi.fn())
const identity = reactive({ user: { public_id: 'user-1' } })
vi.mock('../../../auth', () => ({ useUserStore: () => identity }))

vi.mock('./resource', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./resource')>()
  return { ...actual, fetchBillingRecords: fetchBillingRecordsMock }
})

vi.mock('../accountWorkspace', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../accountWorkspace')>()),
  useAccountWorkspace: () => ({
    workspaceId,
    isSelectionSettled: ref(true),
  }),
}))

vi.mock('../billing-account/resource', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../billing-account/resource')>()),
  fetchBillingAccount: fetchAccountMock,
}))

const BillingHost = defineComponent({
  setup() {
    return useBillingRecords(true)
  },
  template: `
    <button type="button" @click="loadMore()">加载更多</button>
    <button type="button" @click="retryInitial()">重试</button>
    <output data-testid="error">{{ initialError?.message || '' }}</output>
    <output data-testid="pending">{{ isInitialPending }}</output>
    <output data-testid="record-titles">{{ records.map((record) => record.title).join(',') }}</output>
    <output data-testid="has-more">{{ hasNextPage }}</output>
  `,
})

describe('useBillingRecords', () => {
  beforeEach(() => {
    fetchBillingRecordsMock.mockReset()
    workspaceId.value = 'ws_1'
    identity.user.public_id = 'user-1'
    fetchAccountMock.mockReset().mockImplementation(async () => buildAccount())
  })

  it('surfaces account resolution failures and retries the prerequisite before loading records', async () => {
    fetchAccountMock
      .mockRejectedValueOnce(new Error('account unavailable'))
      .mockResolvedValueOnce(buildAccount())
    fetchBillingRecordsMock.mockResolvedValue({ records: [], next_cursor: undefined })
    const { user } = renderBillingHost()
    await waitFor(() =>
      expect(screen.getByTestId('error')).toHaveTextContent('account unavailable'),
    )
    expect(screen.getByTestId('pending')).toHaveTextContent('false')
    expect(fetchBillingRecordsMock).not.toHaveBeenCalled()
    await user.click(screen.getByRole('button', { name: '重试' }))
    await waitFor(() => expect(fetchBillingRecordsMock).toHaveBeenCalledOnce())
    expect(fetchAccountMock).toHaveBeenCalledTimes(2)
  })

  it('drops privileged pages and late responses when authorization changes', async () => {
    let resolvePrivileged!: (value: BillingRecordPage) => void
    fetchBillingRecordsMock.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          resolvePrivileged = resolve
        }),
    )
    const { queryClient } = renderBillingHost()
    await waitFor(() => expect(fetchBillingRecordsMock).toHaveBeenCalledOnce())
    fetchBillingRecordsMock.mockResolvedValueOnce(buildPage('自身消费'))
    fetchAccountMock.mockResolvedValueOnce(buildAccount('member'))
    await queryClient.invalidateQueries({ queryKey: billingAccountQueryKeys.all })
    await waitFor(() => expect(screen.getByTestId('record-titles')).toHaveTextContent('自身消费'))
    resolvePrivileged(buildPage('全账户充值'))
    await Promise.resolve()
    expect(screen.getByTestId('record-titles')).not.toHaveTextContent('全账户充值')
  })

  it('does not reuse cached account pages for another identity on the same account', async () => {
    fetchBillingRecordsMock.mockResolvedValueOnce(buildPage('管理员充值'))
    renderBillingHost()
    await waitFor(() => expect(screen.getByTestId('record-titles')).toHaveTextContent('管理员充值'))
    fetchBillingRecordsMock.mockRejectedValueOnce(new Error('member records unavailable'))
    fetchAccountMock.mockResolvedValueOnce(buildAccount('member'))
    identity.user.public_id = 'user-2'
    await waitFor(() =>
      expect(screen.getByTestId('error')).toHaveTextContent('member records unavailable'),
    )
    expect(screen.getByTestId('record-titles')).toHaveTextContent(/^$/)
  })

  it('loads 20-record cursor pages and appends the next page', async () => {
    fetchBillingRecordsMock
      .mockResolvedValueOnce(buildPage('局部重绘', 'cursor-91'))
      .mockResolvedValueOnce(buildPage('活动奖励'))
    const { user } = renderBillingHost()

    await waitFor(() => expect(screen.getByTestId('record-titles')).toHaveTextContent('局部重绘'))
    expect(fetchBillingRecordsMock).toHaveBeenNthCalledWith(
      1,
      'ws_1',
      { page_size: 20 },
      expect.any(AbortSignal),
    )
    expect(screen.getByTestId('has-more')).toHaveTextContent('true')

    await user.click(screen.getByRole('button', { name: '加载更多' }))

    await waitFor(() =>
      expect(screen.getByTestId('record-titles')).toHaveTextContent('局部重绘,活动奖励'),
    )
    expect(fetchBillingRecordsMock).toHaveBeenNthCalledWith(
      2,
      'ws_1',
      { page_size: 20, cursor: 'cursor-91' },
      expect.any(AbortSignal),
    )
  })

  it('refetches the first page after an existing Credits settlement event', async () => {
    fetchBillingRecordsMock.mockResolvedValue(buildPage('草图成图'))
    renderBillingHost()
    await waitFor(() => expect(screen.getByTestId('record-titles')).toHaveTextContent('草图成图'))
    expect(fetchBillingRecordsMock).toHaveBeenCalledOnce()

    notifyUserCreditsInvalidation({
      source: 'task-settlement',
      taskId: 'task-7',
      status: 'completed',
    })

    await waitFor(() => expect(fetchBillingRecordsMock).toHaveBeenCalledTimes(2))
  })
})

function renderBillingHost() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { gcTime: Number.POSITIVE_INFINITY, retry: false } },
  })
  const result = renderWithUser(BillingHost, {
    global: { plugins: [[VueQueryPlugin, { queryClient }]] },
  })
  return { ...result, queryClient }
}

function buildPage(title: string, nextCursor?: string): BillingRecordPage {
  return {
    records: [
      {
        record_id: `record-${title}`,
        occurred_at: '2026-07-14T08:30:00Z',
        kind: 'ai_usage',
        title,
        credit_delta: -20,
        available_credits_after: 980,
      },
    ],
    ...(nextCursor ? { next_cursor: nextCursor } : {}),
  }
}

function buildAccount(consumptionScope: 'account' | 'member' = 'account') {
  return {
    billingAccountId: workspaceId.value === 'ws_1' ? 'ba_1' : 'ba_2',
    workspaceId: workspaceId.value,
    isShared: false,
    consumptionScope,
  }
}
