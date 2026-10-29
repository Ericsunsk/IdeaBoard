import { computed, toValue, type MaybeRefOrGetter } from 'vue'
import { fetchRewardRedemptions, rewardRedemptionsQueryKeys } from './resource'
import { useAccountWorkspace } from '../accountWorkspace'
import { useCursorInfiniteList } from '../../../../core/query/useCursorInfiniteList'

export function useRewardHistory(enabled: MaybeRefOrGetter<boolean> = true) {
  const { workspaceId, isSelectionSettled } = useAccountWorkspace()

  const {
    query,
    items: redemptions,
    hasLoadedPage,
    loadMoreError,
    hasNextPage,
    isFetchingNextPage,
    retryInitial,
    loadMore,
  } = useCursorInfiniteList({
    queryKey: computed(() => rewardRedemptionsQueryKeys.list(workspaceId.value ?? '')),
    fetchPage: ({ pageParam, signal, pageSize }) =>
      fetchRewardRedemptions(
        workspaceId.value ?? '',
        {
          page_size: pageSize,
          ...(pageParam ? { cursor: pageParam } : {}),
        },
        signal,
      ),
    isEnabled: () => toValue(enabled) && isSelectionSettled.value,
    getItems: (page) => page.redemptions,
  })

  return {
    redemptions,
    isInitialPending: computed(() => query.isPending.value && !hasLoadedPage.value),
    initialError: computed(() => (!hasLoadedPage.value ? query.error.value : null)),
    loadMoreError,
    hasNextPage,
    isFetchingNextPage,
    retryInitial,
    loadMore,
  }
}
