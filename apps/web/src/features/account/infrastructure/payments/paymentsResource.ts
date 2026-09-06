import { inject, type InjectionKey } from 'vue'
import { resolveApiData } from '../../../../shared/api/apiResponse'
import { i18n } from '../../../../shared/i18n'
import {
  abandonUserPendingPaymentOrder,
  continueUserPendingPaymentOrder,
  createUserPaymentCheckoutSession,
  getUserPaymentOrderReturnStatus,
  getUserPaymentPurchaseOptions,
  listUserPendingPaymentOrders,
  listUserPaymentRecharges,
} from '../../../../shared/api/generated-query/user-payments/user-payments'
import type {
  CreatePaymentCheckoutRequest,
  PaymentCheckout,
  PaymentOrderReturnStatus,
  PaymentOrderReturnStatusStatus,
  PaymentPurchaseOptions,
  PendingPaymentOrder,
  PendingPaymentOrderList,
  RechargeHistoryItem,
  RechargeHistoryPage,
} from '../../../../shared/api/generated-query/model'
import { subscribeUserEvent } from '../../../../shared/api/userEventStream'

export type PaymentCheckoutRequest = CreatePaymentCheckoutRequest
export type PaymentCheckoutSession = PaymentCheckout
export type {
  PaymentOrderReturnStatus,
  PaymentPurchaseOptions,
  PendingPaymentOrder,
  PendingPaymentOrderList,
  RechargeHistoryItem,
  RechargeHistoryPage,
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

export const paymentsContextKey: InjectionKey<PaymentsContext> =
  Symbol('payments-context')

const productionResource: PaymentsResource = {
  getPurchaseOptions: (signal) =>
    resolveApiData(
      getUserPaymentPurchaseOptions({ signal }),
      i18n.global.t('account.errors.optionsLoadFailed'),
    ),
  createCheckout: (request) =>
    resolveApiData(
      createUserPaymentCheckoutSession(request),
      i18n.global.t('account.errors.checkoutCreateFailed'),
    ),
  getReturnStatus: (publicToken, signal) =>
    resolveApiData(
      getUserPaymentOrderReturnStatus(publicToken, { signal }),
      i18n.global.t('account.errors.orderStatusLoadFailed'),
    ),
  listPendingOrders: (signal) =>
    resolveApiData(
      listUserPendingPaymentOrders({ signal }),
      i18n.global.t('account.errors.pendingOrdersLoadFailed'),
    ),
  continuePendingOrder: (publicToken) =>
    resolveApiData(
      continueUserPendingPaymentOrder(publicToken),
      i18n.global.t('account.errors.continuePaymentFailed'),
    ),
  abandonPendingOrder: (publicToken) =>
    resolveApiData(
      abandonUserPendingPaymentOrder(publicToken),
      i18n.global.t('account.errors.abandonOrderFailed'),
    ),
  listRecharges: (params, signal) =>
    resolveApiData(
      listUserPaymentRecharges(params, { signal }),
      i18n.global.t('account.errors.historyLoadFailed'),
    ),
  subscribe: (listener) => subscribeToPaymentEvents(listener),
}

const defaultContext: PaymentsContext = {
  resource: productionResource,
  navigateToCheckout: (checkoutURL) => window.location.assign(checkoutURL),
}

export function subscribeToPaymentEvents(
  listener: (event: PaymentEvent) => void,
): () => void {
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
