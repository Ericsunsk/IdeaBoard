import { ref } from 'vue'
import { i18n } from '../../shared/i18n'
import { useQueryClient, type InfiniteData, type QueryClient } from '@tanstack/vue-query'
import { userBillingRecordsQueryKeys } from './userBillingRecordsResource'
import {
  redeemUserRewardCode,
  userRewardRedemptionsQueryKeys,
  type UserRewardRedemption,
  type UserRewardRedemptionPage,
  type UserRewardRedeemResult,
} from './userRewardsResource'
import { userCreditsQueryKeys, type UserCreditWallet } from './userCreditsResource'

type UserRewardRedemptionDependencies = {
  redeem: (code: string) => Promise<UserRewardRedeemResult>
  setAvailableBalance: (availableBalance: number) => void
  prependRedemption: (redemption: UserRewardRedemption) => void
  invalidateBillingRecords: () => Promise<unknown>
  invalidateRedemptions: () => Promise<unknown>
}

export function createUserRewardRedemptionWorkflow(dependencies: UserRewardRedemptionDependencies) {
  const pending = ref(false)

  async function redeem(code: string): Promise<UserRewardRedeemResult> {
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

export function useUserRewardRedemption() {
  const queryClient = useQueryClient()
  return createUserRewardRedemptionWorkflow({
    redeem: redeemUserRewardCode,
    setAvailableBalance: (availableBalance) => updateWalletBalance(queryClient, availableBalance),
    prependRedemption: (redemption) => prependUserRewardRedemption(queryClient, redemption),
    invalidateBillingRecords: () =>
      queryClient.invalidateQueries({ queryKey: userBillingRecordsQueryKeys.all }),
    invalidateRedemptions: () =>
      queryClient.invalidateQueries({ queryKey: userRewardRedemptionsQueryKeys.all }),
  })
}

export function prependUserRewardRedemption(
  queryClient: QueryClient,
  redemption: UserRewardRedemption,
): void {
  queryClient.setQueryData<InfiniteData<UserRewardRedemptionPage, number>>(
    userRewardRedemptionsQueryKeys.all,
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
            redemptions: [redemption, ...cached.pages[0].redemptions],
          },
          ...cached.pages.slice(1),
        ],
      }
    },
  )
}

function updateWalletBalance(queryClient: QueryClient, availableBalance: number) {
  queryClient.setQueryData<UserCreditWallet>(userCreditsQueryKeys.wallet, (wallet) => {
    if (!wallet) return wallet
    return { ...wallet, available_credits: availableBalance }
  })
}
