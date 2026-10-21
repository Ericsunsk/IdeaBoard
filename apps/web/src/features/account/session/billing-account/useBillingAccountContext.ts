import { computed, toValue, type MaybeRefOrGetter } from 'vue'
import { useQuery } from '@tanstack/vue-query'
import { billingAccountQueryKeys, fetchBillingAccount } from './resource'
import type { BillingConsumptionReadScope, BillingWalletReadScope } from './readScope'
import { accountScoped, noRetry } from '../../../../core/query/queryDefaults'

// Owns account resolution and readiness for one caller-selected target.
// Every reader uses the same cache contract; selection remains caller-owned.
export function useBillingAccountContext(options: {
  workspaceId: MaybeRefOrGetter<string | null | undefined>
  identity: MaybeRefOrGetter<string | null | undefined>
  isEnabled?: MaybeRefOrGetter<boolean>
}) {
  const workspaceId = computed(() => toValue(options.workspaceId) ?? '')
  const identity = computed(() => toValue(options.identity) ?? '')
  const isEnabled = computed(() => toValue(options.isEnabled) !== false && identity.value !== '')
  const query = useQuery({
    queryKey: computed(() => billingAccountQueryKeys.detail(workspaceId.value, identity.value)),
    queryFn: async ({ queryKey, signal }) => ({
      ...(await fetchBillingAccount(queryKey[3], signal)),
      identity: queryKey[4],
    }),
    enabled: isEnabled,
    ...accountScoped(),
    ...noRetry(),
  })
  const isReady = computed(
    () =>
      isEnabled.value &&
      !query.isError.value &&
      query.data.value != null &&
      query.data.value.workspaceId === workspaceId.value &&
      query.data.value.identity === identity.value,
  )
  const account = computed(() => (isReady.value ? query.data.value : null))
  const walletScope = computed<BillingWalletReadScope>(() => ({
    workspaceId: workspaceId.value,
    billingAccountId: account.value?.billingAccountId ?? '',
    identity: identity.value,
  }))
  const readScope = computed<BillingConsumptionReadScope>(() => ({
    ...walletScope.value,
    consumptionScope: account.value?.consumptionScope,
  }))

  return {
    account,
    error: query.error,
    isLoading: query.isLoading,
    retry: () => query.refetch(),
    billingAccountId: computed(() => account.value?.billingAccountId),
    consumptionScope: computed(() => account.value?.consumptionScope),
    isShared: computed(() => account.value?.isShared === true),
    walletScope,
    readScope,
    isReady,
    readError: (error: MaybeRefOrGetter<Error | null>) =>
      computed(() => query.error.value ?? toValue(error)),
    retryRead: (refetch: () => unknown) => (isReady.value ? refetch() : query.refetch()),
    isCurrentScope: (scope: BillingConsumptionReadScope | undefined) =>
      isReady.value &&
      scope?.workspaceId === readScope.value.workspaceId &&
      scope.billingAccountId === readScope.value.billingAccountId &&
      scope.identity === readScope.value.identity &&
      scope.consumptionScope === readScope.value.consumptionScope,
  }
}
