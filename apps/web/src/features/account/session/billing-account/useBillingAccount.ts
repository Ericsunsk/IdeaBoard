import { computed, toValue, type MaybeRefOrGetter } from 'vue'
import { useUserStore } from '../../../auth'
import { useQuery } from '@tanstack/vue-query'
import { billingAccountQueryKeys, fetchBillingAccount } from './resource'
import type { BillingConsumptionReadScope, BillingWalletReadScope } from './readScope'
import { useAccountWorkspace } from '../accountWorkspace'
import { accountScoped, noRetry } from '../../../../core/query/queryDefaults'

// Owns the prerequisite lifecycle and immutable contexts for account reads.
// Callers keep their own pagination, range retention and transport adapters.
export function useBillingAccount(enabled: MaybeRefOrGetter<boolean> = true) {
  const userStore = useUserStore()
  const { workspaceId, isSelectionSettled } = useAccountWorkspace({ enabled })
  const identity = computed(() => userStore.user?.public_id ?? '')
  const query = useQuery({
    queryKey: computed(() =>
      billingAccountQueryKeys.detail(workspaceId.value ?? '', identity.value),
    ),
    queryFn: async ({ queryKey, signal }) => ({
      ...(await fetchBillingAccount(queryKey[3], signal)),
      identity: queryKey[4],
    }),
    enabled: () => toValue(enabled) && isSelectionSettled.value,
    ...accountScoped(),
    ...noRetry(),
  })
  const isReady = computed(
    () =>
      toValue(enabled) &&
      isSelectionSettled.value &&
      !query.isError.value &&
      query.data.value != null &&
      query.data.value.workspaceId === workspaceId.value &&
      query.data.value.identity === identity.value,
  )
  const walletScope = computed<BillingWalletReadScope>(() => ({
    workspaceId: workspaceId.value ?? '',
    billingAccountId: query.data.value?.billingAccountId ?? '',
    identity: identity.value,
  }))
  const readScope = computed<BillingConsumptionReadScope>(() => ({
    ...walletScope.value,
    consumptionScope: query.data.value?.consumptionScope,
  }))

  return {
    billingAccountId: computed(() => query.data.value?.billingAccountId),
    consumptionScope: computed(() => query.data.value?.consumptionScope),
    isShared: computed(() => query.data.value?.isShared === true),
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
