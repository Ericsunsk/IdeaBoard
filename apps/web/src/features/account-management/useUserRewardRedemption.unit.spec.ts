import { describe, expect, it, vi } from 'vitest'
import { createUserRewardRedemptionWorkflow } from './useUserRewardRedemption'

const redeemed = {
  redemption: {
    id: 41,
    campaign_id: 7,
    code_id: 9,
    user_id: 1,
    credit_amount: 80,
    credit_transaction_id: 'credit-tx-41',
    available_balance_after: 180,
    redeemed_at: '2026-07-15T10:00:00Z',
  },
  credited_amount: 80,
  available_balance: 180,
  replayed: false,
}

describe('createUserRewardRedemptionWorkflow', () => {
  it('redeems through the resource and immediately synchronizes Idea reads', async () => {
    const redeem = vi.fn().mockResolvedValue(redeemed)
    const setAvailableBalance = vi.fn()
    const invalidateBillingRecords = vi.fn().mockResolvedValue(undefined)
    const workflow = createUserRewardRedemptionWorkflow({
      redeem,
      setAvailableBalance,
      invalidateBillingRecords,
    })

    await expect(workflow.redeem('  IDEA-2026  ')).resolves.toEqual(redeemed)

    expect(redeem).toHaveBeenCalledWith('IDEA-2026')
    expect(setAvailableBalance).toHaveBeenCalledWith(180)
    expect(invalidateBillingRecords).toHaveBeenCalledOnce()
    expect(workflow.pending.value).toBe(false)
  })

  it('does not mutate Idea reads when redemption fails', async () => {
    const error = new Error('network')
    const setAvailableBalance = vi.fn()
    const workflow = createUserRewardRedemptionWorkflow({
      redeem: vi.fn().mockRejectedValue(error),
      setAvailableBalance,
      invalidateBillingRecords: vi.fn(),
    })

    await expect(workflow.redeem('IDEA2026')).rejects.toBe(error)
    expect(setAvailableBalance).not.toHaveBeenCalled()
    expect(workflow.pending.value).toBe(false)
  })
})
