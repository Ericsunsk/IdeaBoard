import { computed, toValue, type MaybeRefOrGetter } from 'vue'
import { useQuery } from '@tanstack/vue-query'
import type {
  CreditReconciliationRun,
  ListAdminCreditReconciliationRunsParams,
} from '../../../../shared/api/generated-query/model'
import { adminCreditsBillingApi, type AdminCreditsBillingApi } from '../creditsBillingApi'

export type AdminCreditRun = CreditReconciliationRun

export const adminCreditRunHistoryQueryRoot = [
  'admin',
  'credits',
  'reconciliation',
  'runs',
] as const

interface RunHistoryCriteria {
  beforeId?: number
}

export function useAdminCreditRunHistoryResource(
  input: {
    enabled: MaybeRefOrGetter<boolean>
    beforeId: MaybeRefOrGetter<number | undefined>
    selectedRunId: MaybeRefOrGetter<number | null>
  },
  dependencies: { api?: AdminCreditsBillingApi } = {},
) {
  const api = dependencies.api ?? adminCreditsBillingApi
  const enabled = computed(() => toValue(input.enabled))
  const selectedRunId = computed(() => toValue(input.selectedRunId) ?? 0)
  const criteria = computed<RunHistoryCriteria>(() => ({ beforeId: toValue(input.beforeId) }))
  const runsQuery = useQuery({
    queryKey: computed(() => [...adminCreditRunHistoryQueryRoot, criteria.value]),
    queryFn: async ({ queryKey, signal }) => {
      const requestCriteria = queryKey.at(-1) as RunHistoryCriteria
      const params: ListAdminCreditReconciliationRunsParams = {
        before_id: requestCriteria.beforeId,
        limit: 20,
      }
      return {
        criteria: requestCriteria,
        page: await api.listReconciliationRuns(params, { signal }),
      }
    },
    enabled,
    placeholderData: (previous) => previous,
    refetchInterval: computed(() => (enabled.value ? 5_000 : false)),
  })
  const runQuery = useQuery({
    queryKey: computed(() => [...adminCreditRunHistoryQueryRoot, selectedRunId.value]),
    queryFn: ({ signal }) => api.getReconciliationRun(selectedRunId.value, { signal }),
    enabled: computed(() => enabled.value && selectedRunId.value > 0),
    refetchInterval: (query) => {
      const status = query.state.data?.status
      return status === 'queued' || status === 'running' ? 3_000 : false
    },
  })

  const page = computed(() => {
    const data = runsQuery.data.value
    if (!data || data.criteria.beforeId !== criteria.value.beforeId) return undefined
    return data.page
  })

  return {
    runsQuery,
    runQuery,
    page,
    dataCriteria: computed(() => runsQuery.data.value?.criteria),
    selectedRun: computed(() => runQuery.data.value),
  }
}
