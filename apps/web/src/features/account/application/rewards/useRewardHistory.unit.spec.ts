import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { screen, waitFor } from '@testing-library/vue'
import { defineComponent } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { renderWithUser } from '../../../../test/render'
import type { RewardRedemptionPageData } from '../../infrastructure'
import { useRewardHistory } from './useRewardHistory'

const fetchRewardRedemptionsMock = vi.hoisted(() => vi.fn())

vi.mock('../../infrastructure/rewards/resource', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../../infrastructure/rewards/resource')>()
  return { ...actual, fetchRewardRedemptions: fetchRewardRedemptionsMock }
})

const RewardsHost = defineComponent({
  setup() {
    return useRewardHistory(true)
  },
  template: `
    <button type="button" @click="loadMore()">加载更多</button>
    <button type="button" @click="retryInitial()">重新加载</button>
    <output data-testid="reward-codes">{{ redemptions.map((item) => item.reward_code_display_name).join(',') }}</output>
    <output data-testid="initial-error">{{ initialError?.message }}</output>
    <output data-testid="load-more-error">{{ loadMoreError?.message }}</output>
    <output data-testid="has-more">{{ hasNextPage }}</output>
  `,
})

describe('useRewardHistory', () => {
  beforeEach(() => fetchRewardRedemptionsMock.mockReset())

  it('loads 20-record cursor pages and appends the next page', async () => {
    fetchRewardRedemptionsMock
      .mockResolvedValueOnce(buildPage('夏日灵感奖励', 41))
      .mockResolvedValueOnce(buildPage('新用户奖励'))
    const { user } = renderRewardsHost()

    await waitFor(() =>
      expect(screen.getByTestId('reward-codes')).toHaveTextContent('夏日灵感奖励'),
    )
    expect(fetchRewardRedemptionsMock).toHaveBeenNthCalledWith(
      1,
      { limit: 20 },
      expect.any(AbortSignal),
    )

    await user.click(screen.getByRole('button', { name: '加载更多' }))

    await waitFor(() =>
      expect(screen.getByTestId('reward-codes')).toHaveTextContent('夏日灵感奖励,新用户奖励'),
    )
    expect(fetchRewardRedemptionsMock).toHaveBeenNthCalledWith(
      2,
      { before_id: 41, limit: 20 },
      expect.any(AbortSignal),
    )
  })

  it('exposes separate recovery paths for initial and load-more failures', async () => {
    fetchRewardRedemptionsMock
      .mockRejectedValueOnce(new Error('首次加载失败'))
      .mockResolvedValueOnce(buildPage('夏日灵感奖励', 41))
      .mockRejectedValueOnce(new Error('下一页失败'))
      .mockResolvedValueOnce(buildPage('新用户奖励'))
    const { user } = renderRewardsHost()

    await waitFor(() =>
      expect(screen.getByTestId('initial-error')).toHaveTextContent('首次加载失败'),
    )
    await user.click(screen.getByRole('button', { name: '重新加载' }))
    await waitFor(() =>
      expect(screen.getByTestId('reward-codes')).toHaveTextContent('夏日灵感奖励'),
    )

    await user.click(screen.getByRole('button', { name: '加载更多' }))
    await waitFor(() =>
      expect(screen.getByTestId('load-more-error')).toHaveTextContent('下一页失败'),
    )
    await user.click(screen.getByRole('button', { name: '加载更多' }))
    await waitFor(() =>
      expect(screen.getByTestId('reward-codes')).toHaveTextContent('夏日灵感奖励,新用户奖励'),
    )
  })
})

function renderRewardsHost() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { gcTime: Number.POSITIVE_INFINITY, retry: false } },
  })
  return renderWithUser(RewardsHost, {
    global: { plugins: [[VueQueryPlugin, { queryClient }]] },
  })
}

function buildPage(rewardCodeDisplayName: string, nextBeforeID?: number): RewardRedemptionPageData {
  return {
    redemptions: [
      {
        id: rewardCodeDisplayName.length,
        reward_code_display_name: rewardCodeDisplayName,
        credited_amount: 80,
        masked_code_hint: '•••• 2922',
        redeemed_at: '2026-07-15T10:00:00Z',
      },
    ],
    ...(nextBeforeID ? { next_before_id: nextBeforeID } : {}),
  }
}
