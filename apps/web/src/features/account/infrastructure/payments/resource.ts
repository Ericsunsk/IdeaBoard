import { inject, type InjectionKey } from 'vue'
import { api } from '../../../../shared/api/client'
import {
  resolveApiData,
  type ApiResponseWithData,
} from '../../../../shared/api/apiResponse'
import { i18n } from '../../../../shared/i18n'

// User payments boundary (issue #809). The generated `user-payments` client
// is gone but the backend user plane still serves `/api/user/payments/*`
// with the shared envelope, so this boundary transports through the central
// `api.request` with local view types. The event subscription still rides
// the surviving `/api/user/events` stream.
export type PaymentCheckoutRequest = {
  idempotency_key: string
  idea_quantity: number
  terms_accepted: true
  terms_version: string
  refund_policy_version: string
}

export type PaymentCheckoutSession = {
  checkout_url: string
  public_token: string
  expires_at: string
  reused: boolean
}

const PaymentOrderReturnStatusStatus = {
  processing: 'processing',
  credited: 'credited',
  failed: 'failed',
} as const

type PaymentOrderReturnStatusStatus =
  (typeof PaymentOrderReturnStatusStatus)[keyof typeof PaymentOrderReturnStatusStatus]

export type PaymentOrderReturnStatus = {
  idea_quantity: number
  status: PaymentOrderReturnStatusStatus
  updated_at: string
}

export type PaymentLegalDocument = {
  version: string
  url: string
}

export type PaymentPurchaseOptions = {
  purchases_available: boolean
  unavailable_reason?: string
  restriction_type?: string
  quick_idea_quantities: number[]
  min_idea_quantity: number
  max_idea_quantity: number
  ideas_per_usd: number
  terms: PaymentLegalDocument
  refund_policy: PaymentLegalDocument
}

const PendingPaymentOrderStatus = {
  checkout_open: 'checkout_open',
  async_payment_pending: 'async_payment_pending',
  paid_pending_credit: 'paid_pending_credit',
  credited: 'credited',
  cancelled: 'cancelled',
  expired: 'expired',
  payment_failed: 'payment_failed',
} as const

type PendingPaymentOrderStatus =
  (typeof PendingPaymentOrderStatus)[keyof typeof PendingPaymentOrderStatus]

export type PendingPaymentOrder = {
  public_token: string
  idea_quantity: number
  checkout_url?: string
  status: PendingPaymentOrderStatus
  expires_at: string
  updated_at: string
}

export type PendingPaymentOrderList = {
  items: PendingPaymentOrder[]
}

export type RechargeHistoryItem = {
  occurred_at: string
  idea_quantity: number
  paid_amount_minor?: number
  paid_currency?: string
  status: string
  refunded_idea_quantity?: number
  refunded_amount_minor?: number
  receipt_url?: string
}

export type RechargeHistoryPage = {
  items: RechargeHistoryItem[]
  next_cursor?: string
}
export type PaymentOrderStatus = PaymentOrderReturnStatusStatus

export type PaymentEvent = {
  public_token?: string
  status: PaymentOrderStatus | 'refund_updated' | 'dispute_updated'
}

export interface PaymentsResource {
  getPurchaseOptions(signal?: AbortSignal): Promise<PaymentPurchaseOptions>
  createCheckout(request: PaymentCheckoutRequest): Promise<PaymentCheckoutSession>
  getReturnStatus(publicToken: string, signal?: AbortSignal): Promise<PaymentOrderReturnStatus>
  listPendingOrders(signal?: AbortSignal): Promise<PendingPaymentOrderList>
  continuePendingOrder(publicToken: string): Promise<PendingPaymentOrder>
  abandonPendingOrder(publicToken: string): Promise<PendingPaymentOrder>
  listRecharges(
    params: { cursor?: string; limit: number },
    signal?: AbortSignal,
  ): Promise<RechargeHistoryPage>
  subscribe(listener: (event: PaymentEvent) => void): () => void
}

export type PaymentsContext = {
  resource: PaymentsResource
  navigateToCheckout: (checkoutURL: string) => void
}

import { subscribeUserEvent } from '../../../../shared/api/userEventStream'

export const paymentsContextKey: InjectionKey<PaymentsContext> = Symbol('payments-context')

function paymentQueryString(params: Record<string, string | number | undefined>): string {
  const search = new URLSearchParams()
  for (const [key, value] of Object.entries(params)) {
    if (value !== undefined) search.append(key, String(value))
  }
  const query = search.toString()
  return query ? `?${query}` : ''
}

