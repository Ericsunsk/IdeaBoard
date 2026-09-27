import { computed, toValue, type MaybeRefOrGetter } from 'vue'
import { useInfiniteQuery } from '@tanstack/vue-query'
import {
  fetchRewardRedemptions,
  rewardRedemptionsQueryKeys,
} from '../../infrastructure/rewards/resource'
import { useAccountWorkspace } from '../accountWorkspace'
import { PAGE_SIZES } from '../../../../shared/config/pagination'
import { noRetry } from '../../../../shared/query/queryDefaults'

const pageSize = PAGE_SIZES.standard

export function useRewardHistory(enabled: MaybeRefOrGetter<boolean> = true) {
  const { workspaceId, selectionSettled } = useAccountWorkspace()

  const query = useInfiniteQuery({
    queryKey: computed(() => rewardRedemptionsQueryKeys.list(workspaceId.value ?? '')),
    queryFn: ({ pageParam, signal }: { pageParam: string; signal: AbortSignal }) =>
      fetchRewardRedemptions(
        workspaceId.value ?? '',
        {
          page_size: pageSize,
          ...(pageParam ? { cursor: pageParam } : {}),
        },
        signal,
      ),
    initialPageParam: '',
    getNextPageParam: (lastPage) => lastPage.next_cursor,
    enabled: () => toValue(enabled) && selectionSettled.value,
    ...noRetry(),
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
