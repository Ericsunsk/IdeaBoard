import { computed, toValue, type MaybeRefOrGetter } from 'vue'
import { useQuery } from '@tanstack/vue-query'
import type {
  CreditReconciliationFinding,
  ListAdminCreditReconciliationFindingQueueParams,
} from '../../../../shared/api/generated-query/model'
import { adminCreditsBillingApi, type AdminCreditsBillingApi } from '../creditsBillingApi'

export type AdminCreditFindingStatus = 'open' | 'resolved'
export type AdminCreditFinding = CreditReconciliationFinding

interface FindingQueueCriteria {
  status: AdminCreditFindingStatus
  beforeId?: number
}

export function useAdminCreditFindingQueueResource(
  input: {
    status: MaybeRefOrGetter<AdminCreditFindingStatus>
    beforeId: MaybeRefOrGetter<number | undefined>
  },
  dependencies: { api?: AdminCreditsBillingApi } = {},
) {
  const api = dependencies.api ?? adminCreditsBillingApi
  const criteria = computed<FindingQueueCriteria>(() => ({
    status: toValue(input.status),
    beforeId: toValue(input.beforeId),
  }))
  const query = useQuery({
    queryKey: computed(() => ['admin', 'credits', 'reconciliation', 'findings', criteria.value]),
    queryFn: async ({ queryKey, signal }) => {
      const requestCriteria = queryKey.at(-1) as FindingQueueCriteria
      const params: ListAdminCreditReconciliationFindingQueueParams = {
        status: requestCriteria.status,
        before_id: requestCriteria.beforeId,
        limit: 25,
      }
      return {
        criteria: requestCriteria,
        page: await api.listReconciliationFindingQueue(params, { signal }),
      }
    },
    placeholderData: (previous) => previous,
    refetchInterval: (query) => {
      const requestCriteria = query.queryKey.at(-1) as FindingQueueCriteria
      return requestCriteria.status === 'open' ? 5_000 : 30_000
    },
  })

  const currentPage = computed(() => {
    const data = query.data.value
    return data?.criteria.status === criteria.value.status &&
      data.criteria.beforeId === criteria.value.beforeId
      ? data.page
      : undefined
  })

  return {
    query,
    page: currentPage,
    dataCriteria: computed(() => query.data.value?.criteria),
  }
}
