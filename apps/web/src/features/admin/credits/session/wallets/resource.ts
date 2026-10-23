import { computed, toValue, watch, type MaybeRefOrGetter } from 'vue'
import { useMutation, useQuery, useQueryClient } from '@tanstack/vue-query'
import type { ControlLedgerTransaction } from '@/api/generated-query/control/model'
import { subscribeUserCreditsInvalidation } from '@/core/query/userCreditsInvalidation'
import { PAGE_SIZES } from '@/core/config/pagination'
import { keepPrevious } from '@/core/query/queryDefaults'
import { adminCreditsBillingApi, type AdminCreditsBillingApi } from '../shared/api'
import {
  buildAdminCreditCommandRequest,
  type AdminCreditCommandInput,
} from '../../core/wallets/creditCommandContract'

const walletQueryRoot = ['control', 'credits', 'wallets'] as const
const accountWalletQueryRoot = ['control', 'credits', 'accounts'] as const

export type AdminCreditLedgerTransaction = ControlLedgerTransaction

const ADMIN_CREDIT_LEDGER_PAGE_SIZE = PAGE_SIZES.adminLedger

export type AdminWalletTarget =
  { kind: 'user'; id: string } | { kind: 'billingAccount'; id: string }

interface AdminCreditLedgerCriteria {
  target: AdminWalletTarget | null
  cursor?: string | undefined
}

type AdminCreditCommandResult = {
  transaction_id: string
  wasReplayed: boolean
}

type CommandMutations = ReturnType<typeof createCreditCommandMutations>

function createCreditCommandMutations(api: AdminCreditsBillingApi, invalidateWallet: () => void) {
  const grantMutation = useMutation({
    mutationFn: (
      input: { body: Parameters<AdminCreditsBillingApi['grant']>[0] } & {
        idempotencyKey: string
      },
    ) =>
      api.grant(input.body, {
        idempotencyKey: input.idempotencyKey,
      }),
    onSuccess: invalidateWallet,
  })
  const deductionMutation = useMutation({
    mutationFn: (
      input: { body: Parameters<AdminCreditsBillingApi['deduct']>[0] } & {
        idempotencyKey: string
      },
    ) =>
      api.deduct(input.body, {
        idempotencyKey: input.idempotencyKey,
      }),
    onSuccess: invalidateWallet,
  })
  const refundMutation = useMutation({
    mutationFn: (
      input: { body: Parameters<AdminCreditsBillingApi['refund']>[0] } & {
        idempotencyKey: string
      },
    ) =>
      api.refund(input.body, {
        idempotencyKey: input.idempotencyKey,
      }),
    onSuccess: invalidateWallet,
  })
  const correctionMutation = useMutation({
    mutationFn: (
      input: { body: Parameters<AdminCreditsBillingApi['correct']>[0] } & {
        idempotencyKey: string
      },
    ) =>
      api.correct(input.body, {
        idempotencyKey: input.idempotencyKey,
      }),
    onSuccess: invalidateWallet,
  })
  const restrictMutation = useMutation({
    mutationFn: (
      input: { body: Parameters<AdminCreditsBillingApi['restrictWallet']>[0] } & {
        idempotencyKey: string
      },
    ) =>
      api.restrictWallet(input.body, {
        idempotencyKey: input.idempotencyKey,
      }),
    onSuccess: invalidateWallet,
  })
  const releaseMutation = useMutation({
    mutationFn: (
      input: {
        restrictionId: string
        body: Parameters<AdminCreditsBillingApi['releaseWalletRestriction']>[1]
      } & { idempotencyKey: string },
    ) =>
      api.releaseWalletRestriction(input.restrictionId, input.body, {
        idempotencyKey: input.idempotencyKey,
      }),
    onSuccess: invalidateWallet,
  })

  async function executeCommand(input: AdminCreditCommandInput): Promise<AdminCreditCommandResult> {
    const built = buildAdminCreditCommandRequest(input)
    const options = { idempotencyKey: built.idempotencyKey }
    if (input.kind === 'grant') {
      const result = await grantMutation.mutateAsync({
        body: built.body as Parameters<AdminCreditsBillingApi['grant']>[0],
        ...options,
      })
      return { transaction_id: result.transaction_id, wasReplayed: result.replayed === true }
    }
    if (input.kind === 'deduction') {
      const result = await deductionMutation.mutateAsync({
        body: built.body as Parameters<AdminCreditsBillingApi['deduct']>[0],
        ...options,
      })
      return { transaction_id: result.transaction_id, wasReplayed: result.replayed === true }
    }
    if (input.kind === 'correction') {
      const result = await correctionMutation.mutateAsync({
        body: built.body as Parameters<AdminCreditsBillingApi['correct']>[0],
        ...options,
      })
      return { transaction_id: result.transaction_id, wasReplayed: result.replayed === true }
    }
    const result = await refundMutation.mutateAsync({
      body: built.body as Parameters<AdminCreditsBillingApi['refund']>[0],
      ...options,
    })
    return { transaction_id: result.transaction_id, wasReplayed: result.replayed === true }
  }

  return {
    grantMutation,
    deductionMutation,
    refundMutation,
    correctionMutation,
    restrictMutation,
    releaseMutation,
    executeCommand,
    isCommandPending: computed(
      () =>
        grantMutation.isPending.value ||
        deductionMutation.isPending.value ||
        refundMutation.isPending.value ||
        correctionMutation.isPending.value,
    ),
    isRestrictionPending: computed(
      () => restrictMutation.isPending.value || releaseMutation.isPending.value,
    ),
  }
}

