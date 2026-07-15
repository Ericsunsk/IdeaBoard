import { beforeEach, describe, expect, it, vi } from 'vitest'
import { fetchUserRewardRedemptions, userRewardRedemptionsQueryKeys } from './userRewardsResource'

const generated = vi.hoisted(() => ({
  listUserRewardRedemptions: vi.fn(),
  redeemUserReward: vi.fn(),
}))

vi.mock('../../shared/api/generated-query/user-rewards/user-rewards', () => generated)

describe('userRewardsResource', () => {
  beforeEach(() => vi.clearAllMocks())

  it('reads a cursor page through the feature resource boundary', async () => {
    generated.listUserRewardRedemptions.mockResolvedValue({
      code: 0,
      data: {
        redemptions: [
          {
            id: 42,
            campaign_display_name: '夏日灵感奖励',
            credited_amount: 80,
            masked_code_hint: '•••• 2922',
            redeemed_at: '2026-07-15T10:00:00Z',
          },
        ],
        next_before_id: 40,
      },
    })

    const controller = new AbortController()
    await expect(
      fetchUserRewardRedemptions({ before_id: 50, limit: 20 }, controller.signal),
    ).resolves.toMatchObject({
      redemptions: [{ id: 42, campaign_display_name: '夏日灵感奖励' }],
      next_before_id: 40,
    })
    expect(generated.listUserRewardRedemptions).toHaveBeenCalledWith(
      { before_id: 50, limit: 20 },
      { signal: controller.signal },
    )
    expect(userRewardRedemptionsQueryKeys.all).toEqual(['user', 'rewards', 'redemptions'])
  })

  it('surfaces API feedback instead of manufacturing an empty page', async () => {
    generated.listUserRewardRedemptions.mockResolvedValue({
      code: 500,
      msg: '奖励服务暂不可用',
    })

    await expect(fetchUserRewardRedemptions({ limit: 20 })).rejects.toThrow('奖励服务暂不可用')
  })
})
