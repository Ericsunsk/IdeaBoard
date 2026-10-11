import {
  getControlCreditHealth,
  getControlReconciliationRun,
  listControlReconciliationFindings,
  listControlReconciliationRuns,
  startControlReconciliation,
} from '../../../../../api/generated-query/control/control-credits/control-credits'
import type {
  ListControlReconciliationFindingsParams,
  ListControlReconciliationRunsParams,
} from '../../../../../api/generated-query/control/model'
import {
  createControlCreditDeduction,
  createControlCreditGrant,
  createControlCreditRefund,
  getControlCreditWallet,
  listControlCreditLedger,
} from '../../../../../api/generated-query/control/control-credits/control-credits'
import type {
  ControlCreditCommandRequest,
  ControlCreditRefundRequest,
  ControlCreditResult,
  ControlCreditWallet,
  ControlLedgerTransaction,
  ListControlCreditLedgerParams,
} from '../../../../../api/generated-query/control/model'
import { requireContractData } from '../../../../../api/contractKernel'
import {
  buildControlMutationHeaders,
  type ControlMutationOptions,
} from '../../../shared'

// Reconciliation views (issue #809). The legacy `admin-credits` generated
// client is gone; the same domain service now serves the Control Plane
// (`control-credits`), so these views keep the legacy snake_case shapes the
// health pages and presentations consume, unwrapped from the control
// envelope. New code must import these views from here, never generated
// legacy models.
export const CreditHealthStatus = {
  unchecked: 'unchecked',
  healthy: 'healthy',
  attention: 'attention',
  critical: 'critical',
  stale: 'stale',
  failed: 'failed',
} as const

export type CreditHealthStatus = (typeof CreditHealthStatus)[keyof typeof CreditHealthStatus]

type CreditHealthFindingSummary = {
  total: number
  warning: number
  error: number
  critical: number
  high_risk: number
}

const CreditReconciliationRunStatus = {
  queued: 'queued',
  running: 'running',
  completed: 'completed',
  failed: 'failed',
} as const

type CreditReconciliationRunStatus =
  (typeof CreditReconciliationRunStatus)[keyof typeof CreditReconciliationRunStatus]

const CreditReconciliationRunTrigger = {
  manual: 'manual',
  scheduled: 'scheduled',
} as const

type CreditReconciliationRunTrigger =
  (typeof CreditReconciliationRunTrigger)[keyof typeof CreditReconciliationRunTrigger]

export type CreditReconciliationRun = {
  id: number
  status: CreditReconciliationRunStatus
  trigger: CreditReconciliationRunTrigger
  requested_by_public_id?: string
  requested_by_display_name?: string
  reason?: string
  scheduled_for?: string
  finding_count: number
  critical_count: number
  isolated_wallet_count: number
  attempt_count: number
  error_message?: string
  requested_at: string
  started_at?: string
  completed_at?: string
}

export type CreditHealth = {
  status: CreditHealthStatus
  active_run?: CreditReconciliationRun
  latest_successful_run?: CreditReconciliationRun
  latest_failed_run?: CreditReconciliationRun
  open_findings: CreditHealthFindingSummary
  restricted_wallet_count: number
}

export const CreditReconciliationFindingKind = {
  ledger_postings_invalid: 'ledger_postings_invalid',
  command_shape_invalid: 'command_shape_invalid',
  wallet_projection_mismatch: 'wallet_projection_mismatch',
  stale_hold: 'stale_hold',
  task_hold_missing_billing_snapshot: 'task_hold_missing_billing_snapshot',
  task_trace_missing: 'task_trace_missing',
  task_billing_fact_missing: 'task_billing_fact_missing',
  active_task_not_held: 'active_task_not_held',
  terminal_task_settlement_mismatch: 'terminal_task_settlement_mismatch',
  task_terminal_transaction_count: 'task_terminal_transaction_count',
  idempotency_request_hash_conflict: 'idempotency_request_hash_conflict',
  task_settlement_reference_invalid: 'task_settlement_reference_invalid',
  duplicate_business_reference: 'duplicate_business_reference',
  correction_amount_exceeds_original: 'correction_amount_exceeds_original',
  refund_amount_invalid: 'refund_amount_invalid',
  illegal_reference_chain: 'illegal_reference_chain',
  hold_state_invalid: 'hold_state_invalid',
  hold_terminal_transaction_count: 'hold_terminal_transaction_count',
  charge_state_invalid: 'charge_state_invalid',
} as const

export type CreditReconciliationFindingKind =
  (typeof CreditReconciliationFindingKind)[keyof typeof CreditReconciliationFindingKind]

const CreditReconciliationFindingSeverity = {
  warning: 'warning',
  error: 'error',
  critical: 'critical',
} as const

type CreditReconciliationFindingSeverity =
  (typeof CreditReconciliationFindingSeverity)[keyof typeof CreditReconciliationFindingSeverity]

