import { computed, toValue, type MaybeRefOrGetter } from 'vue'
import { useInfiniteQuery, useQuery, useQueryClient } from '@tanstack/vue-query'
import {
  fetchUserCreditLedger,
  fetchUserCreditWallet,
  userCreditsQueryKeys,
} from './userCreditsResource'
import { useUserCreditsInvalidationBridge } from './useUserCreditsInvalidationBridge'

const ledgerPageSize = 20
const initialLedgerPageParam = 0

export function useUserCredits(enabled: MaybeRefOrGetter<boolean> = true) {
  const queryClient = useQueryClient()
  useUserCreditsInvalidationBridge()
  const walletQuery = useQuery({
    queryKey: userCreditsQueryKeys.wallet,
    queryFn: fetchUserCreditWallet,
    enabled: () => toValue(enabled),
  })
  const ledgerQuery = useInfiniteQuery({
    queryKey: userCreditsQueryKeys.ledger,
    queryFn: ({ pageParam }: { pageParam: number }) =>
      fetchUserCreditLedger({
        limit: ledgerPageSize,
        ...(pageParam > 0 ? { before_sequence: pageParam } : {}),
      }),
    initialPageParam: initialLedgerPageParam,
    getNextPageParam: (lastPage) => lastPage.next_before_sequence ?? undefined,
    enabled: () => toValue(enabled),
  })

  const refresh = () => queryClient.invalidateQueries({ queryKey: userCreditsQueryKeys.all })

  return {
    wallet: computed(() => walletQuery.data.value),
    transactions: computed(
      () => ledgerQuery.data.value?.pages.flatMap((page) => page.transactions) ?? [],
    ),
    walletPending: walletQuery.isPending,
    walletError: walletQuery.error,
    ledgerPending: ledgerQuery.isPending,
    ledgerError: ledgerQuery.error,
    hasNextPage: ledgerQuery.hasNextPage,
    isFetchingNextPage: ledgerQuery.isFetchingNextPage,
    fetchNextPage: ledgerQuery.fetchNextPage,
    refresh,
  }
}
