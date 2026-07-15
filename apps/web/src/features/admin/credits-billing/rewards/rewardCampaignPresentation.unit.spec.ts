import { describe, expect, it } from 'vitest'
import type { RewardCampaign } from './rewardCampaignResource'
import {
  createRewardCampaignView,
  rewardCampaignStatusLabel,
} from './rewardCampaignPresentation'

describe('reward campaign presentation', () => {
  it.each([
    ['draft', '草稿'],
    ['cancelled', '已取消'],
    ['expired', '已过期'],
    ['exhausted', '已耗尽'],
    ['paused', '已暂停'],
    ['scheduled', '待开始'],
    ['active', '进行中'],
  ] as const)('maps the derived %s status to %s', (status, label) => {
    expect(rewardCampaignStatusLabel(status)).toBe(label)
  })

  it('formats the Shanghai schedule and bounded redemption progress', () => {
    const view = createRewardCampaignView(
      makeCampaign({ redemptionCount: 120, maxRedemptions: 100 }),
    )

    expect(view.scheduleLabel).toBe('2026/07/15 18:30 - 2026/08/01 00:00')
    expect(view.progressLabel).toBe('120 / 100')
    expect(view.progressPercent).toBe(100)
    expect(view.lifecycleLabel).toBe('已激活')
  })
})

function makeCampaign(overrides: Partial<RewardCampaign> = {}): RewardCampaign {
  return {
    id: 7,
    displayName: '新用户奖励',
    internalNote: '七月活动',
    creditAmount: 80,
    mode: 'public',
    lifecycleStatus: 'active',
    status: 'scheduled',
    maxRedemptions: 100,
    redemptionCount: 12,
    startsAt: '2026-07-15T10:30:00Z',
    expiresAt: '2026-07-31T16:00:00Z',
    createdBy: 1,
    createdAt: '2026-07-15T10:00:00Z',
    updatedAt: '2026-07-15T10:00:00Z',
    ...overrides,
  }
}
