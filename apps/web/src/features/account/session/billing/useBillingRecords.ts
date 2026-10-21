import { computed, type MaybeRefOrGetter } from 'vue'
import { useInfiniteQuery } from '@tanstack/vue-query'
import { useCreditsSync } from '../credits/useCreditsSync'
import { billingRecordsQueryKeys, fetchBillingRecords } from './resource'
import { PAGE_SIZES } from '../../../../core/config/pagination'
import { noRetry } from '../../../../core/query/queryDefaults'
import { useBillingAccount } from '../billing-account/useBillingAccount'

const pageSize = PAGE_SIZES.standard

export function useBillingRecords(enabled: MaybeRefOrGetter<boolean> = true) {
  useCreditsSync()
  const billing = useBillingAccount(enabled)

  const query = useInfiniteQuery({
    queryKey: computed(() => billingRecordsQueryKeys.list(billing.readScope.value)),
    queryFn: ({ queryKey, pageParam, signal }) =>
      fetchBillingRecords(
        queryKey[3].workspaceId,
        {
          page_size: pageSize,
          ...(pageParam ? { cursor: pageParam } : {}),
        },
        signal,
      ),
    initialPageParam: '',
    getNextPageParam: (lastPage) => lastPage.next_cursor,
    enabled: billing.isReady,
    ...noRetry(),
  })

  const records = computed(() =>
    billing.isReady.value ? (query.data.value?.pages.flatMap((page) => page.records) ?? []) : [],
  )
  const hasLoadedPage = computed(() => query.data.value !== undefined)
  const initialError = billing.readError(() => (!hasLoadedPage.value ? query.error.value : null))

  // jscpd:ignore-start -- billing and reward pagination expose distinct domain-shaped results.
  return {
    billingAccountId: billing.billingAccountId,
    consumptionScope: billing.consumptionScope,
    isShared: billing.isShared,
    records,
    isInitialPending: computed(
      () => !initialError.value && query.isPending.value && !hasLoadedPage.value,
    ),
    initialError,
    loadMoreError: computed(() => (query.isFetchNextPageError.value ? query.error.value : null)),
    hasNextPage: query.hasNextPage,
    isFetchingNextPage: query.isFetchingNextPage,
    retryInitial: () => billing.retryRead(query.refetch),
    loadMore: query.fetchNextPage,
  }
  // jscpd:ignore-end
}
