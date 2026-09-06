import { computed, toValue, type MaybeRefOrGetter } from 'vue'
import { useInfiniteQuery } from '@tanstack/vue-query'
import {
  fetchRewardRedemptions,
  rewardRedemptionsQueryKeys,
} from '../../infrastructure/rewards/rewardsResource'

const pageSize = 20
const initialPageParam = 0

export function useRewardHistory(enabled: MaybeRefOrGetter<boolean> = true) {
  const query = useInfiniteQuery({
    queryKey: rewardRedemptionsQueryKeys.all,
    queryFn: ({ pageParam, signal }: { pageParam: number; signal: AbortSignal }) =>
      fetchRewardRedemptions(
        {
          limit: pageSize,
          ...(pageParam > 0 ? { before_id: pageParam } : {}),
        },
        signal,
      ),
    initialPageParam,
    getNextPageParam: (lastPage) => lastPage.next_before_id ?? undefined,
    enabled: () => toValue(enabled),
    retry: false,
  })

  const redemptions = computed(
    () => query.data.value?.pages.flatMap((page) => page.redemptions) ?? [],
  )
  const hasLoadedPage = computed(() => query.data.value !== undefined)

  // jscpd:ignore-start -- billing and reward pagination expose distinct domain-shaped results.
  return {
    redemptions,
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
