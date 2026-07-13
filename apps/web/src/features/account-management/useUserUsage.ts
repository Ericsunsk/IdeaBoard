import { computed, shallowRef, toValue, watch, type MaybeRefOrGetter } from 'vue'
import { useQuery } from '@tanstack/vue-query'
import {
  fetchUserUsage,
  userUsageQueryKeys,
  type AccountUsage,
  type UserUsageRange,
} from './userUsageResource'

const USER_USAGE_STALE_TIME_MS = 30_000

type SuccessfulUsage = {
  range: UserUsageRange
  usage: AccountUsage
}

export function useUserUsage(
  range: MaybeRefOrGetter<UserUsageRange>,
  enabled: MaybeRefOrGetter<boolean> = true,
) {
  const selectedRange = computed(() => toValue(range))
  const lastSuccessful = shallowRef<SuccessfulUsage>()
  const query = useQuery({
    queryKey: computed(() => userUsageQueryKeys.current(selectedRange.value)),
    queryFn: async ({ queryKey, signal }): Promise<SuccessfulUsage> => {
      const requestRange = queryKey[3] as UserUsageRange
      return {
        range: requestRange,
        usage: await fetchUserUsage(requestRange, signal),
      }
    },
    enabled: () => toValue(enabled),
    staleTime: USER_USAGE_STALE_TIME_MS,
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