const productionResource: PaymentsResource = {
  getPurchaseOptions: (signal) =>
    resolveApiData(
      api.request<ApiResponseWithData<PaymentPurchaseOptions>>('/user/payments/purchase-options', {
        method: 'GET',
        ...(signal == null ? {} : { signal }),
      }),
      i18n.global.t('account.errors.optionsLoadFailed'),
    ),
  createCheckout: (request) =>
    resolveApiData(
      api.request<ApiResponseWithData<PaymentCheckoutSession>>(
        '/user/payments/checkout-sessions',
        {
          method: 'POST',
          body: request as unknown as Record<string, unknown>,
        },
      ),
      i18n.global.t('account.errors.checkoutCreateFailed'),
    ),
  getReturnStatus: (publicToken, signal) =>
    resolveApiData(
      api.request<ApiResponseWithData<PaymentOrderReturnStatus>>(
        `/user/payments/orders/${encodeURIComponent(publicToken)}`,
        {
          method: 'GET',
          ...(signal == null ? {} : { signal }),
        },
      ),
      i18n.global.t('account.errors.orderStatusLoadFailed'),
    ),
  listPendingOrders: (signal) =>
    resolveApiData(
      api.request<ApiResponseWithData<PendingPaymentOrderList>>('/user/payments/pending-orders', {
        method: 'GET',
        ...(signal == null ? {} : { signal }),
      }),
      i18n.global.t('account.errors.pendingOrdersLoadFailed'),
    ),
  continuePendingOrder: (publicToken) =>
    resolveApiData(
      api.request<ApiResponseWithData<PendingPaymentOrder>>(
        `/user/payments/pending-orders/${encodeURIComponent(publicToken)}/continue`,
        { method: 'POST' },
      ),
      i18n.global.t('account.errors.continuePaymentFailed'),
    ),
  abandonPendingOrder: (publicToken) =>
    resolveApiData(
      api.request<ApiResponseWithData<PendingPaymentOrder>>(
        `/user/payments/pending-orders/${encodeURIComponent(publicToken)}/abandon`,
        { method: 'POST' },
      ),
      i18n.global.t('account.errors.abandonOrderFailed'),
    ),
  listRecharges: (params, signal) =>
    resolveApiData(
      api.request<ApiResponseWithData<RechargeHistoryPage>>(
        `/user/payments/recharges${paymentQueryString({ cursor: params.cursor, limit: params.limit })}`,
        {
          method: 'GET',
          ...(signal == null ? {} : { signal }),
        },
      ),
      i18n.global.t('account.errors.historyLoadFailed'),
    ),
  subscribe: (listener) => subscribeToPaymentEvents(listener),
}

const defaultContext: PaymentsContext = {
  resource: productionResource,
  navigateToCheckout: (checkoutURL) => window.location.assign(checkoutURL),
}

export function subscribeToPaymentEvents(listener: (event: PaymentEvent) => void): () => void {
  const handlePayment = (message: MessageEvent<string>) => {
    const event = parsePaymentEvent(message.data)
    if (event) listener(event)
  }
  return subscribeUserEvent('payment.updated', handlePayment)
}

export function parsePaymentEvent(raw: string): PaymentEvent | null {
  try {
    const value: unknown = JSON.parse(raw)
    if (!value || typeof value !== 'object') return null
    const candidate = value as { public_token?: unknown; status?: unknown }
    if (
      candidate.status !== 'processing' &&
      candidate.status !== 'credited' &&
      candidate.status !== 'failed' &&
      candidate.status !== 'refund_updated' &&
      candidate.status !== 'dispute_updated'
    ) {
      return null
    }
    if (candidate.public_token !== undefined && typeof candidate.public_token !== 'string') {
      return null
    }
    return {
      status: candidate.status,
      ...(candidate.public_token ? { public_token: candidate.public_token } : {}),
    }
  } catch {
    return null
  }
}

export function usePaymentsContext(): PaymentsContext {
  return inject(paymentsContextKey, defaultContext)
}

export const paymentsQueryKeys = {
  all: ['user', 'payments'] as const,
  options: ['user', 'payments', 'purchase-options'] as const,
  pending: ['user', 'payments', 'pending-orders'] as const,
  history: ['user', 'payments', 'recharges'] as const,
  returnStatus: (publicToken: string) =>
    ['user', 'payments', 'orders', 'return', publicToken] as const,
}
