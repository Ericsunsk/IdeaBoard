import {
  createControlPaymentRefund,
  getControlPaymentOrder,
  listControlPaymentOrders,
} from '../../../../shared/api/generated-query/control/control-payments/control-payments'
import type { ListControlPaymentOrdersParams } from '../../../../shared/api/generated-query/control/model'
import {
  buildContractRequestHeaders,
  newRequestId,
  requireContractData,
} from '../../../../shared/api/contractKernel'
import { stripUndefined } from '../../../../shared/api/stripUndefined'
import { PAGE_SIZES } from '../../../../shared/config/pagination'
import type { AdminPaymentOrderStatus, SearchAdminPaymentOrdersInput } from './paymentOrderTypes'

export type PaymentOrderApiUser = {
  public_id: string
  display_name: string
}

export type PaymentOrderApiSummary = {
  public_order_number: string
  user: PaymentOrderApiUser
  idea_quantity: number
  base_amount_minor: number
  base_currency: string
  paid_amount_minor?: number
  paid_currency?: string
  status: AdminPaymentOrderStatus
  stripe_status: string
  credit_status: string
  created_at: string
  paid_at?: string
  credited_at?: string
  last_checked_at?: string
  anomalous_at?: string
}

export type PaymentOrderApiPage = {
  orders: PaymentOrderApiSummary[]
  total: number
  next_cursor?: string
}

type PaymentOrderApiStripeSummary = {
  account_id?: string
  customer_id?: string
  checkout_session_id?: string
  payment_intent_id?: string
  charge_id?: string
}

type PaymentOrderApiReconciliation = {
  anomalous: boolean
  anomalous_at?: string
  last_checked_at?: string
  next_attempt_at?: string
  last_error_code?: string
  last_error_summary?: string
}

export type PaymentOrderApiRefund = {
  id?: string
  status: string
  idea_quantity?: number
  amount_minor?: number
  currency?: string
  stripe_status?: string
  updated_at?: string
  error_summary?: string
}

export type PaymentOrderApiDispute = {
  id?: string
  status: string
  provider_status?: string
  amount_minor?: number
  currency?: string
  ratio_basis_points?: number
  target_idea_quantity?: number
  debited_idea_quantity?: number
  unrecovered_credits?: number
  stripe_dashboard_url?: string
  opened_at?: string
  closed_at?: string
  updated_at?: string
}

export type PaymentOrderApiDetail = {
  public_order_number: string
  user: PaymentOrderApiUser
  idea_quantity: number
  base_amount_minor: number
  base_currency: string
  paid_subtotal_minor?: number
  paid_tax_minor?: number
  paid_total_minor?: number
  paid_currency?: string
  status: AdminPaymentOrderStatus
  stripe_status: string
  credit_status: string
  stripe: PaymentOrderApiStripeSummary
  terms_version: string
  refund_policy_version: string
  receipt_url?: string
  expires_at: string
  created_at: string
  updated_at: string
  paid_at?: string
  credited_at?: string
  reconciliation: PaymentOrderApiReconciliation
  refunds?: PaymentOrderApiRefund[]
  disputes?: PaymentOrderApiDispute[]
  refundable_idea_quantity?: number
}

export type AdminPaymentOrderApiAdapter = {
  search: (input: SearchAdminPaymentOrdersInput) => Promise<PaymentOrderApiPage>
  get: (publicOrderNumber: string, signal?: AbortSignal) => Promise<PaymentOrderApiDetail>
  refund: (
    publicOrderNumber: string,
    input: { idea_quantity: number; idempotency_key: string; operatorReason: string },
  ) => Promise<PaymentOrderApiRefund>
}

// Admin payment order transport (W2 transports). Reads and refunds go
// through the Control Plane order operations with the contract envelope
// (`{data}` over the same AdminService projections); refunds carry a
// header-only Idempotency-Key plus an operator reason (mirrored into the
// body `reason` field) and emit a control audit record. The deleted
// `/api/admin/payments/orders*` routes are gone; this module is the single
// transport.
export function createAdminPaymentOrderApiAdapter(): AdminPaymentOrderApiAdapter {
  return {
    search: async ({ orderNumber, user, status, anomalyOnly, cursor, limit = PAGE_SIZES.standard, signal }) => {
      // The generated filter param is typed as order_number only, but the
      // control contract accepts the same six legacy filters (see the
      // openapi description on ListControlPaymentOrdersParams).
      const params = stripUndefined({
        order_number: orderNumber?.trim() ? orderNumber.trim().toUpperCase() : undefined,
        user: user?.trim() ? user.trim() : undefined,
        status: status ?? undefined,
        anomaly_only: anomalyOnly ? 'true' : undefined,
        cursor: cursor ?? undefined,
        limit,
      }) as ListControlPaymentOrdersParams
      try {
        const body = await listControlPaymentOrders(
          params,
          ...(signal == null ? [] : [{ signal }]),
        )
        return requireContractData<PaymentOrderApiPage>(body, '加载支付订单失败')
      } catch (error) {
        throw withControlCopy(error, '加载支付订单失败')
      }
    },
    get: async (publicOrderNumber, signal) => {
      try {
        const body = await getControlPaymentOrder(
          publicOrderNumber.trim().toUpperCase(),
          ...(signal == null ? [] : [{ signal }]),
        )
        return requireContractData<PaymentOrderApiDetail>(body, '加载支付订单详情失败')
      } catch (error) {
        throw withControlCopy(error, '加载支付订单详情失败')
      }
    },
    refund: async (publicOrderNumber, input) => {
      const operatorReason = requireOperatorReason(input.operatorReason)
      try {
        const body = await createControlPaymentRefund(
          publicOrderNumber.trim().toUpperCase(),
          { idea_quantity: input.idea_quantity, reason: operatorReason },
          {
            headers: buildContractRequestHeaders({
              requestId: newRequestId(),
              idempotencyKey: input.idempotency_key,
              operatorReason,
            }),
          },
        )
        return requireContractData<PaymentOrderApiRefund>(body, '发起退款失败')
      } catch (error) {
        throw withControlCopy(error, '发起退款失败')
      }
    },
  }
}

function requireOperatorReason(reason: string): string {
  const trimmed = reason.trim()
  if (!trimmed) throw controlError('control_reason_required', '需要填写操作原因')
  return trimmed
}

function controlError(code: string, message: string): Error & { code?: string | undefined } {
  const error = new Error(message) as Error & { code?: string | undefined }
  error.code = code
  return error
}

// withControlCopy maps control error codes to operator-facing copy.
// Branch on code, never on message text.
function withControlCopy(error: unknown, fallback: string): Error {
  const code = readErrorCode(error)
  if (code === 'idempotency_conflict' || code === 409) {
    return controlError(code === 409 ? 'idempotency_conflict' : code, '操作已提交，请勿重复操作')
  }
  if (code === 'control_reason_required') {
    return controlError(code, '需要填写操作原因')
  }
  if (code === 'control_not_found' || code === 'not_found' || code === 404) {
    return controlError('control_not_found', '支付订单不存在')
  }
  if (error instanceof Error) return error
  return controlError('control_failed', fallback)
}

function readErrorCode(error: unknown): string | number | undefined {
  if (error && typeof error === 'object' && 'code' in error) {
    const code = (error as { code?: unknown }).code
    if (typeof code === 'string' || typeof code === 'number') return code
  }
  if (error && typeof error === 'object' && 'status' in error) {
    const status = (error as { status?: unknown }).status
    if (status === 404 || status === 409) return status
  }
  return undefined
}
