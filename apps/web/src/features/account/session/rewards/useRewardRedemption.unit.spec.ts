import { QueryClient, type InfiniteData, type QueryFilters } from '@tanstack/vue-query'
import { describe, expect, it, vi } from 'vitest'
import { billingRecordsQueryKeys } from '../billing/resource'
import { creditsQueryKeys } from '../credits/resource'
import { rewardRedemptionsQueryKeys, type RewardRedemptionPageData } from './resource'
import { invalidateUserMoney } from '../invalidateUserMoney'
import { createRewardRedemptionWorkflow, prependRewardRedemption } from './useRewardRedemption'

const redeemed = {
  redemption: {
    id: 41,
    reward_code_id: 9,
    user_id: 1,
    credit_amount: 80,
    credit_transaction_id: 'credit-tx-41',
    available_balance_after: 180,
    redeemed_at: '2026-07-15T10:00:00Z',
  },
  credited_amount: 80,
  available_balance: 180,
  billing_account_id: 'ba_beneficiary_7001',
  replayed: false,
  history_item: {
    id: 41,
    reward_code_display_name: '夏日灵感奖励',
    credited_amount: 80,
    masked_code_hint: '•••• 2026',
    redeemed_at: '2026-07-15T10:00:00Z',
  },
}

describe('createRewardRedemptionWorkflow', () => {
  it('redeems through the resource and immediately synchronizes Idea reads', async () => {
    const redeem = vi.fn().mockResolvedValue(redeemed)
    const setAvailableBalance = vi.fn()
    const invalidateBillingRecords = vi.fn().mockResolvedValue(undefined)
    const prependRedemption = vi.fn()
    const invalidateRedemptions = vi.fn().mockResolvedValue(undefined)
    const workflow = createRewardRedemptionWorkflow({
      redeem,
      setAvailableBalance,
      invalidateBillingRecords,
      prependRedemption,
      invalidateRedemptions,
    })

    await expect(workflow.redeem('  IDEA-2026  ')).resolves.toEqual(redeemed)

    expect(redeem).toHaveBeenCalledWith('IDEA-2026')
    expect(setAvailableBalance).toHaveBeenCalledWith(180)
    expect(prependRedemption).toHaveBeenCalledWith(redeemed.history_item)
    expect(invalidateBillingRecords).toHaveBeenCalledOnce()
    expect(invalidateRedemptions).toHaveBeenCalledOnce()
    expect(workflow.pending.value).toBe(false)
  })

  it('does not mutate Idea reads when redemption fails', async () => {
    const error = new Error('network')
    const setAvailableBalance = vi.fn()
    const workflow = createRewardRedemptionWorkflow({
      redeem: vi.fn().mockRejectedValue(error),
      setAvailableBalance,
      invalidateBillingRecords: vi.fn(),
      prependRedemption: vi.fn(),
      invalidateRedemptions: vi.fn(),
    })

    await expect(workflow.redeem('IDEA2026')).rejects.toBe(error)
    expect(setAvailableBalance).not.toHaveBeenCalled()
    expect(workflow.pending.value).toBe(false)
  })

  it('does not turn a committed redemption into a failure when cache refresh fails', async () => {
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => undefined)
    const setAvailableBalance = vi.fn()
    const prependRedemption = vi.fn()
    const workflow = createRewardRedemptionWorkflow({
      redeem: vi.fn().mockResolvedValue(redeemed),
      setAvailableBalance,
      prependRedemption,
      invalidateBillingRecords: vi.fn().mockRejectedValue(new Error('billing refresh failed')),
      invalidateRedemptions: vi.fn().mockRejectedValue(new Error('rewards refresh failed')),
    })

    await expect(workflow.redeem('IDEA2026')).resolves.toEqual(redeemed)
    expect(setAvailableBalance).toHaveBeenCalledWith(180)
    expect(prependRedemption).toHaveBeenCalledWith(redeemed.history_item)
    expect(consoleError).toHaveBeenCalledTimes(2)
    expect(workflow.pending.value).toBe(false)
  })

  it('issues a wallet refetch through the production invalidation wiring', async () => {
    // Guards the redemption=>wallet link: the workflow delegates balance sync
    // to `setAvailableBalance`, and the production wiring routes that through
    // `invalidateUserMoney` (credits + billing roots). A regression that drops
    // that wiring would leave the wallet stale after redemption.
    const queryClient = new QueryClient()
    const invalidateQueries = vi.spyOn(queryClient, 'invalidateQueries')
    const workflow = createRewardRedemptionWorkflow({
      redeem: vi.fn().mockResolvedValue(redeemed),
      setAvailableBalance: () => invalidateUserMoney(queryClient),
      prependRedemption: (redemption) => prependRewardRedemption(queryClient, redemption),
      invalidateBillingRecords: () =>
        queryClient.invalidateQueries({ queryKey: billingRecordsQueryKeys.all, exact: false }),
      invalidateRedemptions: () =>
        queryClient.invalidateQueries({ queryKey: rewardRedemptionsQueryKeys.all, exact: false }),
    })

    await expect(workflow.redeem('IDEA2026')).resolves.toEqual(redeemed)

    expect(invalidateQueries).toHaveBeenCalledWith({
      queryKey: creditsQueryKeys.all,
      exact: false,
    } satisfies QueryFilters)
    expect(invalidateQueries).toHaveBeenCalledWith({
      queryKey: billingRecordsQueryKeys.all,
      exact: false,
    } satisfies QueryFilters)
    expect(invalidateQueries).toHaveBeenCalledWith({
      queryKey: rewardRedemptionsQueryKeys.all,
      exact: false,
    } satisfies QueryFilters)
  })

  it('prepends a new history item and never duplicates a replayed redemption', () => {
    const queryClient = new QueryClient()
    queryClient.setQueryData<InfiniteData<RewardRedemptionPageData, number>>(
      rewardRedemptionsQueryKeys.all,
      {
        pages: [
          {
            redemptions: [
              {
                id: 40,
                reward_code_display_name: '新用户奖励',
                credited_amount: 20,
                masked_code_hint: '•••• 0001',
                redeemed_at: '2026-07-14T10:00:00Z',
              },
            ],
          },
        ],
        pageParams: [0],
      },
    )

    prependRewardRedemption(queryClient, redeemed.history_item)
    prependRewardRedemption(queryClient, redeemed.history_item)

    const cached = queryClient.getQueryData<InfiniteData<RewardRedemptionPageData, number>>(
      rewardRedemptionsQueryKeys.all,
    )
    expect(cached?.pages[0]?.redemptions.map((item) => item.id)).toEqual([41, 40])
  })
})
