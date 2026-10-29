import { computed, type MaybeRefOrGetter } from 'vue'
import { useCreditsSync } from '../credits/useCreditsSync'
import { billingRecordsQueryKeys, fetchBillingRecords } from './resource'
import { useCursorInfiniteList } from '../../../../core/query/useCursorInfiniteList'
import { useBillingAccount } from '../billing-account/useBillingAccount'

export function useBillingRecords(enabled: MaybeRefOrGetter<boolean> = true) {
  useCreditsSync()
  const billing = useBillingAccount(enabled)

  const { query, items, hasLoadedPage, loadMoreError, hasNextPage, isFetchingNextPage, loadMore } =
    useCursorInfiniteList({
      queryKey: computed(() =>
        billingRecordsQueryKeys.list(
          billing.readScope.value.workspaceId,
          billing.readScope.value.billingAccountId,
          billing.readScope.value.identity,
          billing.readScope.value.consumptionScope ?? '',
        ),
      ),
      fetchPage: ({ queryKey, pageParam, signal, pageSize }) =>
        fetchBillingRecords(
          queryKey[3] as string,
          {
            page_size: pageSize,
            ...(pageParam ? { cursor: pageParam } : {}),
          },
          signal,
        ),
      isEnabled: billing.isReady,
      getItems: (page) => page.records,
    })

  const records = computed(() => (billing.isReady.value ? [...items.value] : []))
  const initialError = billing.readError(() => (!hasLoadedPage.value ? query.error.value : null))

  return {
    billingAccountId: billing.billingAccountId,
    consumptionScope: billing.consumptionScope,
    isShared: billing.isShared,
    records,
    isInitialPending: computed(
      () => !initialError.value && query.isPending.value && !hasLoadedPage.value,
    ),
    initialError,
    loadMoreError,
    hasNextPage,
    isFetchingNextPage,
    retryInitial: () => billing.retryRead(query.refetch),
    loadMore,
  }
}
