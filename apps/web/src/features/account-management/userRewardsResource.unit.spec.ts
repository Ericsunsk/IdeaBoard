import { beforeEach, describe, expect, it, vi } from 'vitest'
import {
  fetchUserRewardRedemptions,
  redeemUserRewardCode,
  userRewardRedemptionsQueryKeys,
} from './userRewardsResource'

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
            reward_code_display_name: '夏日灵感奖励',
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
      redemptions: [{ id: 42, reward_code_display_name: '夏日灵感奖励' }],
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
      message_safe: true,
    })

    await expect(fetchUserRewardRedemptions({ limit: 20 })).rejects.toThrow('奖励服务暂不可用')
  })

  it.each([
    [400, 'Invalid request.'],
    [422, 'This redemption code is invalid or has expired.'],
    [409, 'You have already redeemed this reward.'],
    [423, 'This account cannot redeem rewards right now.'],
    [429, 'Too many attempts. Please try again later.'],
    [500, 'Network error. Please try again.'],
  ])('maps redemption HTTP %d to stable user copy', async (status, message) => {
    generated.redeemUserReward.mockRejectedValue(
      Object.assign(new Error('unsafe upstream detail'), { status }),
    )

    await expect(redeemUserRewardCode('IDEA2026')).rejects.toThrow(message)
  })

  it('maps redemption transport failures to stable user copy', async () => {
    generated.redeemUserReward.mockRejectedValue(new Error('fetch failed'))

    await expect(redeemUserRewardCode('IDEA2026')).rejects.toThrow(
      'Network error. Please try again.',
    )
  })
})
