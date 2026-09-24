import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createRewardCodeRedemptionResource } from './rewardCodeRedemptionResource'

const transport = vi.hoisted(() => ({
  request: vi.fn(),
}))

vi.mock('../../../../shared/api/client', () => ({
  api: { request: transport.request },
}))

describe('RewardCodeRedemptionResource', () => {
  beforeEach(() => vi.clearAllMocks())

  it('maps a keyset page to RewardCode vocabulary and forwards cancellation', async () => {
    transport.request.mockResolvedValue({
      code: 0,
      msg: 'ok',
      data: {
        redemptions: [
          {
            id: 51,
            reward_code_id: 7,
            user_public_id: '22222222-2222-4222-8222-222222222222',
            display_name: '兑换用户',
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
          userPublicId: '22222222-2222-4222-8222-222222222222',
          displayName: '兑换用户',
          creditedAmount: 80,
          redeemedAt: '2026-07-22T03:04:05Z',
        },
      ],
      nextBeforeId: 40,
    })
    expect(transport.request).toHaveBeenCalledWith(
      '/admin/rewards/codes/7/redemptions?before_id=60&limit=10',
      expect.objectContaining({ method: 'GET', signal }),
    )
  })

  it('maps technical detail without exposing Identity PII or Code secrets', async () => {
    transport.request.mockResolvedValue({
      code: 0,
      msg: 'ok',
      data: {
        id: 51,
        reward_code_id: 7,
        user_public_id: '22222222-2222-4222-8222-222222222222',
        display_name: '兑换用户',
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
      userPublicId: '22222222-2222-4222-8222-222222222222',
      displayName: '兑换用户',
      creditedAmount: 80,
      redeemedAt: '2026-07-22T03:04:05Z',
      creditTransactionId: 'credit-transaction-9001',
      maskedCodeHint: '•••• 2026',
    })
    expect(transport.request).toHaveBeenCalledWith(
      '/admin/rewards/codes/7/redemptions/51',
      expect.objectContaining({ method: 'GET', signal }),
    )
  })
})