const CreditReconciliationFindingRisk = {
  low: 'low',
  high: 'high',
} as const

type CreditReconciliationFindingRisk =
  (typeof CreditReconciliationFindingRisk)[keyof typeof CreditReconciliationFindingRisk]

const CreditReconciliationFindingStatus = {
  open: 'open',
  resolved: 'resolved',
} as const

type CreditReconciliationFindingStatus =
  (typeof CreditReconciliationFindingStatus)[keyof typeof CreditReconciliationFindingStatus]

export type CreditReconciliationFinding = {
  id: number
  fingerprint: string
  finding_type: CreditReconciliationFindingKind
  severity: CreditReconciliationFindingSeverity
  risk: CreditReconciliationFindingRisk
  wallet_public_id?: string
  wallet_display_name?: string
  subject_type: string
  subject_id: string
  details: { [key: string]: unknown }
  status: CreditReconciliationFindingStatus
  isolated: boolean
  occurrence_count: number
  first_seen_at: string
  last_seen_at: string
  resolved_at?: string
}

export type CreditReconciliationRequest = {
  reason: string
}

export type ListAdminCreditReconciliationRunsParams = {
  before_id?: number
  limit?: number
}

export type ListAdminCreditReconciliationFindingQueueParams = {
  status: CreditReconciliationFindingStatus
  before_id?: number
  limit?: number
}

type CreditReconciliationRunPage = {
  runs: CreditReconciliationRun[]
  next_before_id?: number
}

type CreditReconciliationFindingPage = {
  findings: CreditReconciliationFinding[]
  next_before_id?: number
}

type ControlRequestOptions = {
  signal?: AbortSignal | null | undefined
}

function toControlOptions(options?: ControlRequestOptions) {
  return options?.signal == null ? undefined : { signal: options.signal }
}

// Control wallet projection for the dialog: usr_ user id, status and
// active restrictions mirror the control wallet read.
type ControlWalletView = {
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

type ControlLedgerView = {
  transactions: ControlLedgerTransaction[]
  nextCursor: string | undefined
  hasMore: boolean
}

type ControlCreditMutationOptions = ControlMutationOptions & {
  idempotencyKey: string
  signal?: AbortSignal | null | undefined
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
    headers: buildControlMutationHeaders(options),
  })
  return requireContractData<ControlCreditResult>(response, '赠送Idea失败')
}

async function deduct(body: ControlCreditCommandRequest, options: ControlCreditMutationOptions) {
  const response = await createControlCreditDeduction(body, {
    headers: buildControlMutationHeaders(options),
  })
  return requireContractData<ControlCreditResult>(response, '扣减Idea失败')
}

async function refund(body: ControlCreditRefundRequest, options: ControlCreditMutationOptions) {
  const response = await createControlCreditRefund(body, {
    headers: buildControlMutationHeaders(options),
  })
  return requireContractData<ControlCreditResult>(response, '退款Idea失败')
}

async function getCreditHealth(options?: ControlRequestOptions): Promise<CreditHealth> {
  const envelope = await getControlCreditHealth(toControlOptions(options))
  return requireContractData<CreditHealth>(envelope, '加载账本健康状态失败')
}

async function startReconciliation(
  request: CreditReconciliationRequest,
  options?: ControlRequestOptions,
): Promise<CreditReconciliationRun> {
  const reason = request.reason?.trim() ? request.reason.trim() : 'manual reconciliation'
  const envelope = await startControlReconciliation(
    { reason },
    {
      ...toControlOptions(options),
      headers: buildControlMutationHeaders({ operatorReason: reason }),
    },
  )
  return requireContractData<CreditReconciliationRun>(envelope, '启动账本检查失败')
}

async function listReconciliationRuns(
  params?: ListAdminCreditReconciliationRunsParams,
  options?: ControlRequestOptions,
): Promise<CreditReconciliationRunPage> {
  const envelope = await listControlReconciliationRuns(
    params as ListControlReconciliationRunsParams,
    toControlOptions(options),
  )
  return requireContractData<CreditReconciliationRunPage>(envelope, '加载账本检查记录失败')
}

async function getReconciliationRun(
  runId: number,
  options?: ControlRequestOptions,
): Promise<CreditReconciliationRun> {
  const envelope = await getControlReconciliationRun(runId, toControlOptions(options))
  return requireContractData<CreditReconciliationRun>(envelope, '加载账本检查详情失败')
}

async function listReconciliationFindingQueue(
  params: ListAdminCreditReconciliationFindingQueueParams,
  options?: ControlRequestOptions,
): Promise<CreditReconciliationFindingPage> {
  const envelope = await listControlReconciliationFindings(
    params as ListControlReconciliationFindingsParams,
    toControlOptions(options),
  )
  return requireContractData<CreditReconciliationFindingPage>(envelope, '加载账本异常队列失败')
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
