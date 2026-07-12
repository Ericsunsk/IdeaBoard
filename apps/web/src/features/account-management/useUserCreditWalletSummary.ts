import { computed, toValue, type MaybeRefOrGetter } from 'vue'
import { useQuery } from '@tanstack/vue-query'
import { fetchUserCreditWallet, userCreditsQueryKeys } from './userCreditsResource'
import { useUserCreditsInvalidationBridge } from './useUserCreditsInvalidationBridge'

export function useUserCreditWalletSummary(enabled: MaybeRefOrGetter<boolean> = true) {
  useUserCreditsInvalidationBridge()
  const query = useQuery({
    queryKey: userCreditsQueryKeys.wallet,
    queryFn: fetchUserCreditWallet,
    enabled: () => toValue(enabled),
    staleTime: 15_000,
  })
  return {
    availableCredits: computed(() => query.data.value?.available_credits),
    isPending: query.isPending,
  }
}
