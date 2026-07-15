import { ref } from 'vue'
import { useQueryClient, type QueryClient } from '@tanstack/vue-query'
import { userBillingRecordsQueryKeys } from './userBillingRecordsResource'
import {
  redeemUserRewardCode,
  type UserRewardRedeemResult,
} from './userRewardsResource'
import { userCreditsQueryKeys, type UserCreditWallet } from './userCreditsResource'

type UserRewardRedemptionDependencies = {
  redeem: (code: string) => Promise<UserRewardRedeemResult>
  setAvailableBalance: (availableBalance: number) => void
  invalidateBillingRecords: () => Promise<unknown>
}

export function createUserRewardRedemptionWorkflow(
  dependencies: UserRewardRedemptionDependencies,
) {
  const pending = ref(false)

  async function redeem(code: string): Promise<UserRewardRedeemResult> {
    if (pending.value) throw new Error('兑换请求正在处理中')
    pending.value = true
    try {
      const result = await dependencies.redeem(code.trim())
      dependencies.setAvailableBalance(result.available_balance)
      await dependencies.invalidateBillingRecords()
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
    setAvailableBalance: (availableBalance) =>
      updateWalletBalance(queryClient, availableBalance),
    invalidateBillingRecords: () =>
      queryClient.invalidateQueries({ queryKey: userBillingRecordsQueryKeys.all }),
  })
}

function updateWalletBalance(queryClient: QueryClient, availableBalance: number) {
  queryClient.setQueryData<UserCreditWallet>(userCreditsQueryKeys.wallet, (wallet) => {
    if (!wallet) return wallet
    return { ...wallet, available_credits: availableBalance }
  })
}
