import { computed, toValue, type MaybeRefOrGetter } from 'vue'
import { useInfiniteQuery } from '@tanstack/vue-query'
import { useCreditsSync } from '../recharge/useCreditsSync'
import {
  billingRecordsQueryKeys,
  fetchBillingRecords,
} from '../../infrastructure/billing/billingResource'

const pageSize = 20
const initialPageParam = 0

export function useBillingRecords(enabled: MaybeRefOrGetter<boolean> = true) {
  useCreditsSync()

  const query = useInfiniteQuery({
    queryKey: billingRecordsQueryKeys.all,
    queryFn: ({ pageParam, signal }: { pageParam: number; signal: AbortSignal }) =>
      fetchBillingRecords(
        {
          limit: pageSize,
          ...(pageParam > 0 ? { before_sequence: pageParam } : {}),
        },
        signal,
      ),
    initialPageParam,
    getNextPageParam: (lastPage) => lastPage.next_before_sequence ?? undefined,
    enabled: () => toValue(enabled),
    retry: false,
  })

  const records = computed(() => query.data.value?.pages.flatMap((page) => page.records) ?? [])
  const hasLoadedPage = computed(() => query.data.value !== undefined)

  // jscpd:ignore-start -- billing and reward pagination expose distinct domain-shaped results.
  return {
    records,
    isInitialPending: computed(() => query.isPending.value && !hasLoadedPage.value),
    initialError: computed(() => (!hasLoadedPage.value ? query.error.value : null)),
    loadMoreError: computed(() => (query.isFetchNextPageError.value ? query.error.value : null)),
    hasNextPage: query.hasNextPage,
    isFetchingNextPage: query.isFetchingNextPage,
    retryInitial: query.refetch,
    loadMore: query.fetchNextPage,
  }
  // jscpd:ignore-end
}
