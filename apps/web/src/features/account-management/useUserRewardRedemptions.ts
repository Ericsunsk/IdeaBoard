import { computed, toValue, type MaybeRefOrGetter } from 'vue'
import { useInfiniteQuery } from '@tanstack/vue-query'
import { fetchUserRewardRedemptions, userRewardRedemptionsQueryKeys } from './userRewardsResource'

const pageSize = 20
const initialPageParam = 0

export function useUserRewardRedemptions(enabled: MaybeRefOrGetter<boolean> = true) {
  const query = useInfiniteQuery({
    queryKey: userRewardRedemptionsQueryKeys.all,
    queryFn: ({ pageParam, signal }: { pageParam: number; signal: AbortSignal }) =>
      fetchUserRewardRedemptions(
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
}
