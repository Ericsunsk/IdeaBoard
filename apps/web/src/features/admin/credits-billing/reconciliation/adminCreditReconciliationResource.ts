import { computed, toValue, type MaybeRefOrGetter } from 'vue'
import { useMutation, useQuery, useQueryClient } from '@tanstack/vue-query'
import type {
  CreditReconciliationRequest,
  CreditReconciliationRun,
  ListAdminCreditReconciliationRunsParams,
} from '../../../../shared/api/generated-query/model'
import { adminCreditsBillingApi, type AdminCreditsBillingApi } from '../creditsBillingApi'

const reconciliationQueryRoot = ['admin', 'credits', 'reconciliation'] as const

export type AdminCreditReconciliationRun = CreditReconciliationRun

interface ReconciliationRunsCriteria {
  beforeId?: number
}

export function useAdminCreditReconciliationResource(
  input: {
    runBeforeId: MaybeRefOrGetter<number | undefined>
    selectedRunId: MaybeRefOrGetter<number | null>
  },
  dependencies: { api?: AdminCreditsBillingApi } = {},
) {
  const api = dependencies.api ?? adminCreditsBillingApi
  const queryClient = useQueryClient()
  const normalizedRunId = computed(() => toValue(input.selectedRunId) ?? 0)
  const runsCriteria = computed<ReconciliationRunsCriteria>(() => ({
    beforeId: toValue(input.runBeforeId),
  }))
  const runsQuery = useQuery({
    queryKey: computed(() => [...reconciliationQueryRoot, 'runs', runsCriteria.value]),
    queryFn: async ({ queryKey, signal }) => {
      const criteria = queryKey.at(-1) as ReconciliationRunsCriteria
      const params: ListAdminCreditReconciliationRunsParams = {
        before_id: criteria.beforeId,
        limit: 20,
      }
      return {
        criteria,
        page: await api.listReconciliationRuns(params, { signal }),
      }
    },
    placeholderData: (previous) => previous,
    refetchInterval: 5_000,
  })
  const runQuery = useQuery({
    queryKey: computed(() => [...reconciliationQueryRoot, 'runs', normalizedRunId.value]),
    queryFn: ({ signal }) => api.getReconciliationRun(normalizedRunId.value, { signal }),
    enabled: computed(() => normalizedRunId.value > 0),
    refetchInterval: (query) => {
      const status = query.state.data?.status
      return status === 'queued' || status === 'running' ? 3_000 : false
    },
  })
  const startMutation = useMutation({
    mutationFn: (request: CreditReconciliationRequest) => api.startReconciliation(request),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: reconciliationQueryRoot }),
  })

  return {
    runsQuery,
    runQuery,
    runs: computed(() => runsQuery.data.value?.page),
    runsDataCriteria: computed(() => runsQuery.data.value?.criteria),
    selectedRun: computed(() => runQuery.data.value),
    startPending: computed(() => startMutation.isPending.value),
    start: (request: CreditReconciliationRequest) => startMutation.mutateAsync(request),
  }
}
