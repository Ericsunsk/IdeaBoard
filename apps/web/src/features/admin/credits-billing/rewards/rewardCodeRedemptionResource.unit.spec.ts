import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createRewardCodeRedemptionResource } from './rewardCodeRedemptionResource'

const generated = vi.hoisted(() => ({
  detail: vi.fn(),
  list: vi.fn(),
}))

vi.mock('../../../../shared/api/generated-query/admin-rewards/admin-rewards', () => ({
  getAdminRewardCodeRedemption: generated.detail,
  listAdminRewardCodeRedemptions: generated.list,
}))

describe('RewardCodeRedemptionResource', () => {
  beforeEach(() => vi.clearAllMocks())

  it('maps a keyset page to RewardCode vocabulary and forwards cancellation', async () => {
    generated.list.mockResolvedValue({
      code: 0,
      msg: 'ok',
      data: {
        redemptions: [
          {
            id: 51,
            reward_code_id: 7,
            user_id: 23,
            credited_amount: 80,
            redeemed_at: '2026-07-22T03:04:05Z',
          },
        ],
        next_before_id: 40,
      },
    })
    const signal = new AbortController().signal

    await expect(
      createRewardCodeRedemptionResource().list(7, { beforeId: 60, limit: 10, signal }),
    ).resolves.toEqual({
      redemptions: [
        {
          id: 51,
          rewardCodeId: 7,
          userId: 23,
          creditedAmount: 80,
          redeemedAt: '2026-07-22T03:04:05Z',
        },
      ],
      nextBeforeId: 40,
    })
    expect(generated.list).toHaveBeenCalledWith(7, { before_id: 60, limit: 10 }, { signal })
  })

  it('maps technical detail without exposing Identity PII or Code secrets', async () => {
    generated.detail.mockResolvedValue({
      code: 0,
      msg: 'ok',
      data: {
        id: 51,
        reward_code_id: 7,
        user_id: 23,
        credited_amount: 80,
        redeemed_at: '2026-07-22T03:04:05Z',
        credit_transaction_id: 'credit-transaction-9001',
        masked_code_hint: '•••• 2026',
      },
    })
    const signal = new AbortController().signal

    await expect(createRewardCodeRedemptionResource().get(7, 51, { signal })).resolves.toEqual({
      id: 51,
      rewardCodeId: 7,
      userId: 23,
      creditedAmount: 80,
      redeemedAt: '2026-07-22T03:04:05Z',
      creditTransactionId: 'credit-transaction-9001',
      maskedCodeHint: '•••• 2026',
    })
    expect(generated.detail).toHaveBeenCalledWith(7, 51, { signal })
  })
})
