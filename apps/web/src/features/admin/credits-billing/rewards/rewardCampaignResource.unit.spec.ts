import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createAdminRewardCampaignResource } from './rewardCampaignResource'

const generated = vi.hoisted(() => ({
  list: vi.fn(),
  create: vi.fn(),
  issue: vi.fn(),
  activate: vi.fn(),
}))

vi.mock('../../../../shared/api/generated-query/admin-rewards/admin-rewards', () => ({
  listAdminRewardCampaigns: generated.list,
  createAdminRewardCampaign: generated.create,
  issueAdminRewardPublicCode: generated.issue,
  activateAdminRewardCampaign: generated.activate,
}))

const campaign = {
  id: 7,
  display_name: '新用户奖励',
  internal_note: '',
  credit_amount: 80,
  mode: 'public' as const,
  status: 'draft' as const,
  max_redemptions: 100,
  redemption_count: 0,
  created_by: 1,
  created_at: '2026-07-15T10:00:00Z',
  updated_at: '2026-07-15T10:00:00Z',
}

describe('AdminRewardCampaignResource', () => {
  beforeEach(() => vi.clearAllMocks())

  it('maps create, one-time public Code issuance, and activation commands', async () => {
    generated.create.mockResolvedValue({ code: 0, msg: 'ok', data: campaign })
    generated.issue.mockResolvedValue({
      code: 0,
      msg: 'ok',
      data: { code: { id: 9, campaign_id: 7, hint: '2026', issued_by: 1, issued_at: campaign.created_at }, plaintext: 'IDEA2026' },
    })
    generated.activate.mockResolvedValue({
      code: 0,
      msg: 'ok',
      data: { ...campaign, status: 'active' },
    })
    const resource = createAdminRewardCampaignResource()

    await resource.create({
      displayName: ' 新用户奖励 ', internalNote: '', creditAmount: 80,
      maxRedemptions: 100, reason: ' 创建活动 ',
    })
    const issued = await resource.issuePublicCode(7, { code: ' idea-2026 ', reason: ' 发行 ' })
    const activated = await resource.activate(7, ' 激活 ')

    expect(generated.create).toHaveBeenCalledWith({
      display_name: '新用户奖励', internal_note: '', credit_amount: 80,
      max_redemptions: 100, reason: '创建活动',
    })
    expect(generated.issue).toHaveBeenCalledWith(7, { code: 'idea-2026', reason: '发行' })
    expect(issued.plaintext).toBe('IDEA2026')
    expect(generated.activate).toHaveBeenCalledWith(7, { reason: '激活' })
    expect(activated.status).toBe('active')
  })
})
