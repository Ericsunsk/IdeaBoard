import { computed, toValue, type MaybeRefOrGetter } from 'vue'
import { useMutation, useQuery, useQueryClient } from '@tanstack/vue-query'
import type {
  CreditHealth,
  CreditReconciliationRequest,
} from '../shared/api'
import { adminCreditsBillingApi, type AdminCreditsBillingApi } from '../shared/api'
import { adminPolling } from '../../../../shared/query/queryDefaults'

const adminCreditHealthQueryKey = ['admin', 'credits', 'health'] as const

export function useAdminCreditHealthResource(
  options: { enabled?: MaybeRefOrGetter<boolean> } = {},
  dependencies: { api?: AdminCreditsBillingApi } = {},
) {
  const api = dependencies.api ?? adminCreditsBillingApi
  const queryClient = useQueryClient()
  const enabled = computed(() => (options.enabled == null ? true : toValue(options.enabled)))
  const query = useQuery({
    queryKey: adminCreditHealthQueryKey,
    queryFn: ({ signal }) => api.getCreditHealth({ signal }),
    enabled,
    // Interval only: the global retry policy stays (a failed poll still
    // retries once); only the active/idle cadence converges on the preset.
    refetchInterval: adminPolling(
      5_000,
      30_000,
      (data) => (data as CreditHealth | undefined)?.active_run != null,
    ).refetchInterval,
  })
  const startMutation = useMutation({
    mutationFn: (request: CreditReconciliationRequest) => api.startReconciliation(request),
    onSuccess: (run) => {
      queryClient.setQueryData<CreditHealth>(adminCreditHealthQueryKey, (current) =>
        current ? { ...current, active_run: run } : current,
      )
      void queryClient.invalidateQueries({ queryKey: adminCreditHealthQueryKey })
    },
  })

  return {
    query,
    health: computed(() => query.data.value),
    startPending: computed(() => startMutation.isPending.value),
    start: (request: CreditReconciliationRequest) => startMutation.mutateAsync(request),
  }
}