export function useAdminWalletResource(
  target: MaybeRefOrGetter<AdminWalletTarget | null>,
  cursor: MaybeRefOrGetter<string | undefined>,
  dependencies: { api?: AdminCreditsBillingApi } = {},
) {
  const api = dependencies.api ?? adminCreditsBillingApi
  const queryClient = useQueryClient()
  const currentTarget = computed<AdminWalletTarget | null>(() => {
    const current = toValue(target)
    return current ? { kind: current.kind, id: current.id } : null
  })
  const queryRoot = computed(() =>
    currentTarget.value?.kind === 'billingAccount' ? accountWalletQueryRoot : walletQueryRoot,
  )
  const ledgerCriteria = computed<AdminCreditLedgerCriteria>(() => ({
    target: currentTarget.value,
    cursor: toValue(cursor),
  }))
  const isEnabled = computed(() => Boolean(currentTarget.value?.id))

  const walletQuery = useQuery({
    queryKey: computed(() => [...queryRoot.value, currentTarget.value?.id ?? '', 'summary']),
    queryFn: ({ signal }) => {
      const target = currentTarget.value
      if (!target?.id) throw new Error('Wallet target is required')
      return target.kind === 'user'
        ? api.getWallet(target.id, { signal })
        : api.getAccountWallet(target.id, { signal })
    },
    enabled: isEnabled,
  })
  const ledgerQuery = useQuery({
    queryKey: computed(() => [
      ...queryRoot.value,
      currentTarget.value?.id ?? '',
      'ledger',
      ledgerCriteria.value,
    ]),
    queryFn: async ({ queryKey, signal }) => {
      const criteria = queryKey.at(-1) as AdminCreditLedgerCriteria
      const target = criteria.target
      if (!target?.id) throw new Error('Wallet target is required')
      const params = {
        ...(criteria.cursor === undefined ? {} : { cursor: criteria.cursor }),
        page_size: ADMIN_CREDIT_LEDGER_PAGE_SIZE,
      }
      const page = await (target.kind === 'user'
        ? api.listLedger(target.id, params, { signal })
        : api.listAccountLedger(target.id, params, { signal }))
      return { criteria, page }
    },
    enabled: isEnabled,
    ...keepPrevious(),
  })

  // Commands may finish after the dialog changes target. Invalidate both
  // namespaces so the affected wallet cannot keep a stale summary or ledger.
  function invalidateWallet() {
    return Promise.all([
      queryClient.invalidateQueries({ queryKey: walletQueryRoot }),
      queryClient.invalidateQueries({ queryKey: accountWalletQueryRoot }),
    ])
  }

  watch(
    () => currentTarget.value?.kind === 'user',
    (isUser, _, onCleanup) => {
      if (!isUser) return
      onCleanup(
        subscribeUserCreditsInvalidation(() => {
          void queryClient.invalidateQueries({ queryKey: walletQueryRoot })
        }),
      )
    },
    { immediate: true, flush: 'sync' },
  )

  const commands: CommandMutations = createCreditCommandMutations(api, () => {
    void invalidateWallet()
  })

  return {
    walletQuery,
    ledgerQuery,
    wallet: computed(() => walletQuery.data.value),
    ledger: computed(() => ledgerQuery.data.value?.page),
    ledgerDataCriteria: computed(() => ledgerQuery.data.value?.criteria),
    isCommandPending: commands.isCommandPending,
    isRestrictionPending: commands.isRestrictionPending,
    executeCommand: commands.executeCommand,
    restrictMutation: commands.restrictMutation,
    releaseMutation: commands.releaseMutation,
  }
}
