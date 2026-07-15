import { describe, expect, it, vi } from 'vitest'
import type { RewardCampaign } from './rewardCampaignResource'
import {
  getRewardCampaignActionDescriptor,
  type RewardCampaignActionResource,
  type RewardCampaignAuditedAction,
} from './rewardCampaignActions'

describe('reward campaign action descriptors', () => {
  it.each([
    ['issue', '确认发行公共兑换码', '发行公共兑换码失败', 'warn'],
    ['activate', '确认激活奖励活动', '激活奖励活动失败', 'warn'],
    ['pause', '确认暂停奖励活动', '暂停奖励活动失败', 'warn'],
    ['resume', '确认恢复奖励活动', '恢复奖励活动失败', 'warn'],
    ['cancel', '确认取消奖励活动', '取消奖励活动失败', 'danger'],
    ['delete', '确认删除空 Draft', '删除奖励活动失败', 'danger'],
  ] as const)(
    'maps %s to one dialog, fallback, and intent descriptor',
    (action, title, errorFallback, intent) => {
      const descriptor = getRewardCampaignActionDescriptor(action)

      expect(descriptor.dialog('七月奖励')).toEqual(
        expect.objectContaining({ title, intent, message: expect.stringContaining('七月奖励') }),
      )
      expect(descriptor.errorFallback).toBe(errorFallback)
      expect(descriptor.successMessage).toBeTruthy()
    },
  )

  it('dispatches every action through its descriptor command', async () => {
    const resource = makeResource()
    const campaign = makeCampaign()
    const actions: RewardCampaignAuditedAction[] = [
      'issue',
      'activate',
      'pause',
      'resume',
      'cancel',
      'delete',
    ]

    const results = await Promise.all(
      actions.map((action) =>
        getRewardCampaignActionDescriptor(action).execute({
          resource,
          campaign,
          publicCode: 'IDEA2026',
          reason: '运营操作',
        }),
      ),
    )

    expect(resource.issuePublicCode).toHaveBeenCalledWith(7, {
      code: 'IDEA2026',
      reason: '运营操作',
    })
    expect(resource.activate).toHaveBeenCalledWith(7, '运营操作')
    expect(resource.pause).toHaveBeenCalledWith(7, '运营操作')
    expect(resource.resume).toHaveBeenCalledWith(7, '运营操作')
    expect(resource.cancel).toHaveBeenCalledWith(7, '运营操作')
    expect(resource.delete).toHaveBeenCalledWith(7, '运营操作')
    expect(results.map((result) => result.type)).toEqual([
      'issued',
      'campaign',
      'campaign',
      'campaign',
      'campaign',
      'deleted',
    ])
  })
})

function makeResource() {
  const campaign = makeCampaign()
  return {
    issuePublicCode: vi.fn().mockResolvedValue({ plaintext: 'IDEA2026' }),
    activate: vi.fn().mockResolvedValue(campaign),
    pause: vi.fn().mockResolvedValue(campaign),
    resume: vi.fn().mockResolvedValue(campaign),
    cancel: vi.fn().mockResolvedValue(campaign),
    delete: vi.fn().mockResolvedValue(undefined),
  } satisfies RewardCampaignActionResource
}

function makeCampaign(): RewardCampaign {
  return {
    id: 7,
    displayName: '七月奖励',
    internalNote: '',
    creditAmount: 80,
    mode: 'public',
    lifecycleStatus: 'active',
    status: 'active',
    maxRedemptions: 100,
    redemptionCount: 0,
    createdBy: 1,
    createdAt: '2026-07-15T10:00:00Z',
    updatedAt: '2026-07-15T10:00:00Z',
  }
}
