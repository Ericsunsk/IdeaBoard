import { computed, toValue, type MaybeRefOrGetter } from 'vue'
import { useMutation, useQuery, useQueryClient } from '@tanstack/vue-query'
import type {
  CreditReconciliationRequest,
  CreditReconciliationRun,
  ListAdminCreditReconciliationFindingsParams,
  ListAdminCreditReconciliationRunsParams,
} from '../../../../shared/api/generated-query/model'
import { adminCreditsBillingApi, type AdminCreditsBillingApi } from '../creditsBillingApi'

const reconciliationQueryRoot = ['admin', 'credits', 'reconciliation'] as const

export type AdminCreditReconciliationRun = CreditReconciliationRun

interface ReconciliationRunsCriteria {
  beforeId?: number
}

interface ReconciliationFindingsCriteria {
  runId: number
  status?: 'open' | 'resolved'
  beforeId?: number
}

export function useAdminCreditReconciliationResource(
  input: {
    runBeforeId: MaybeRefOrGetter<number | undefined>
    selectedRunId: MaybeRefOrGetter<number | null>
    findingStatus: MaybeRefOrGetter<'open' | 'resolved' | undefined>
    findingBeforeId: MaybeRefOrGetter<number | undefined>
  },
  dependencies: { api?: AdminCreditsBillingApi } = {},
) {
  const api = dependencies.api ?? adminCreditsBillingApi
  const queryClient = useQueryClient()
  const normalizedRunId = computed(() => toValue(input.selectedRunId) ?? 0)
  const runsCriteria = computed<ReconciliationRunsCriteria>(() => ({
    beforeId: toValue(input.runBeforeId),
  }))
  const findingsCriteria = computed<ReconciliationFindingsCriteria>(() => ({
    runId: normalizedRunId.value,
    status: toValue(input.findingStatus),
    beforeId: toValue(input.findingBeforeId),
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
  const findingsQuery = useQuery({
    queryKey: computed(() => [
      ...reconciliationQueryRoot,
      'runs',
      findingsCriteria.value.runId,
      'findings',
      findingsCriteria.value,
    ]),
    queryFn: async ({ queryKey, signal }) => {
      const criteria = queryKey.at(-1) as ReconciliationFindingsCriteria
      const params: ListAdminCreditReconciliationFindingsParams = {
        status: criteria.status,
        before_id: criteria.beforeId,
        limit: 25,
      }
      return {
        criteria,
        page: await api.listReconciliationFindings(criteria.runId, params, { signal }),
      }
    },
    enabled: computed(() => normalizedRunId.value > 0),
    placeholderData: (previous) => previous,
  })
  const startMutation = useMutation({
    mutationFn: (request: CreditReconciliationRequest) => api.startReconciliation(request),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: reconciliationQueryRoot }),
  })

  return {
    runsQuery,
    runQuery,
    findingsQuery,
    runs: computed(() => runsQuery.data.value?.page),
    runsDataCriteria: computed(() => runsQuery.data.value?.criteria),
    selectedRun: computed(() => runQuery.data.value),
    findings: computed(() => findingsQuery.data.value?.page),
    findingsDataCriteria: computed(() => findingsQuery.data.value?.criteria),
    startPending: computed(() => startMutation.isPending.value),
    start: (request: CreditReconciliationRequest) => startMutation.mutateAsync(request),
  }
}
