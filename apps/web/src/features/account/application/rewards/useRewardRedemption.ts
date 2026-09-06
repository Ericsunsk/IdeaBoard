import { ref } from 'vue'
import { i18n } from '../../../../shared/i18n'
import { useQueryClient, type InfiniteData, type QueryClient } from '@tanstack/vue-query'
import {
  billingRecordsQueryKeys,
  creditsQueryKeys,
  redeemRewardCode,
  rewardRedemptionsQueryKeys,
  type CreditWalletSummary,
  type RewardRedeemResult,
  type RewardRedemptionItem,
  type RewardRedemptionPageData,
} from '../../infrastructure'

export type RewardRedemptionDependencies = {
  redeem: (code: string) => Promise<RewardRedeemResult>
  setAvailableBalance: (availableBalance: number) => void
  prependRedemption: (redemption: RewardRedemptionItem) => void
  invalidateBillingRecords: () => Promise<unknown>
  invalidateRedemptions: () => Promise<unknown>
}

export function createRewardRedemptionWorkflow(dependencies: RewardRedemptionDependencies) {
  const pending = ref(false)

  async function redeem(code: string): Promise<RewardRedeemResult> {
    if (pending.value) throw new Error(i18n.global.t('account.errors.redeemInProgress'))
    pending.value = true
    try {
      const result = await dependencies.redeem(code.trim())
      dependencies.setAvailableBalance(result.available_balance)
      dependencies.prependRedemption(result.history_item)
      const refreshResults = await Promise.allSettled([
        dependencies.invalidateBillingRecords(),
        dependencies.invalidateRedemptions(),
      ])
      for (const refreshResult of refreshResults) {
        if (refreshResult.status === 'rejected') {
          console.error('[reward-redemption] read invalidation failed', refreshResult.reason)
        }
      }
      return result
    } finally {
      pending.value = false
    }
  }

  return { pending, redeem }
}
export function useRewardRedemption() {
  const queryClient = useQueryClient()
  return createRewardRedemptionWorkflow({
    redeem: redeemRewardCode,
    setAvailableBalance: (availableBalance) => updateWalletBalance(queryClient, availableBalance),
    prependRedemption: (redemption) => prependRewardRedemption(queryClient, redemption),
    invalidateBillingRecords: () =>
      queryClient.invalidateQueries({ queryKey: billingRecordsQueryKeys.all }),
    invalidateRedemptions: () =>
      queryClient.invalidateQueries({ queryKey: rewardRedemptionsQueryKeys.all }),
  })
}

export function prependRewardRedemption(
  queryClient: QueryClient,
  redemption: RewardRedemptionItem,
): void {
  queryClient.setQueryData<InfiniteData<RewardRedemptionPageData, number>>(
    rewardRedemptionsQueryKeys.all,
    (cached) => {
      if (
        cached?.pages.some((page) => page.redemptions.some((item) => item.id === redemption.id))
      ) {
        return cached
      }
      if (!cached?.pages.length) {
        return {
          pages: [{ redemptions: [redemption] }],
          pageParams: [0],
        }
      }

      return {
        ...cached,
        pages: [
          {
            ...cached.pages[0],
            redemptions: [redemption, ...(cached.pages[0]?.redemptions ?? [])],
          },
          ...cached.pages.slice(1),
        ],
      }
    },
  )
}

function updateWalletBalance(queryClient: QueryClient, availableBalance: number) {
  queryClient.setQueryData<CreditWalletSummary>(creditsQueryKeys.wallet, (wallet) => {
    if (!wallet) return wallet
    return { ...wallet, available_credits: availableBalance }
  })
}
