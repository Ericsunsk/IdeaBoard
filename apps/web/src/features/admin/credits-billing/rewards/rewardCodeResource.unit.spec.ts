import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createAdminRewardCodeResource } from './rewardCodeResource'

const generated = vi.hoisted(() => ({ lookup: vi.fn(), revoke: vi.fn() }))

vi.mock('../../../../shared/api/generated-query/admin-rewards/admin-rewards', () => ({
  lookupAdminRewardCode: generated.lookup,
  revokeAdminRewardCode: generated.revoke,
}))

const code = {
  id: 17,
  campaign_id: 7,
  campaign_display_name: '新用户奖励',
  batch_id: 3,
  masked_code_hint: '•••• T293',
  status: 'available' as const,
  issued_at: '2026-07-16T01:00:00Z',
}

describe('AdminRewardCodeResource', () => {
  beforeEach(() => vi.clearAllMocks())

  it('uses POST-body exact lookup and maps masked metadata only', async () => {
    generated.lookup.mockResolvedValue({ code: 0, msg: 'ok', data: code })

    const result = await createAdminRewardCodeResource().lookup('SECRET-T293')

    expect(generated.lookup).toHaveBeenCalledWith({ code: 'SECRET-T293' })
    expect(result).toEqual({
      id: 17,
      campaignId: 7,
      campaignDisplayName: '新用户奖励',
      batchId: 3,
      maskedCodeHint: '•••• T293',
      status: 'available',
      issuedAt: '2026-07-16T01:00:00Z',
      revokedAt: undefined,
      redemptionId: undefined,
      redeemedAt: undefined,
    })
    expect(JSON.stringify(result)).not.toContain('SECRET-T293')
  })

  it('confirms an irreversible revoke with a normalized reason', async () => {
    generated.revoke.mockResolvedValue({
      code: 0,
      msg: 'ok',
      data: { ...code, status: 'revoked', revoked_at: '2026-07-16T02:00:00Z' },
    })

    const result = await createAdminRewardCodeResource().revoke(17, ' 运营撤销 ')

    expect(generated.revoke).toHaveBeenCalledWith(17, {
      confirmed: true,
      reason: '运营撤销',
    })
    expect(result.status).toBe('revoked')
  })
})
