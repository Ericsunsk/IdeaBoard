import { computed, shallowRef, toValue, watch, type MaybeRefOrGetter } from 'vue'
import { useQuery } from '@tanstack/vue-query'
import {
  fetchUsage,
  usageQueryKeys,
  type AccountUsage,
  type UsageRange,
} from '../../infrastructure/usage/resource'
import { useAccountWorkspace } from '../accountWorkspace'
import {
  accountScoped,
  keepPrevious,
  noRetry,
  STALE_TIME_MS,
} from '../../../../core/query/queryDefaults'

type SuccessfulUsage = {
  range: UsageRange
  usage: AccountUsage
}

export function useUsage(
  range: MaybeRefOrGetter<UsageRange>,
  enabled: MaybeRefOrGetter<boolean> = true,
) {
  const selectedRange = computed(() => toValue(range))
  const { workspaceId, selectionSettled } = useAccountWorkspace()
  const lastSuccessful = shallowRef<SuccessfulUsage>()
  const query = useQuery({
    queryKey: computed(() => usageQueryKeys.current(workspaceId.value ?? '', selectedRange.value)),
    queryFn: async ({ queryKey, signal }): Promise<SuccessfulUsage> => {
      const requestWorkspaceId = queryKey[3] as string
      const requestRange = queryKey[4] as UsageRange
      return {
        range: requestRange,
        usage: await fetchUsage(requestWorkspaceId, requestRange, signal),
      }
    },
    enabled: () => toValue(enabled) && selectionSettled.value,
    ...accountScoped(STALE_TIME_MS.usage),
    refetchOnWindowFocus: true,
    ...noRetry(),
    ...keepPrevious(),
  })

  watch(
    [query.data, query.isPlaceholderData],
    ([result, isPlaceholderData]) => {
      if (result && !isPlaceholderData) {
        lastSuccessful.value = result
      }
    },
    { immediate: true, flush: 'sync' },
  )

  return {
    usage: computed(() => lastSuccessful.value?.usage),
    displayRange: computed(() => lastSuccessful.value?.range),
    isInitialPending: computed(() => !lastSuccessful.value && query.isPending.value),
    isSwitching: computed(
      () =>
        !!lastSuccessful.value &&
        query.isFetching.value &&
        lastSuccessful.value.range !== selectedRange.value,
    ),
    error: query.error,
    retry: query.refetch,
  }
}
