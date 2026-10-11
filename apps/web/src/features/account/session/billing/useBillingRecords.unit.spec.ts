import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { screen, waitFor } from '@testing-library/vue'
import { defineComponent, ref } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { notifyUserCreditsInvalidation } from '../../../../core/query/userCreditsInvalidation'
import { renderWithUser } from '../../../../test/render'
import type { BillingRecordPageData } from './resource'
import { useBillingRecords } from './useBillingRecords'

const fetchBillingRecordsMock = vi.hoisted(() => vi.fn())

vi.mock('./resource', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./resource')>()
  return { ...actual, fetchBillingRecords: fetchBillingRecordsMock }
})

vi.mock('../accountWorkspace', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../accountWorkspace')>()),
  useAccountWorkspace: () => ({
    workspaceId: ref('ws_1'),
    selectionSettled: ref(true),
  }),
}))

const BillingHost = defineComponent({
  setup() {
    return useBillingRecords(true)
  },
  template: `
    <button type="button" @click="loadMore()">加载更多</button>
    <output data-testid="record-titles">{{ records.map((record) => record.title).join(',') }}</output>
    <output data-testid="has-more">{{ hasNextPage }}</output>
  `,
})

describe('useBillingRecords', () => {
  beforeEach(() => fetchBillingRecordsMock.mockReset())

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
  return renderWithUser(BillingHost, {
    global: { plugins: [[VueQueryPlugin, { queryClient }]] },
  })
}

function buildPage(title: string, nextCursor?: string): BillingRecordPageData {
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
