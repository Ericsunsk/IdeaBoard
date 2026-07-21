import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createAdminRewardCodeResource } from './rewardCodeAdminResource'

const generated = vi.hoisted(() => ({ create: vi.fn(), list: vi.fn() }))

vi.mock('../../../../shared/api/generated-query/admin-rewards/admin-rewards', () => ({
  createAdminRewardCode: generated.create,
  listAdminRewardCodes: generated.list,
}))

const rewardCode = {
  id: 7,
  display_name: '新用户奖励',
  masked_code_hint: '•••• 2026',
  credit_amount: 80,
  max_redemptions: 100,
  redemption_count: 12,
  enabled: true,
  status: 'enabled' as const,
  created_by: 1,
  created_at: '2026-07-20T10:00:00Z',
  updated_at: '2026-07-20T10:00:00Z',
}

describe('AdminRewardCodeResource', () => {
  beforeEach(() => vi.clearAllMocks())

  it('maps list metadata without exposing digest or plaintext', async () => {
    generated.list.mockResolvedValue({
      code: 0,
      msg: 'ok',
      data: { reward_codes: [rewardCode], next_before_id: 6 },
    })

    const page = await createAdminRewardCodeResource().list(8)

    expect(generated.list).toHaveBeenCalledWith({ before_id: 8, limit: 20 })
    expect(page).toEqual({
      rewardCodes: [
        expect.objectContaining({
          id: 7,
          displayName: '新用户奖励',
          maskedCodeHint: '•••• 2026',
          status: 'enabled',
        }),
      ],
      nextBeforeId: 6,
    })
  })

  it('preserves the client idempotency key and converts Shanghai expiration', async () => {
    generated.create.mockResolvedValue({
      code: 0,
      msg: 'ok',
      data: { reward_code: rewardCode, plaintext: 'ABCD12342026', replayed: false },
    })

    const result = await createAdminRewardCodeResource().create({
      displayName: ' 新用户奖励 ',
      code: 'ABCD-1234-2026',
      creditAmount: 80,
      maxRedemptions: 100,
      expiresAt: '2026-08-01T00:00',
      idempotencyKey: 'stable-create-key',
    })

    expect(generated.create).toHaveBeenCalledWith({
      display_name: '新用户奖励',
      code: 'ABCD-1234-2026',
      credit_amount: 80,
      max_redemptions: 100,
      expires_at: '2026-07-31T16:00:00.000Z',
      idempotency_key: 'stable-create-key',
    })
    expect(result).toEqual({
      rewardCode: expect.objectContaining({ id: 7, displayName: '新用户奖励' }),
      plaintext: 'ABCD12342026',
      replayed: false,
    })
  })
})
