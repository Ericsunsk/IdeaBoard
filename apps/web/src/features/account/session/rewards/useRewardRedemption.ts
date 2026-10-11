import { ref } from 'vue'
import { i18n } from '../../../../core/i18n'
import { useQueryClient, type InfiniteData, type QueryClient } from '@tanstack/vue-query'
import { invalidateUserMoney } from '../invalidateUserMoney'
import { useAccountWorkspace } from '../accountWorkspace'
import { billingRecordsQueryKeys } from '../billing/resource'
import {
  redeemRewardCode,
  rewardRedemptionsQueryKeys,
  type RewardRedeemResult,
  type RewardRedemptionItem,
  type RewardRedemptionPageData,
} from './resource'

export type RewardRedemptionDependencies = {
  redeem: (code: string) => Promise<RewardRedeemResult>
  setAvailableBalance: (availableBalance: number) => void
  prependRedemption: (redemption: RewardRedemptionItem) => void
  invalidateBillingRecords: () => Promise<unknown>
  invalidateRedemptions: () => Promise<unknown>
}

export function createRewardRedemptionWorkflow(dependencies: RewardRedemptionDependencies) {
  // Ad-hoc reentrancy guard (not `useAsyncAction`): redemption must throw on
  // concurrent calls instead of counting them.
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
  const { workspaceId } = useAccountWorkspace()
  return createRewardRedemptionWorkflow({
    redeem: (code) => redeemRewardCode(workspaceId.value ?? '', code),
    setAvailableBalance: () => {
      // Wallet keys are workspace-scoped now; redemption invalidates the
      // whole wallet root instead of patching one entry.
      invalidateUserMoney(queryClient)
    },
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
  queryClient.setQueryData<InfiniteData<RewardRedemptionPageData, string>>(
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
          pageParams: [''],
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
