import {
  getAdminCreditHealth,
  getAdminCreditReconciliationRun,
  listAdminCreditReconciliationFindingQueue,
  listAdminCreditReconciliationRuns,
  startAdminCreditReconciliation,
} from '../../../../shared/api/generated-query/admin-credits/admin-credits'
import {
  createControlCreditDeduction,
  createControlCreditGrant,
  createControlCreditRefund,
  getControlCreditWallet,
  listControlCreditLedger,
} from '../../../../shared/api/generated-query/control/control-credits/control-credits'
import type {
  ControlCreditCommandRequest,
  ControlCreditRefundRequest,
  ControlCreditResult,
  ControlCreditWallet,
  ControlLedgerTransaction,
  ListControlCreditLedgerParams,
} from '../../../../shared/api/generated-query/control/model'
import {
  buildContractRequestHeaders,
  newRequestId,
  requireContractData,
} from '../../../../shared/api/contractKernel'
import { resolveApiData } from '../../../../shared/api/apiResponse'

// Control wallet projection for the dialog: usr_ user id, status and
// active restrictions mirror the control wallet read.
export type ControlWalletView = {
  public_id: string
  display_name: string
  status: string
  available_credits: number
  reserved_credits: number
  active_restrictions: Array<{
    restriction_id: string
    public_id: string
    display_name: string
    type: string
    source: string
    related_object: unknown
    reason: string
    created_by: { type: string }
    created_at: string
  }>
}

export type ControlLedgerView = {
  transactions: ControlLedgerTransaction[]
  nextCursor: string | undefined
  hasMore: boolean
}

export type ControlCreditMutationOptions = {
  idempotencyKey: string
  operatorReason: string
  signal?: AbortSignal | null | undefined
}

function controlMutationHeaders(options: ControlCreditMutationOptions) {
  return buildContractRequestHeaders({
    requestId: newRequestId(),
    idempotencyKey: options.idempotencyKey,
    operatorReason: options.operatorReason,
  })
}

async function getWallet(userId: string, options?: { signal?: AbortSignal | null | undefined }) {
  const response = await getControlCreditWallet(
    userId,
    options?.signal == null ? undefined : { signal: options.signal },
  )
  const wallet = requireContractData<ControlCreditWallet>(response, '加载 Wallet 失败')
  const view: ControlWalletView = {
    public_id: wallet.user_id,
    display_name: wallet.display_name ?? '',
    status: wallet.status ?? 'active',
    available_credits: wallet.available_credits ?? 0,
    reserved_credits: wallet.reserved_credits ?? 0,
    active_restrictions: (wallet.active_restrictions ?? []).map((restriction) => ({
      restriction_id: restriction.restriction_id,
      public_id: restriction.user_id,
      display_name: restriction.display_name ?? '',
      type: restriction.type,
      source: restriction.source ?? '',
      related_object: restriction.related_object,
      reason: restriction.reason ?? '',
      created_by: { type: (restriction.created_by as { type?: string } | undefined)?.type ?? '' },
      created_at: restriction.created_at ?? '',
    })),
  }
  return view
}

async function listLedger(
  userId: string,
  params?: ListControlCreditLedgerParams,
  options?: { signal?: AbortSignal | null | undefined },
) {
  const response = await listControlCreditLedger(
    userId,
    params,
    options?.signal == null ? undefined : { signal: options.signal },
  )
  // The generated client resolves the paginated envelope directly ({data,
  // meta}); HTTP errors throw via the mutator, so only shape-guard here.
  const transactions = Array.isArray(response.data) ? response.data : []
  const meta = response.meta
  const view: ControlLedgerView = {
    transactions: transactions.map((transaction) => ({
      sequence_id: transaction.sequence_id,
      transaction_id: transaction.transaction_id,
      command_type: transaction.command_type,
      amount_credits: transaction.amount_credits,
      ...(transaction.available_credits_after === undefined
        ? {}
        : { available_credits_after: transaction.available_credits_after }),
      ...(transaction.reason === undefined ? {} : { reason: transaction.reason }),
      ...(transaction.charge === undefined ? {} : { charge: transaction.charge }),
      created_at: transaction.created_at,
    })),
    nextCursor:
      meta?.has_more && typeof meta.next_cursor === 'string' && meta.next_cursor
        ? meta.next_cursor
        : undefined,
    hasMore: meta?.has_more === true,
  }
  return view
}

async function grant(body: ControlCreditCommandRequest, options: ControlCreditMutationOptions) {
  const response = await createControlCreditGrant(body, {
    headers: controlMutationHeaders(options),
  })
  return requireContractData<ControlCreditResult>(response, '赠送Idea失败')
}

async function deduct(body: ControlCreditCommandRequest, options: ControlCreditMutationOptions) {
  const response = await createControlCreditDeduction(body, {
    headers: controlMutationHeaders(options),
  })
  return requireContractData<ControlCreditResult>(response, '扣减Idea失败')
}

async function refund(body: ControlCreditRefundRequest, options: ControlCreditMutationOptions) {
  const response = await createControlCreditRefund(body, {
    headers: controlMutationHeaders(options),
  })
  return requireContractData<ControlCreditResult>(response, '退款Idea失败')
}

async function getCreditHealth(...args: Parameters<typeof getAdminCreditHealth>) {
  return resolveApiData(getAdminCreditHealth(...args), '加载账本健康状态失败')
}

async function startReconciliation(...args: Parameters<typeof startAdminCreditReconciliation>) {
  return resolveApiData(startAdminCreditReconciliation(...args), '启动账本检查失败')
}

async function listReconciliationRuns(
  ...args: Parameters<typeof listAdminCreditReconciliationRuns>
) {
  return resolveApiData(listAdminCreditReconciliationRuns(...args), '加载账本检查记录失败')
}

async function getReconciliationRun(...args: Parameters<typeof getAdminCreditReconciliationRun>) {
  return resolveApiData(getAdminCreditReconciliationRun(...args), '加载账本检查详情失败')
}

async function listReconciliationFindingQueue(
  ...args: Parameters<typeof listAdminCreditReconciliationFindingQueue>
) {
  return resolveApiData(listAdminCreditReconciliationFindingQueue(...args), '加载账本异常队列失败')
}

export const adminCreditsBillingApi = {
  getWallet,
  listLedger,
  grant,
  deduct,
  refund,
  getCreditHealth,
  startReconciliation,
  listReconciliationRuns,
  getReconciliationRun,
  listReconciliationFindingQueue,
}

export type AdminCreditsBillingApi = typeof adminCreditsBillingApi
