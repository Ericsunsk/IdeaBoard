import { computed, onScopeDispose, toValue, type MaybeRefOrGetter } from 'vue'
import { useMutation, useQuery, useQueryClient } from '@tanstack/vue-query'
import type { ControlLedgerTransaction } from '../../../../shared/api/generated-query/control/model'
import { subscribeUserCreditsInvalidation } from '../../../../shared/query/userCreditsInvalidation'
import { PAGE_SIZES } from '../../../../shared/config/pagination'
import { keepPrevious } from '../../../../shared/query/queryDefaults'
import { adminCreditsBillingApi, type AdminCreditsBillingApi } from '../shared/api'
import {
  buildAdminCreditCommandRequest,
  type AdminCreditCommandInput,
} from './creditCommandContract'

const walletQueryRoot = ['control', 'credits', 'wallets'] as const

export type AdminCreditLedgerTransaction = ControlLedgerTransaction

const ADMIN_CREDIT_LEDGER_PAGE_SIZE = PAGE_SIZES.adminLedger

interface AdminCreditLedgerCriteria {
  userPublicId: string
  cursor?: string | undefined
}

export type AdminCreditCommandResult = {
  transaction_id: string
  replayed: boolean
}

export function useAdminCreditWalletResource(
  userPublicId: MaybeRefOrGetter<string | null>,
  cursor: MaybeRefOrGetter<string | undefined>,
  dependencies: {
    api?: AdminCreditsBillingApi
  } = {},
) {
  const api = dependencies.api ?? adminCreditsBillingApi
  const queryClient = useQueryClient()
  const normalizedUserPublicId = computed(() => toValue(userPublicId) ?? '')
  const normalizedCursor = computed(() => toValue(cursor))
  const ledgerCriteria = computed<AdminCreditLedgerCriteria>(() => ({
    userPublicId: normalizedUserPublicId.value,
    ...(normalizedCursor.value === undefined ? {} : { cursor: normalizedCursor.value }),
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
      const page = await api.listLedger(
        criteria.userPublicId,
        {
          ...(criteria.cursor === undefined ? {} : { cursor: criteria.cursor }),
          page_size: ADMIN_CREDIT_LEDGER_PAGE_SIZE,
        },
        { signal },
      )
      return { criteria, page }
    },
    enabled,
    ...keepPrevious(),
  })

  function invalidateWallet() {
    return queryClient.invalidateQueries({ queryKey: walletQueryRoot })
  }

  const unsubscribeCreditsInvalidation = subscribeUserCreditsInvalidation(() => {
    void invalidateWallet()
  })
  onScopeDispose(unsubscribeCreditsInvalidation)

  const grantMutation = useMutation({
    mutationFn: (
      input: { body: Parameters<AdminCreditsBillingApi['grant']>[0] } & {
        idempotencyKey: string
        operatorReason: string
      },
    ) =>
      api.grant(input.body, {
        idempotencyKey: input.idempotencyKey,
        operatorReason: input.operatorReason,
      }),
    onSuccess: invalidateWallet,
  })
  const deductionMutation = useMutation({
    mutationFn: (
      input: { body: Parameters<AdminCreditsBillingApi['deduct']>[0] } & {
        idempotencyKey: string
        operatorReason: string
      },
    ) =>
      api.deduct(input.body, {
        idempotencyKey: input.idempotencyKey,
        operatorReason: input.operatorReason,
      }),
    onSuccess: invalidateWallet,
  })
  const refundMutation = useMutation({
    mutationFn: (
      input: { body: Parameters<AdminCreditsBillingApi['refund']>[0] } & {
        idempotencyKey: string
        operatorReason: string
      },
    ) =>
      api.refund(input.body, {
        idempotencyKey: input.idempotencyKey,
        operatorReason: input.operatorReason,
      }),
    onSuccess: invalidateWallet,
  })

  async function executeCommand(input: AdminCreditCommandInput): Promise<AdminCreditCommandResult> {
    const built = buildAdminCreditCommandRequest(input)
    const options = { idempotencyKey: built.idempotencyKey, operatorReason: built.operatorReason }
    if (input.kind === 'grant') {
      const result = await grantMutation.mutateAsync({
        body: built.body as Parameters<AdminCreditsBillingApi['grant']>[0],
        ...options,
      })
      return { transaction_id: result.transaction_id, replayed: result.replayed === true }
    }
    if (input.kind === 'deduction') {
      const result = await deductionMutation.mutateAsync({
        body: built.body as Parameters<AdminCreditsBillingApi['deduct']>[0],
        ...options,
      })
      return { transaction_id: result.transaction_id, replayed: result.replayed === true }
    }
    const result = await refundMutation.mutateAsync({
      body: built.body as Parameters<AdminCreditsBillingApi['refund']>[0],
      ...options,
    })
    return { transaction_id: result.transaction_id, replayed: result.replayed === true }
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
