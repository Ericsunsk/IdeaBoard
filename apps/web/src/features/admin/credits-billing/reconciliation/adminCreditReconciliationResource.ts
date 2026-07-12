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
  const runParams = computed<ListAdminCreditReconciliationRunsParams>(() => ({
    before_id: toValue(input.runBeforeId),
    limit: 20,
  }))
  const findingParams = computed<ListAdminCreditReconciliationFindingsParams>(() => ({
    status: toValue(input.findingStatus),
    before_id: toValue(input.findingBeforeId),
    limit: 25,
  }))

  const runsQuery = useQuery({
    queryKey: computed(() => [...reconciliationQueryRoot, 'runs', runParams.value]),
    queryFn: ({ signal }) => api.listReconciliationRuns(runParams.value, { signal }),
    placeholderData: (previous) => previous,
    refetchInterval: 5_000,
  })
  const runQuery = useQuery({
    queryKey: computed(() => [...reconciliationQueryRoot, 'runs', normalizedRunId.value]),
    queryFn: ({ signal }) => api.getReconciliationRun(normalizedRunId.value, { signal }),
    enabled: computed(() => normalizedRunId.value > 0),
    refetchInterval: (query) => {
      const status = query.state.data?.data?.status
      return status === 'queued' || status === 'running' ? 3_000 : false
    },
  })
  const findingsQuery = useQuery({
    queryKey: computed(() => [
      ...reconciliationQueryRoot,
      'runs',
      normalizedRunId.value,
      'findings',
      findingParams.value,
    ]),
    queryFn: ({ signal }) =>
      api.listReconciliationFindings(normalizedRunId.value, findingParams.value, { signal }),
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
    runs: computed(() => runsQuery.data.value?.data),
    selectedRun: computed(() => runQuery.data.value?.data),
    findings: computed(() => findingsQuery.data.value?.data),
    startPending: computed(() => startMutation.isPending.value),
    start: (request: CreditReconciliationRequest) => startMutation.mutateAsync(request),
  }
}
