import { computed, onScopeDispose, toValue, type MaybeRefOrGetter } from 'vue'
import { useMutation, useQuery, useQueryClient } from '@tanstack/vue-query'
import type {
  AdminCreditDeductionRequest,
  AdminCreditGrantRequest,
  AdminCreditRefundRequest,
  CreditLedgerTransaction,
  ListAdminCreditLedgerParams,
} from '../../../../shared/api/generated-query/model'
import { subscribeUserCreditsInvalidation } from '../../../../shared/query/userCreditsInvalidation'
import { stripUndefined } from '../../../../shared/api/stripUndefined'
import { adminCreditsBillingApi, type AdminCreditsBillingApi } from '../shared/api'
import {
  buildAdminCreditCommandRequest,
  type AdminCreditCommandInput,
} from './creditCommandContract'

const walletQueryRoot = ['admin', 'credits', 'wallets'] as const

export type AdminCreditLedgerTransaction = CreditLedgerTransaction

interface AdminCreditLedgerCriteria {
  userPublicId: string
  beforeSequence?: number | undefined
}

export function useAdminCreditWalletResource(
  userPublicId: MaybeRefOrGetter<string | null>,
  beforeSequence: MaybeRefOrGetter<number | undefined>,
  dependencies: {
    api?: AdminCreditsBillingApi
    idFactory?: () => string
  } = {},
) {
  const api = dependencies.api ?? adminCreditsBillingApi
  const queryClient = useQueryClient()
  const normalizedUserPublicId = computed(() => toValue(userPublicId) ?? '')
  const ledgerCriteria = computed<AdminCreditLedgerCriteria>(() => ({
    userPublicId: normalizedUserPublicId.value,
    beforeSequence: toValue(beforeSequence),
  }))
  const enabled = computed(() => normalizedUserPublicId.value.length > 0)

  const walletQuery = useQuery({
    queryKey: computed(() => [...walletQueryRoot, normalizedUserPublicId.value, 'summary']),
    queryFn: ({ signal }) => api.getWallet(normalizedUserPublicId.value, { signal }),
    enabled,
  })
  const ledgerQuery = useQuery({
    queryKey: computed(() => [
      ...walletQueryRoot,
      normalizedUserPublicId.value,
      'ledger',
      ledgerCriteria.value,
    ]),
    queryFn: async ({ queryKey, signal }) => {
      const criteria = queryKey.at(-1) as AdminCreditLedgerCriteria
      const params: ListAdminCreditLedgerParams = stripUndefined({
        before_sequence: criteria.beforeSequence,
        limit: 25,
      })
      return {
        criteria,
        page: await api.listLedger(criteria.userPublicId, params, { signal }),
      }
    },
    enabled,
    placeholderData: (previous) => previous,
  })

  function invalidateWallet() {
    return queryClient.invalidateQueries({ queryKey: walletQueryRoot })
  }

  const unsubscribeCreditsInvalidation = subscribeUserCreditsInvalidation(() => {
    void invalidateWallet()
  })
  onScopeDispose(unsubscribeCreditsInvalidation)

  const grantMutation = useMutation({
    mutationFn: (request: AdminCreditGrantRequest) => api.grant(request),
    onSuccess: invalidateWallet,
  })
  const deductionMutation = useMutation({
    mutationFn: (request: AdminCreditDeductionRequest) => api.deduct(request),
    onSuccess: invalidateWallet,
  })
  const refundMutation = useMutation({
    mutationFn: (request: AdminCreditRefundRequest) => api.refund(request),
    onSuccess: invalidateWallet,
  })

  async function executeCommand(input: AdminCreditCommandInput) {
    const request = buildAdminCreditCommandRequest(input, dependencies.idFactory)
    if (input.kind === 'grant') {
      return grantMutation.mutateAsync(request as AdminCreditGrantRequest)
    }
    if (input.kind === 'deduction') {
      return deductionMutation.mutateAsync(request as AdminCreditDeductionRequest)
    }
    return refundMutation.mutateAsync(request as AdminCreditRefundRequest)
  }

  return {
    walletQuery,
    ledgerQuery,
    wallet: computed(() => walletQuery.data.value),
    ledger: computed(() => ledgerQuery.data.value?.page),
    ledgerDataCriteria: computed(() => ledgerQuery.data.value?.criteria),
    commandPending: computed(
      () =>
        grantMutation.isPending.value ||
        deductionMutation.isPending.value ||
        refundMutation.isPending.value,
    ),
    executeCommand,
  }
}
