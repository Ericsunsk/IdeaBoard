import { computed, shallowRef, toValue, watch, type MaybeRefOrGetter } from 'vue'
import { useQuery } from '@tanstack/vue-query'
import {
  fetchUsage,
  usageQueryKeys,
  type AccountUsage,
  type UsageRange,
} from '../../infrastructure/usage/usageResource'

const USAGE_STALE_TIME_MS = 30_000

type SuccessfulUsage = {
  range: UsageRange
  usage: AccountUsage
}

export function useUsage(
  range: MaybeRefOrGetter<UsageRange>,
  enabled: MaybeRefOrGetter<boolean> = true,
) {
  const selectedRange = computed(() => toValue(range))
  const lastSuccessful = shallowRef<SuccessfulUsage>()
  const query = useQuery({
    queryKey: computed(() => usageQueryKeys.current(selectedRange.value)),
    queryFn: async ({ queryKey, signal }): Promise<SuccessfulUsage> => {
      const requestRange = queryKey[3] as UsageRange
      return {
        range: requestRange,
        usage: await fetchUsage(requestRange, signal),
      }
    },
    enabled: () => toValue(enabled),
    staleTime: USAGE_STALE_TIME_MS,
    refetchOnWindowFocus: true,
    retry: false,
    placeholderData: (previousData) => previousData,
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
