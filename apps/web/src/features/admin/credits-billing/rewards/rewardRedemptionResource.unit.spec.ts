import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createAdminRewardRedemptionResource } from './rewardRedemptionResource'

const generated = vi.hoisted(() => ({ list: vi.fn() }))

vi.mock('../../../../shared/api/generated-query/admin-rewards/admin-rewards', () => ({
  listAdminRewardRedemptions: generated.list,
}))

describe('AdminRewardRedemptionResource', () => {
  beforeEach(() => vi.clearAllMocks())

  it('maps a Campaign-scoped cursor page without exposing raw Code', async () => {
    generated.list.mockResolvedValue({
      code: 0,
      msg: 'ok',
      data: {
        redemptions: [
          {
            id: 21,
            user_id: 3,
            credit_amount: 80,
            masked_code_hint: '•••• T293',
            credit_transaction_id: 'credit-transaction-293',
            redeemed_at: '2026-07-16T03:00:00Z',
          },
        ],
        next_before_id: 20,
      },
    })

    const page = await createAdminRewardRedemptionResource().list(7, 30)

    expect(generated.list).toHaveBeenCalledWith(7, { before_id: 30, limit: 20 })
    expect(page).toEqual({
      redemptions: [
        {
          id: 21,
          userId: 3,
          creditAmount: 80,
          maskedCodeHint: '•••• T293',
          creditTransactionId: 'credit-transaction-293',
          redeemedAt: '2026-07-16T03:00:00Z',
        },
      ],
      nextBeforeId: 20,
    })
  })
})
