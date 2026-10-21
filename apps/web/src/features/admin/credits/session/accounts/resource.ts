import { computed, toValue, type MaybeRefOrGetter } from 'vue'
import { useQuery } from '@tanstack/vue-query'
import { stripUndefined } from '../../../../../api/stripUndefined'
import { PAGE_SIZES } from '../../../../../core/config/pagination'
import { keepPrevious } from '../../../../../core/query/queryDefaults'
import {
  adminCreditsBillingApi,
  type AdminBillingAccountListItem,
  type AdminBillingAccountOwnerScope,
  type AdminCreditsBillingApi,
} from '../shared/api'

const accountListQueryRoot = ['control', 'billing', 'accounts'] as const

export type AdminBillingAccountCriteria = {
  ownerScope: AdminBillingAccountOwnerScope
  cursor?: string | undefined
}

export function useAdminBillingAccountListResource(
  criteria: MaybeRefOrGetter<AdminBillingAccountCriteria>,
  dependencies: { api?: AdminCreditsBillingApi } = {},
) {
  const api = dependencies.api ?? adminCreditsBillingApi
  const normalized = computed<AdminBillingAccountCriteria>(() => toValue(criteria))
  const query = useQuery({
    queryKey: computed(() => [...accountListQueryRoot, normalized.value]),
    queryFn: async ({ queryKey, signal }) => {
      const current = queryKey.at(-1) as AdminBillingAccountCriteria
      const page = await api.listBillingAccounts(
        stripUndefined({
          ...(current.ownerScope === 'all' ? {} : { owner_scope: current.ownerScope }),
          ...(current.cursor === undefined ? {} : { cursor: current.cursor }),
          page_size: PAGE_SIZES.adminLedger,
        }),
        { signal },
      )
      return { criteria: current, page }
    },
    ...keepPrevious(),
  })
  return {
    query,
    accounts: computed<AdminBillingAccountListItem[]>(() => query.data.value?.page.accounts ?? []),
    nextCursor: computed(() => query.data.value?.page.nextCursor),
    hasMore: computed(() => query.data.value?.page.hasMore === true),
    dataCriteria: computed(() => query.data.value?.criteria),
  }
}

export function useAdminBillingAccountDetailResource(
  billingAccountId: MaybeRefOrGetter<string | null>,
  dependencies: { api?: AdminCreditsBillingApi } = {},
) {
  const api = dependencies.api ?? adminCreditsBillingApi
  const normalized = computed(() => toValue(billingAccountId) ?? '')
  const isEnabled = computed(() => normalized.value.length > 0)
  const query = useQuery({
    queryKey: computed(() => [...accountListQueryRoot, normalized.value, 'detail']),
    queryFn: ({ signal }) => api.getBillingAccount(normalized.value, { signal }),
    enabled: isEnabled,
  })
  return {
    query,
    detail: computed(() => query.data.value),
  }
}
