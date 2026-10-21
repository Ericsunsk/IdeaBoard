import { computed, shallowRef, toValue, watch, type MaybeRefOrGetter } from 'vue'
import { useQuery } from '@tanstack/vue-query'
import { fetchUsage, usageQueryKeys, type AccountUsage, type UsageRange } from './resource'
import type { BillingConsumptionReadScope } from '../billing-account/readScope'
import { useBillingAccount } from '../billing-account/useBillingAccount'
import { accountScoped, noRetry, STALE_TIME_MS } from '../../../../core/query/queryDefaults'

type SuccessfulUsage = {
  scope: BillingConsumptionReadScope
  range: UsageRange
  usage: AccountUsage
}

export function useUsage(
  range: MaybeRefOrGetter<UsageRange>,
  enabled: MaybeRefOrGetter<boolean> = true,
) {
  const selectedRange = computed(() => toValue(range))
  const billing = useBillingAccount(enabled)
  const lastSuccessful = shallowRef<SuccessfulUsage>()
  const query = useQuery({
    queryKey: computed(() => usageQueryKeys.current(billing.readScope.value, selectedRange.value)),
    queryFn: async ({ queryKey, signal }): Promise<SuccessfulUsage> => {
      // Read scope from the key, never from the closure: a late response
      // from a previous workspace or account must not replace the current
      // selection.
      const scope = queryKey[3]
      const requestRange = queryKey[4]
      return {
        scope,
        range: requestRange,
        usage: await fetchUsage(scope.workspaceId, requestRange, signal),
      }
    },
    enabled: billing.isReady,
    ...accountScoped(STALE_TIME_MS.usage),
    refetchOnWindowFocus: true,
    ...noRetry(),
  })

  watch(
    query.data,
    (result) => {
      if (result) lastSuccessful.value = result
    },
    { immediate: true, flush: 'sync' },
  )
  // Retain successful ranges only inside the selected funds/workspace scope.
  // A pending or failed switch must never display the previous account.
  const currentSuccessful = computed(() => {
    const result = lastSuccessful.value
    return billing.isCurrentScope(result?.scope) ? result : undefined
  })

  const error = billing.readError(query.error)

  return {
    billingAccountId: billing.billingAccountId,
    isShared: billing.isShared,
    usage: computed(() => currentSuccessful.value?.usage),
    displayRange: computed(() => currentSuccessful.value?.range),
    isInitialPending: computed(
      () => !error.value && !currentSuccessful.value && query.isPending.value,
    ),
    isSwitching: computed(
      () =>
        !!currentSuccessful.value &&
        query.isFetching.value &&
        currentSuccessful.value.range !== selectedRange.value,
    ),
    error,
    retry: () => billing.retryRead(query.refetch),
  }
}
