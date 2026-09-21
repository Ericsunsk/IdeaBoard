import { api } from '../../../../shared/api/client'
import { resolveApiData } from '../../../../shared/api/apiResponse'
import { getCreateAdminPaymentRefundUrl } from '../../../../shared/api/generated-query/admin-payments/admin-payments'
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

type ApiEnvelope<T> = {
  code: number
  msg: string
  data?: T | null
}

export type AdminPaymentOrderApiAdapter = {
  search: (input: SearchAdminPaymentOrdersInput) => Promise<PaymentOrderApiPage>
  get: (publicOrderNumber: string, signal?: AbortSignal) => Promise<PaymentOrderApiDetail>
  refund: (
    publicOrderNumber: string,
    input: { idea_quantity: number; idempotency_key: string },
  ) => Promise<PaymentOrderApiRefund>
}

type PaymentOrderApiTransport = Pick<typeof api, 'request'>

export function createAdminPaymentOrderApiAdapter(
  transport: PaymentOrderApiTransport = api,
): AdminPaymentOrderApiAdapter {
  return {
    search: ({ orderNumber, user, status, anomalyOnly, cursor, limit = 20, signal }) =>
      resolveApiData(
        transport.request<ApiEnvelope<PaymentOrderApiPage>>('/admin/payments/orders', {
          query: {
            ...(orderNumber?.trim() ? { order_number: orderNumber.trim().toUpperCase() } : {}),
            ...(user?.trim() ? { user: user.trim() } : {}),
            ...(status ? { status } : {}),
            ...(anomalyOnly ? { anomaly_only: true } : {}),
            ...(cursor ? { cursor } : {}),
            limit,
          },
          ...(signal ? { signal } : {}),
        }),
        '加载支付订单失败',
      ),
    get: (publicOrderNumber, signal) =>
      resolveApiData(
        transport.request<ApiEnvelope<PaymentOrderApiDetail>>(
          `/admin/payments/orders/${encodeURIComponent(publicOrderNumber.trim().toUpperCase())}`,
          signal ? { signal } : undefined,
        ),
        '加载支付订单详情失败',
      ),
    refund: (publicOrderNumber, input) =>
      resolveApiData(
        transport.request<ApiEnvelope<PaymentOrderApiRefund>>(
          getCreateAdminPaymentRefundUrl(publicOrderNumber.trim().toUpperCase()),
          { method: 'POST', body: input },
        ),
        '发起退款失败',
      ),
  }
}
