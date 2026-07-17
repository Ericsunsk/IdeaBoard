import { computed, toValue, type MaybeRefOrGetter } from 'vue'
import { useMutation, useQuery, useQueryClient } from '@tanstack/vue-query'
import type { CreditReconciliationRequest } from '../../../../shared/api/generated-query/model'
import { adminCreditsBillingApi, type AdminCreditsBillingApi } from '../creditsBillingApi'

export const adminCreditHealthQueryKey = ['admin', 'credits', 'health'] as const

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
    refetchInterval: (query) => (query.state.data?.active_run ? 5_000 : 30_000),
  })
  const startMutation = useMutation({
    mutationFn: (request: CreditReconciliationRequest) => api.startReconciliation(request),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: adminCreditHealthQueryKey })
    },
  })

  return {
    query,
    health: computed(() => query.data.value),
    startPending: computed(() => startMutation.isPending.value),
    start: (request: CreditReconciliationRequest) => startMutation.mutateAsync(request),
  }
}
