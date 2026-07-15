import { beforeEach, describe, expect, it, vi } from 'vitest'
import {
  createAdminRewardCampaignResource,
  RewardCampaignConflictError,
} from './rewardCampaignResource'

const generated = vi.hoisted(() => ({
  activate: vi.fn(),
  cancel: vi.fn(),
  create: vi.fn(),
  delete: vi.fn(),
  issue: vi.fn(),
  list: vi.fn(),
  pause: vi.fn(),
  resume: vi.fn(),
  update: vi.fn(),
}))

vi.mock('../../../../shared/api/generated-query/admin-rewards/admin-rewards', () => ({
  activateAdminRewardCampaign: generated.activate,
  cancelAdminRewardCampaign: generated.cancel,
  createAdminRewardCampaign: generated.create,
  deleteAdminRewardCampaign: generated.delete,
  issueAdminRewardPublicCode: generated.issue,
  listAdminRewardCampaigns: generated.list,
  pauseAdminRewardCampaign: generated.pause,
  resumeAdminRewardCampaign: generated.resume,
  updateAdminRewardCampaign: generated.update,
}))

const campaign = {
  id: 7,
  display_name: '新用户奖励',
  internal_note: '七月活动',
  credit_amount: 80,
  mode: 'public' as const,
  lifecycle_status: 'active' as const,
  status: 'scheduled' as const,
  max_redemptions: 100,
  redemption_count: 12,
  issued_code_count: 40,
  valid_unused_code_count: 28,
  starts_at: '2026-07-15T10:30:00Z',
  expires_at: '2026-07-31T16:00:00Z',
  created_by: 1,
  created_at: '2026-07-15T10:00:00Z',
  updated_at: '2026-07-15T10:00:00Z',
}

describe('AdminRewardCampaignResource', () => {
  beforeEach(() => vi.clearAllMocks())

  it('maps lifecycle, derived status, schedule, cap, and redemption progress from list reads', async () => {
    generated.list.mockResolvedValue({
      code: 0,
      msg: 'ok',
      data: { campaigns: [campaign], next_before_id: 6 },
    })

    const page = await createAdminRewardCampaignResource().list()

    expect(page).toEqual({
      campaigns: [
        expect.objectContaining({
          lifecycleStatus: 'active',
          status: 'scheduled',
          startsAt: '2026-07-15T10:30:00Z',
          expiresAt: '2026-07-31T16:00:00Z',
          maxRedemptions: 100,
          redemptionCount: 12,
          issuedCodeCount: 40,
          validUnusedCodeCount: 28,
        }),
      ],
      nextBeforeId: 6,
    })
  })

  it('converts Asia/Shanghai schedule input to the UTC create contract', async () => {
    generated.create.mockResolvedValue({ code: 0, msg: 'ok', data: campaign })
    const resource = createAdminRewardCampaignResource()

    await resource.create({
      displayName: ' 新用户奖励 ',
      internalNote: ' 七月活动 ',
      creditAmount: 80,
      mode: 'single_use',
      maxRedemptions: 100,
      startsAt: '2026-07-15T18:30',
      expiresAt: '2026-08-01T00:00',
      reason: ' 创建活动 ',
    })

    expect(generated.create).toHaveBeenCalledWith({
      display_name: '新用户奖励',
      internal_note: '七月活动',
      credit_amount: 80,
      mode: 'single_use',
      max_redemptions: 100,
      starts_at: '2026-07-15T10:30:00.000Z',
      expires_at: '2026-07-31T16:00:00.000Z',
      reason: '创建活动',
    })
  })

  it('maps updates, one-time Code issuance, and lifecycle commands', async () => {
    generated.update.mockResolvedValue({ code: 0, msg: 'ok', data: campaign })
    generated.issue.mockResolvedValue({
      code: 0,
      msg: 'ok',
      data: {
        code: {
          id: 9,
          campaign_id: 7,
          hint: '2026',
          issued_by: 1,
          issued_at: campaign.created_at,
        },
        plaintext: 'IDEA2026',
      },
    })
    generated.activate.mockResolvedValue({ code: 0, msg: 'ok', data: campaign })
    generated.pause.mockResolvedValue({ code: 0, msg: 'ok', data: campaign })
    generated.resume.mockResolvedValue({ code: 0, msg: 'ok', data: campaign })
    generated.cancel.mockResolvedValue({
      code: 0,
      msg: 'ok',
      data: { ...campaign, lifecycle_status: 'cancelled', status: 'cancelled' },
    })
    generated.delete.mockResolvedValue({ code: 0, msg: 'ok' })
    const resource = createAdminRewardCampaignResource()

    await resource.update(7, {
      displayName: ' 新用户奖励 ',
      internalNote: ' 仅修改备注 ',
      creditAmount: 80,
      maxRedemptions: 100,
      startsAt: '',
      expiresAt: '',
      reason: ' 更新备注 ',
    })
    const issued = await resource.issuePublicCode(7, {
      code: ' idea-2026 ',
      reason: ' 发行 ',
    })
    await resource.activate(7, ' 激活 ')
    await resource.pause(7, ' 暂停 ')
    await resource.resume(7, ' 恢复 ')
    await resource.cancel(7, ' 取消 ')
    await resource.delete(7, ' 删除空 Draft ')

    expect(generated.update).toHaveBeenCalledWith(7, {
      display_name: '新用户奖励',
      internal_note: '仅修改备注',
      credit_amount: 80,
      max_redemptions: 100,
      starts_at: null,
      expires_at: null,
      reason: '更新备注',
    })
    expect(issued.plaintext).toBe('IDEA2026')
    expect(generated.activate).toHaveBeenCalledWith(7, { reason: '激活' })
    expect(generated.pause).toHaveBeenCalledWith(7, { reason: '暂停' })
    expect(generated.resume).toHaveBeenCalledWith(7, { reason: '恢复' })
    expect(generated.cancel).toHaveBeenCalledWith(7, { confirmed: true, reason: '取消' })
    expect(generated.delete).toHaveBeenCalledWith(7, { reason: '删除空 Draft' })
  })

  it('maps update and delete conflicts to stable feature error kinds', async () => {
    generated.update.mockResolvedValue({ code: 409, msg: '奖励活动状态冲突' })
    generated.delete.mockResolvedValue({ code: 409, msg: '奖励活动状态冲突' })
    const resource = createAdminRewardCampaignResource()
    const input = {
      displayName: '新用户奖励',
      internalNote: '备注',
      creditAmount: 80,
      maxRedemptions: 100,
      startsAt: '',
      expiresAt: '',
      reason: '运营调整',
    }

    await expect(resource.update(7, input)).rejects.toEqual(
      expect.objectContaining<Partial<RewardCampaignConflictError>>({ kind: 'rules_frozen' }),
    )
    await expect(resource.delete(7, '清理 Draft')).rejects.toEqual(
      expect.objectContaining<Partial<RewardCampaignConflictError>>({ kind: 'draft_not_empty' }),
    )
  })
})
