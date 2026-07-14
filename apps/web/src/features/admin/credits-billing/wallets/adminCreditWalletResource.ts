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
import { adminCreditsBillingApi, type AdminCreditsBillingApi } from '../creditsBillingApi'
import {
  buildAdminCreditCommandRequest,
  type AdminCreditCommandInput,
} from './creditCommandContract'

const walletQueryRoot = ['admin', 'credits', 'wallets'] as const

export type AdminCreditLedgerTransaction = CreditLedgerTransaction

export function useAdminCreditWalletResource(
  userId: MaybeRefOrGetter<number | null>,
  beforeSequence: MaybeRefOrGetter<number | undefined>,
  dependencies: {
    api?: AdminCreditsBillingApi
    idFactory?: () => string
  } = {},
) {
  const api = dependencies.api ?? adminCreditsBillingApi
  const queryClient = useQueryClient()
  const normalizedUserId = computed(() => toValue(userId) ?? 0)
  const ledgerParams = computed<ListAdminCreditLedgerParams>(() => ({
    before_sequence: toValue(beforeSequence),
    limit: 25,
  }))
  const enabled = computed(() => normalizedUserId.value > 0)

  const walletQuery = useQuery({
    queryKey: computed(() => [...walletQueryRoot, normalizedUserId.value, 'summary']),
    queryFn: ({ signal }) => api.getWallet(normalizedUserId.value, { signal }),
    enabled,
  })
  const ledgerQuery = useQuery({
    queryKey: computed(() => [
      ...walletQueryRoot,
      normalizedUserId.value,
      'ledger',
      ledgerParams.value,
    ]),
    queryFn: ({ signal }) => api.listLedger(normalizedUserId.value, ledgerParams.value, { signal }),
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
    ledger: computed(() => ledgerQuery.data.value),
    commandPending: computed(
      () =>
        grantMutation.isPending.value ||
        deductionMutation.isPending.value ||
        refundMutation.isPending.value,
    ),
    executeCommand,
  }
}
