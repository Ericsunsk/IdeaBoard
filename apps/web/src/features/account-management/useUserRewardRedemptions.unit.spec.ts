import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { screen, waitFor } from '@testing-library/vue'
import { defineComponent } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { renderWithUser } from '../../test/render'
import type { UserRewardRedemptionPage } from './userRewardsResource'
import { useUserRewardRedemptions } from './useUserRewardRedemptions'

const fetchUserRewardRedemptionsMock = vi.hoisted(() => vi.fn())

vi.mock('./userRewardsResource', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./userRewardsResource')>()
  return { ...actual, fetchUserRewardRedemptions: fetchUserRewardRedemptionsMock }
})

const RewardsHost = defineComponent({
  setup() {
    return useUserRewardRedemptions(true)
  },
  template: `
    <button type="button" @click="loadMore()">加载更多</button>
    <button type="button" @click="retryInitial()">重新加载</button>
    <output data-testid="campaigns">{{ redemptions.map((item) => item.campaign_display_name).join(',') }}</output>
    <output data-testid="initial-error">{{ initialError?.message }}</output>
    <output data-testid="load-more-error">{{ loadMoreError?.message }}</output>
    <output data-testid="has-more">{{ hasNextPage }}</output>
  `,
})

describe('useUserRewardRedemptions', () => {
  beforeEach(() => fetchUserRewardRedemptionsMock.mockReset())

  it('loads 20-record cursor pages and appends the next page', async () => {
    fetchUserRewardRedemptionsMock
      .mockResolvedValueOnce(buildPage('夏日灵感奖励', 41))
      .mockResolvedValueOnce(buildPage('新用户奖励'))
    const { user } = renderRewardsHost()

    await waitFor(() => expect(screen.getByTestId('campaigns')).toHaveTextContent('夏日灵感奖励'))
    expect(fetchUserRewardRedemptionsMock).toHaveBeenNthCalledWith(
      1,
      { limit: 20 },
      expect.any(AbortSignal),
    )

    await user.click(screen.getByRole('button', { name: '加载更多' }))

    await waitFor(() =>
      expect(screen.getByTestId('campaigns')).toHaveTextContent('夏日灵感奖励,新用户奖励'),
    )
    expect(fetchUserRewardRedemptionsMock).toHaveBeenNthCalledWith(
      2,
      { before_id: 41, limit: 20 },
      expect.any(AbortSignal),
    )
  })

  it('exposes separate recovery paths for initial and load-more failures', async () => {
    fetchUserRewardRedemptionsMock
      .mockRejectedValueOnce(new Error('首次加载失败'))
      .mockResolvedValueOnce(buildPage('夏日灵感奖励', 41))
      .mockRejectedValueOnce(new Error('下一页失败'))
      .mockResolvedValueOnce(buildPage('新用户奖励'))
    const { user } = renderRewardsHost()

    await waitFor(() =>
      expect(screen.getByTestId('initial-error')).toHaveTextContent('首次加载失败'),
    )
    await user.click(screen.getByRole('button', { name: '重新加载' }))
    await waitFor(() => expect(screen.getByTestId('campaigns')).toHaveTextContent('夏日灵感奖励'))

    await user.click(screen.getByRole('button', { name: '加载更多' }))
    await waitFor(() =>
      expect(screen.getByTestId('load-more-error')).toHaveTextContent('下一页失败'),
    )
    await user.click(screen.getByRole('button', { name: '加载更多' }))
    await waitFor(() =>
      expect(screen.getByTestId('campaigns')).toHaveTextContent('夏日灵感奖励,新用户奖励'),
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

function buildPage(campaignDisplayName: string, nextBeforeID?: number): UserRewardRedemptionPage {
  return {
    redemptions: [
      {
        id: campaignDisplayName.length,
        campaign_display_name: campaignDisplayName,
        credited_amount: 80,
        masked_code_hint: '•••• 2922',
        redeemed_at: '2026-07-15T10:00:00Z',
      },
    ],
    ...(nextBeforeID ? { next_before_id: nextBeforeID } : {}),
  }
}
