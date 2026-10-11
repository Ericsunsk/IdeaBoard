import { computed, toValue, type MaybeRefOrGetter } from 'vue'
import { useInfiniteQuery } from '@tanstack/vue-query'
import { useCreditsSync } from '../credits/useCreditsSync'
import { billingRecordsQueryKeys, fetchBillingRecords } from './resource'
import { PAGE_SIZES } from '../../../../core/config/pagination'
import { noRetry } from '../../../../core/query/queryDefaults'
import { useAccountWorkspace } from '../accountWorkspace'

const pageSize = PAGE_SIZES.standard

export function useBillingRecords(enabled: MaybeRefOrGetter<boolean> = true) {
  useCreditsSync()
  const { workspaceId, selectionSettled } = useAccountWorkspace()

  const query = useInfiniteQuery({
    queryKey: computed(() => billingRecordsQueryKeys.list(workspaceId.value ?? '')),
    queryFn: ({ pageParam, signal }: { pageParam: string; signal: AbortSignal }) =>
      fetchBillingRecords(
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
