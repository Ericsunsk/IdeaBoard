import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { screen, waitFor } from '@testing-library/vue'
import { defineComponent } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { notifyUserCreditsInvalidation } from '../../shared/query/userCreditsInvalidation'
import { renderWithUser } from '../../test/render'
import type { UserBillingRecordPage } from './userBillingRecordsResource'
import { useUserBillingRecords } from './useUserBillingRecords'

const fetchUserBillingRecordsMock = vi.hoisted(() => vi.fn())

vi.mock('./userBillingRecordsResource', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./userBillingRecordsResource')>()
  return { ...actual, fetchUserBillingRecords: fetchUserBillingRecordsMock }
})

const BillingHost = defineComponent({
  setup() {
    return useUserBillingRecords(true)
  },
  template: `
    <button type="button" @click="loadMore()">加载更多</button>
    <output data-testid="record-titles">{{ records.map((record) => record.title).join(',') }}</output>
    <output data-testid="has-more">{{ hasNextPage }}</output>
  `,
})

describe('useUserBillingRecords', () => {
  beforeEach(() => fetchUserBillingRecordsMock.mockReset())

  it('loads 20-record cursor pages and appends the next page', async () => {
    fetchUserBillingRecordsMock
      .mockResolvedValueOnce(buildPage('局部重绘', 91))
      .mockResolvedValueOnce(buildPage('活动奖励'))
    const { user } = renderBillingHost()

    await waitFor(() => expect(screen.getByTestId('record-titles')).toHaveTextContent('局部重绘'))
    expect(fetchUserBillingRecordsMock).toHaveBeenNthCalledWith(1, { limit: 20 }, expect.any(AbortSignal))
    expect(screen.getByTestId('has-more')).toHaveTextContent('true')

    await user.click(screen.getByRole('button', { name: '加载更多' }))

    await waitFor(() =>
      expect(screen.getByTestId('record-titles')).toHaveTextContent('局部重绘,活动奖励'),
    )
    expect(fetchUserBillingRecordsMock).toHaveBeenNthCalledWith(
      2,
      { before_sequence: 91, limit: 20 },
      expect.any(AbortSignal),
    )
  })

  it('refetches the first page after an existing Credits settlement event', async () => {
    fetchUserBillingRecordsMock.mockResolvedValue(buildPage('草图成图'))
    renderBillingHost()
    await waitFor(() => expect(screen.getByTestId('record-titles')).toHaveTextContent('草图成图'))
    expect(fetchUserBillingRecordsMock).toHaveBeenCalledOnce()

    notifyUserCreditsInvalidation({
      source: 'task-settlement',
      taskId: 'task-7',
      status: 'completed',
    })

    await waitFor(() => expect(fetchUserBillingRecordsMock).toHaveBeenCalledTimes(2))
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

function buildPage(title: string, nextBeforeSequence?: number): UserBillingRecordPage {
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
    ...(nextBeforeSequence ? { next_before_sequence: nextBeforeSequence } : {}),
  }
}
