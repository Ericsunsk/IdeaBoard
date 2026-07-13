import { computed, toValue, type MaybeRefOrGetter } from 'vue'
import { useQuery } from '@tanstack/vue-query'
import { fetchUserUsage, userUsageQueryKeys } from './userUsageResource'

export function useUserUsage(enabled: MaybeRefOrGetter<boolean> = true) {
  const query = useQuery({
    queryKey: userUsageQueryKeys.current,
    queryFn: fetchUserUsage,
    enabled: () => toValue(enabled),
    retry: false,
  })

  return {
    usage: computed(() => query.data.value),
    isPending: query.isPending,
    error: query.error,
    refresh: query.refetch,
  }
}
