import { computed, toValue, type MaybeRefOrGetter } from 'vue'
import { useQuery } from '@tanstack/vue-query'
import { fetchUserCreditWallet, userCreditsQueryKeys } from './userCreditsResource'
import { useUserCreditsInvalidationBridge } from './useUserCreditsInvalidationBridge'

export type UserCreditWalletSummaryState =
  | { status: 'loading' }
  | { status: 'ready'; availableCredits: number }
  | { status: 'error'; error: Error }

export function useUserCreditWalletSummary(enabled: MaybeRefOrGetter<boolean> = true) {
  useUserCreditsInvalidationBridge()
  const query = useQuery({
    queryKey: userCreditsQueryKeys.wallet,
    queryFn: fetchUserCreditWallet,
    enabled: () => toValue(enabled),
    staleTime: 15_000,
  })
  const state = computed<UserCreditWalletSummaryState>(() => {
    if (query.isError.value) {
      return {
        status: 'error',
        error: query.error.value ?? new Error('Idea余额加载失败'),
      }
    }
    const availableCredits = query.data.value?.available_credits
    if (query.isPending.value || availableCredits == null) {
      return { status: 'loading' }
    }
    return { status: 'ready', availableCredits }
  })

  return { state }
}
