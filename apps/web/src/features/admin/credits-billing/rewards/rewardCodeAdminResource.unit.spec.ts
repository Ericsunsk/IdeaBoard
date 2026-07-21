import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createAdminRewardCodeResource } from './rewardCodeAdminResource'

const generated = vi.hoisted(() => ({
  create: vi.fn(),
  delete: vi.fn(),
  list: vi.fn(),
  setEnabled: vi.fn(),
  update: vi.fn(),
}))

vi.mock('../../../../shared/api/generated-query/admin-rewards/admin-rewards', () => ({
  createAdminRewardCode: generated.create,
  deleteAdminRewardCode: generated.delete,
  listAdminRewardCodes: generated.list,
  setAdminRewardCodeEnabled: generated.setEnabled,
  updateAdminRewardCode: generated.update,
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

  it('maps atomic rule and Code updates, enabled intent, and delete through generated APIs', async () => {
    generated.update.mockResolvedValue({
      code: 0,
      msg: 'ok',
      data: {
        reward_code: {
          ...rewardCode,
          display_name: '八月奖励',
          masked_code_hint: '•••• WXYZ',
          expires_at: undefined,
        },
        plaintext: 'NEWC-ODE1-WXYZ',
        replayed: false,
      },
    })
    generated.setEnabled.mockResolvedValue({
      code: 0,
      msg: 'ok',
      data: { ...rewardCode, enabled: false, status: 'disabled' },
    })
    generated.delete.mockResolvedValue({
      code: 0,
      msg: 'ok',
      data: { reward_code_id: 7 },
    })
    const resource = createAdminRewardCodeResource()

    await expect(
      resource.update(7, {
        displayName: ' 八月奖励 ',
        creditAmount: 120,
        maxRedemptions: 0,
        expiresAt: '',
        code: ' NEWC-ODE1-WXYZ ',
      }),
    ).resolves.toMatchObject({
      rewardCode: { displayName: '八月奖励', maskedCodeHint: '•••• WXYZ' },
      plaintext: 'NEWC-ODE1-WXYZ',
      replayed: false,
    })
    expect(generated.update).toHaveBeenCalledWith(7, {
      display_name: '八月奖励',
      credit_amount: 120,
      max_redemptions: 0,
      expires_at: null,
      code: 'NEWC-ODE1-WXYZ',
    })

    await expect(resource.setEnabled(7, false)).resolves.toMatchObject({
      enabled: false,
      status: 'disabled',
    })
    expect(generated.setEnabled).toHaveBeenCalledWith(7, { enabled: false })

    await expect(resource.delete(7)).resolves.toBeUndefined()
    expect(generated.delete).toHaveBeenCalledWith(7)
  })

  it('omits Code from a rule-only update', async () => {
    generated.update.mockResolvedValue({
      code: 0,
      msg: 'ok',
      data: { reward_code: rewardCode },
    })

    await createAdminRewardCodeResource().update(7, {
      displayName: '新用户奖励',
      creditAmount: 80,
      maxRedemptions: 100,
      expiresAt: '',
    })

    expect(generated.update).toHaveBeenCalledWith(7, {
      display_name: '新用户奖励',
      credit_amount: 80,
      max_redemptions: 100,
      expires_at: null,
    })
  })
})
