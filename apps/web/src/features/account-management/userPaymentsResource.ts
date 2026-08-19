import { inject, type InjectionKey } from 'vue'
import { resolveApiData } from '../../shared/api/apiResponse'
import {
  abandonUserPendingPaymentOrder,
  continueUserPendingPaymentOrder,
  createUserPaymentCheckoutSession,
  getUserPaymentOrderReturnStatus,
  getUserPaymentPurchaseOptions,
  listUserPendingPaymentOrders,
  listUserPaymentRecharges,
} from '../../shared/api/generated-query/user-payments/user-payments'
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
} from '../../shared/api/generated-query/model'
import { resolveApiUrl } from '../../shared/config/runtimeUrls'

export type UserPaymentCheckoutRequest = CreatePaymentCheckoutRequest
export type UserPaymentCheckout = PaymentCheckout
export type UserPaymentOrderReturnStatus = PaymentOrderReturnStatus
export type UserPaymentOrderStatus = PaymentOrderReturnStatusStatus
export type UserPaymentPurchaseOptions = PaymentPurchaseOptions
export type UserPendingPaymentOrder = PendingPaymentOrder
export type UserPendingPaymentOrderList = PendingPaymentOrderList
export type UserRechargeHistoryItem = RechargeHistoryItem
export type UserRechargeHistoryPage = RechargeHistoryPage

export type UserPaymentEvent = {
  public_token?: string
  status: UserPaymentOrderStatus | 'refund_updated' | 'dispute_updated'
}

export interface UserPaymentsResource {
  getPurchaseOptions(signal?: AbortSignal): Promise<UserPaymentPurchaseOptions>
  createCheckout(request: UserPaymentCheckoutRequest): Promise<UserPaymentCheckout>
  getReturnStatus(publicToken: string, signal?: AbortSignal): Promise<UserPaymentOrderReturnStatus>
  listPendingOrders(signal?: AbortSignal): Promise<UserPendingPaymentOrderList>
  continuePendingOrder(publicToken: string): Promise<UserPendingPaymentOrder>
  abandonPendingOrder(publicToken: string): Promise<UserPendingPaymentOrder>
  listRecharges(
    params: { cursor?: string; limit: number },
    signal?: AbortSignal,
  ): Promise<UserRechargeHistoryPage>
  subscribe(listener: (event: UserPaymentEvent) => void): () => void
}

export type UserPaymentsContext = {
  resource: UserPaymentsResource
  navigateToCheckout: (checkoutURL: string) => void
}

export const userPaymentsContextKey: InjectionKey<UserPaymentsContext> =
  Symbol('user-payments-context')

const productionResource: UserPaymentsResource = {
  getPurchaseOptions: (signal) =>
    resolveApiData(getUserPaymentPurchaseOptions({ signal }), '加载充值选项失败'),
  createCheckout: (request) =>
    resolveApiData(createUserPaymentCheckoutSession(request), '创建支付页面失败'),
  getReturnStatus: (publicToken, signal) =>
    resolveApiData(getUserPaymentOrderReturnStatus(publicToken, { signal }), '加载订单状态失败'),
  listPendingOrders: (signal) =>
    resolveApiData(listUserPendingPaymentOrders({ signal }), '加载待支付订单失败'),
  continuePendingOrder: (publicToken) =>
    resolveApiData(continueUserPendingPaymentOrder(publicToken), '继续支付失败'),
  abandonPendingOrder: (publicToken) =>
    resolveApiData(abandonUserPendingPaymentOrder(publicToken), '放弃待支付订单失败'),
  listRecharges: (params, signal) =>
    resolveApiData(listUserPaymentRecharges(params, { signal }), '加载充值记录失败'),
  subscribe: (listener) => subscribeToUserPaymentEvents(listener),
}

const defaultContext: UserPaymentsContext = {
  resource: productionResource,
  navigateToCheckout: (checkoutURL) => window.location.assign(checkoutURL),
}

type UserPaymentEventSource = {
  addEventListener(event: 'payment', listener: EventListener): void
  removeEventListener(event: 'payment', listener: EventListener): void
  close(): void
}
type UserPaymentEventSourceFactory = (
  url: string,
  options: EventSourceInit,
) => UserPaymentEventSource

export function subscribeToUserPaymentEvents(
  listener: (event: UserPaymentEvent) => void,
  createEventSource: UserPaymentEventSourceFactory = (url, options) =>
    new EventSource(url, options),
): () => void {
  if (typeof EventSource === 'undefined' && arguments.length < 2) return () => undefined
  const source = createEventSource(resolveApiUrl('/user/events'), {
    withCredentials: true,
  })
  const handlePayment = (message: MessageEvent<string>) => {
    const event = parseUserPaymentEvent(message.data)
    if (event) listener(event)
  }
  source.addEventListener('payment', handlePayment as EventListener)
  return () => {
    source.removeEventListener('payment', handlePayment as EventListener)
    source.close()
  }
}

export function parseUserPaymentEvent(raw: string): UserPaymentEvent | null {
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

export function useUserPaymentsContext(): UserPaymentsContext {
  return inject(userPaymentsContextKey, defaultContext)
}

export const userPaymentsQueryKeys = {
  all: ['user', 'payments'] as const,
  options: ['user', 'payments', 'purchase-options'] as const,
  pending: ['user', 'payments', 'pending-orders'] as const,
  history: ['user', 'payments', 'recharges'] as const,
  returnStatus: (publicToken: string) =>
    ['user', 'payments', 'orders', 'return', publicToken] as const,
}
