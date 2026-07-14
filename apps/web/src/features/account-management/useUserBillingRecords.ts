import { computed, toValue, type MaybeRefOrGetter } from 'vue'
import { useInfiniteQuery } from '@tanstack/vue-query'
import { useUserCreditsInvalidationBridge } from './useUserCreditsInvalidationBridge'
import {
  fetchUserBillingRecords,
  userBillingRecordsQueryKeys,
} from './userBillingRecordsResource'

const pageSize = 20
const initialPageParam = 0

export function useUserBillingRecords(enabled: MaybeRefOrGetter<boolean> = true) {
  useUserCreditsInvalidationBridge()

  const query = useInfiniteQuery({
    queryKey: userBillingRecordsQueryKeys.all,
    queryFn: ({ pageParam, signal }: { pageParam: number; signal: AbortSignal }) =>
      fetchUserBillingRecords(
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

  const records = computed(
    () => query.data.value?.pages.flatMap((page) => page.records) ?? [],
  )
  const hasLoadedPage = computed(() => query.data.value !== undefined)

  return {
    records,
    isInitialPending: computed(() => query.isPending.value && !hasLoadedPage.value),
    initialError: computed(() => (!hasLoadedPage.value ? query.error.value : null)),
    loadMoreError: computed(() =>
      query.isFetchNextPageError.value ? query.error.value : null,
    ),
    hasNextPage: query.hasNextPage,
    isFetchingNextPage: query.isFetchingNextPage,
    retryInitial: query.refetch,
    loadMore: query.fetchNextPage,
  }
}
