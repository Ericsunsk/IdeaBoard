// Credit health view types: feature-owned copies of the reconciliation
// read model served by the Control Plane (credits health/reconciliation
// reads). The backend serves the same Credits services as before; only the
// transport moved (/api/control/v1 with the contract envelope), so views
// keep working without generated legacy model code.
export const CreditHealthStatus = {
  unchecked: 'unchecked',
  healthy: 'healthy',
  attention: 'attention',
  critical: 'critical',
  stale: 'stale',
  failed: 'failed',
} as const

export type CreditHealthStatus = (typeof CreditHealthStatus)[keyof typeof CreditHealthStatus]

export type CreditHealthFindingSummary = {
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

export type CreditReconciliationRunStatus =
  (typeof CreditReconciliationRunStatus)[keyof typeof CreditReconciliationRunStatus]

const CreditReconciliationRunTrigger = {
  manual: 'manual',
  scheduled: 'scheduled',
} as const

export type CreditReconciliationRunTrigger =
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

export type CreditReconciliationRequest = {
  reason: string
}

const CreditReconciliationFindingSeverity = {
  warning: 'warning',
  error: 'error',
  critical: 'critical',
} as const

export type CreditReconciliationFindingSeverity =
  (typeof CreditReconciliationFindingSeverity)[keyof typeof CreditReconciliationFindingSeverity]

const CreditReconciliationFindingRisk = {
  low: 'low',
  high: 'high',
} as const

export type CreditReconciliationFindingRisk =
  (typeof CreditReconciliationFindingRisk)[keyof typeof CreditReconciliationFindingRisk]

const CreditReconciliationFindingStatus = {
  open: 'open',
  resolved: 'resolved',
} as const

export type CreditReconciliationFindingStatus =
  (typeof CreditReconciliationFindingStatus)[keyof typeof CreditReconciliationFindingStatus]

export type CreditReconciliationFindingType =
  | 'ledger_postings_invalid'
  | 'command_shape_invalid'
  | 'wallet_projection_mismatch'
  | 'stale_hold'
  | 'task_hold_missing_billing_snapshot'
  | 'task_trace_missing'
  | 'task_billing_fact_missing'
  | 'active_task_not_held'
  | 'terminal_task_settlement_mismatch'
  | 'task_terminal_transaction_count'
  | 'idempotency_request_hash_conflict'
  | 'task_settlement_reference_invalid'
  | 'duplicate_business_reference'
  | 'correction_amount_exceeds_original'
  | 'refund_amount_invalid'
  | 'illegal_reference_chain'
  | 'hold_state_invalid'
  | 'hold_terminal_transaction_count'
  | 'charge_state_invalid'

export type CreditReconciliationFindingDetails = { [key: string]: unknown }

export type CreditReconciliationFinding = {
  id: number
  fingerprint: string
  finding_type: CreditReconciliationFindingType
  severity: CreditReconciliationFindingSeverity
  risk: CreditReconciliationFindingRisk
  wallet_public_id?: string
  wallet_display_name?: string
  subject_type: string
  subject_id: string
  details: CreditReconciliationFindingDetails
  status: CreditReconciliationFindingStatus
  isolated: boolean
  occurrence_count: number
  first_seen_at: string
  last_seen_at: string
  resolved_at?: string
}

export type ListAdminCreditReconciliationRunsParams = {
  before_id?: number | undefined
  limit?: number | undefined
}

export type ListAdminCreditReconciliationFindingQueueParams = {
  status: CreditReconciliationFindingStatus
  before_id?: number | undefined
  limit?: number | undefined
}
