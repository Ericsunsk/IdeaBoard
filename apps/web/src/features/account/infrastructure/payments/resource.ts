import { inject, type InjectionKey } from 'vue'
import {
  abandonWorkspacePendingOrder as abandonWorkspacePendingOrderTransport,
  continueWorkspacePendingOrder as continueWorkspacePendingOrderTransport,
  createWorkspaceCheckoutSession,
  getWorkspaceCheckoutSession,
  getWorkspacePurchaseOptions,
  listWorkspacePendingOrders,
  listWorkspaceRecharges,
} from '../../../../shared/api/generated-query/public/workspace-billing/workspace-billing'
import type {
  WorkspaceCheckoutSession,
  WorkspacePendingOrder,
  WorkspacePendingOrderList,
  WorkspacePurchaseOptions,
  WorkspaceRechargeHistoryItem,
} from '../../../../shared/api/generated-query/public/model'
import { buildContractRequestHeaders, newRequestId } from '../../../../shared/api/contractKernel'
import { workspaceBillingPendingOrderIdempotencyKey } from '../../../../shared/api/idempotencyKey'
import { i18n } from '../../../../shared/i18n'
import { subscribeWorkspaceEvent } from '../../../../shared/api/workspaceEventStream'
import {
  readContractListBody,
  requireAccountWorkspaceId,
  resolveWorkspaceBillingData,
  asWorkspaceBillingError,
} from '../workspaceBilling'

// User payments boundary (W2 transports). Checkout creation, return
// status, purchase options, pending orders (list/continue/abandon), and
// recharge history moved to the Public Data Plane (`/api/v1/workspaces/
// {id}/...`): the v1 `session_id` is the legacy order `public_token`
// (same value, same settlement source), and the v1 pending-order
// `public_token` carries the same value, so view types keep their
// `public_token` names. Settlement notification rides the workspace
// `billing.settled` event. The deleted `/api/user/payments/*` routes
// are gone; this module is the single transport.
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
  getPurchaseOptions(workspaceId: string, signal?: AbortSignal): Promise<PaymentPurchaseOptions>
  createCheckout(workspaceId: string, request: PaymentCheckoutRequest): Promise<PaymentCheckoutSession>
  getReturnStatus(
    workspaceId: string,
    publicToken: string,
    signal?: AbortSignal,
  ): Promise<PaymentOrderReturnStatus>
  listPendingOrders(workspaceId: string, signal?: AbortSignal): Promise<PendingPaymentOrderList>
  continuePendingOrder(workspaceId: string, publicToken: string): Promise<PendingPaymentOrder>
  abandonPendingOrder(workspaceId: string, publicToken: string): Promise<PendingPaymentOrder>
  listRecharges(
    workspaceId: string,
    params: { cursor?: string; limit: number },
    signal?: AbortSignal,
  ): Promise<RechargeHistoryPage>
  subscribe(workspaceId: string, listener: (event: PaymentEvent) => void): () => void
}

export type PaymentsContext = {
  resource: PaymentsResource
  navigateToCheckout: (checkoutURL: string) => void
}

export const paymentsContextKey: InjectionKey<PaymentsContext> = Symbol('payments-context')

export const paymentsResource: PaymentsResource = {
  getPurchaseOptions: async (workspaceId, signal) => {
    const id = requireAccountWorkspaceId(workspaceId)
    const options = await resolveWorkspaceBillingData<WorkspacePurchaseOptions>(
      getWorkspacePurchaseOptions(id, { ...(signal == null ? {} : { signal }) }),
      i18n.global.t('account.errors.optionsLoadFailed'),
    )
    return toPurchaseOptions(options)
  },
  createCheckout: async (workspaceId, request) => {
    const id = requireAccountWorkspaceId(workspaceId)
    const session = await resolveWorkspaceBillingData<WorkspaceCheckoutSession>(
      createWorkspaceCheckoutSession(
        id,
        {
          data: {
            idea_quantity: request.idea_quantity,
            terms_accepted: true,
            terms_version: request.terms_version,
            refund_policy_version: request.refund_policy_version,
          },
        },
        {
          headers: buildContractRequestHeaders({
            requestId: newRequestId(),
            idempotencyKey: request.idempotency_key,
          }),
        },
      ),
      i18n.global.t('account.errors.checkoutCreateFailed'),
    )
    return requireCheckoutSession(session)
  },
  getReturnStatus: async (workspaceId, publicToken, signal) => {
    const id = requireAccountWorkspaceId(workspaceId)
    const session = await resolveWorkspaceBillingData<WorkspaceCheckoutSession>(
      getWorkspaceCheckoutSession(id, publicToken, { signal }),
      i18n.global.t('account.errors.orderStatusLoadFailed'),
    )
    return toReturnStatus(session)
  },
  listPendingOrders: async (workspaceId, signal) => {
    const id = requireAccountWorkspaceId(workspaceId)
    const list = await resolveWorkspaceBillingData<WorkspacePendingOrderList>(
      listWorkspacePendingOrders(id, { ...(signal == null ? {} : { signal }) }),
      i18n.global.t('account.errors.pendingOrdersLoadFailed'),
    )
    if (!Array.isArray(list?.items)) throw new Error(i18n.global.t('account.errors.pendingOrdersLoadFailed'))
    return { items: list.items.map(toPendingOrder) }
  },
  continuePendingOrder: async (workspaceId, publicToken) => {
    const id = requireAccountWorkspaceId(workspaceId)
    const order = await resolveWorkspaceBillingData<WorkspacePendingOrder>(
      continueWorkspacePendingOrderTransport(
        id,
        publicToken.trim(),
        {
          headers: buildContractRequestHeaders({
            requestId: newRequestId(),
            idempotencyKey: workspaceBillingPendingOrderIdempotencyKey('continue'),
          }),
        },
      ),
      i18n.global.t('account.errors.continuePaymentFailed'),
    )
    return toPendingOrder(order)
  },
  abandonPendingOrder: async (workspaceId, publicToken) => {
    const id = requireAccountWorkspaceId(workspaceId)
    const order = await resolveWorkspaceBillingData<WorkspacePendingOrder>(
      abandonWorkspacePendingOrderTransport(
        id,
        publicToken.trim(),
        {
          headers: buildContractRequestHeaders({
            requestId: newRequestId(),
            idempotencyKey: workspaceBillingPendingOrderIdempotencyKey('abandon'),
          }),
        },
      ),
      i18n.global.t('account.errors.abandonOrderFailed'),
    )
    return toPendingOrder(order)
  },
  listRecharges: async (workspaceId, params, signal) => {
    const id = requireAccountWorkspaceId(workspaceId)
    const fallback = i18n.global.t('account.errors.historyLoadFailed')
    const query: { cursor?: string; page_size?: number } = {}
    if (params.cursor?.trim()) query.cursor = params.cursor.trim()
    if (params.limit != null) query.page_size = params.limit
    let body: unknown
    try {
      body = await listWorkspaceRecharges(id, query, { ...(signal == null ? {} : { signal }) })
    } catch (error) {
      throw asWorkspaceBillingError(error, fallback)
    }
    const page = readContractListBody<{ data: WorkspaceRechargeHistoryItem[]; meta: { has_more: boolean; next_cursor?: string | null } }>(body, fallback)
    if (!Array.isArray(page?.data) || page?.meta == null) throw new Error(fallback)
    return {
      items: page.data.map(toRechargeItem),
      ...(page.meta.has_more && page.meta.next_cursor ? { next_cursor: page.meta.next_cursor } : {}),
    }
  },
  subscribe: (workspaceId, listener) => subscribeToPaymentEvents(workspaceId, listener),
}

const defaultContext: PaymentsContext = {
  resource: paymentsResource,
  navigateToCheckout: (checkoutURL) => window.location.assign(checkoutURL),
}

function requireCheckoutSession(session: WorkspaceCheckoutSession): PaymentCheckoutSession {
  const fallback = i18n.global.t('account.errors.checkoutCreateFailed')
  if (!session.session_id || !session.checkout_url || !session.expires_at) {
    throw new Error(fallback)
  }
  return {
    checkout_url: session.checkout_url,
    public_token: session.session_id,
    expires_at: session.expires_at,
    reused: session.reused === true,
  }
}

function toPurchaseOptions(options: WorkspacePurchaseOptions): PaymentPurchaseOptions {
  const result: PaymentPurchaseOptions = {
    purchases_available: options.purchases_available,
    quick_idea_quantities: [...options.quick_idea_quantities],
    min_idea_quantity: options.min_idea_quantity,
    max_idea_quantity: options.max_idea_quantity,
    ideas_per_usd: options.ideas_per_usd,
    terms: { version: options.terms.version, url: options.terms.url },
    refund_policy: {
      version: options.refund_policy.version,
      url: options.refund_policy.url,
    },
  }
  if (options.unavailable_reason != null) result.unavailable_reason = options.unavailable_reason
  if (options.restriction_type != null) result.restriction_type = options.restriction_type
  return result
}

function toPendingOrder(order: WorkspacePendingOrder): PendingPaymentOrder {
  const result: PendingPaymentOrder = {
    public_token: order.public_token,
    idea_quantity: order.idea_quantity,
    status: order.status,
    expires_at: order.expires_at,
    updated_at: order.updated_at,
  }
  if (order.checkout_url != null) result.checkout_url = order.checkout_url
  return result
}

function toRechargeItem(item: WorkspaceRechargeHistoryItem): RechargeHistoryItem {
  const result: RechargeHistoryItem = {
    occurred_at: item.occurred_at,
    idea_quantity: item.idea_quantity,
    status: item.status,
  }
  if (item.paid_amount_minor != null) result.paid_amount_minor = item.paid_amount_minor
  if (item.paid_currency != null) result.paid_currency = item.paid_currency
  if (item.refunded_idea_quantity != null) result.refunded_idea_quantity = item.refunded_idea_quantity
  if (item.refunded_amount_minor != null) result.refunded_amount_minor = item.refunded_amount_minor
  if (item.receipt_url != null) result.receipt_url = item.receipt_url
  return result
}

function toReturnStatus(session: WorkspaceCheckoutSession): PaymentOrderReturnStatus {
  const fallback = i18n.global.t('account.errors.orderStatusLoadFailed')
  if (
    session.status !== PaymentOrderReturnStatusStatus.processing &&
    session.status !== PaymentOrderReturnStatusStatus.credited &&
    session.status !== PaymentOrderReturnStatusStatus.failed
  ) {
    throw new Error(fallback)
  }
  return {
    idea_quantity: session.idea_quantity ?? 0,
    status: session.status,
    updated_at: session.expires_at ?? '',
  }
}

export function subscribeToPaymentEvents(
  workspaceId: string,
  listener: (event: PaymentEvent) => void,
): () => void {
  // No selection means no stream: stay unsubscribed (the queries stay
  // disabled with guidance) instead of guessing a workspace.
  const trimmed = workspaceId?.trim() ?? ''
  if (!trimmed) return () => {}
  const id = requireAccountWorkspaceId(trimmed)
  // The v1 stream reports settlement per checkout session; a cursor gap
  // means state may have been missed, so refresh the same way.
  const notifySettled = (sessionId?: string) =>
    listener(sessionId ? { status: 'credited', public_token: sessionId } : { status: 'credited' })
  return subscribeWorkspaceEvent(
    id,
    (envelope) => {
      if (envelope.type !== 'billing.settled') return
      const sessionId =
        envelope.resource.type === 'checkout_session' ? envelope.resource.id : undefined
      notifySettled(sessionId)
    },
    {
      types: ['billing.settled'],
      onGap: () => notifySettled(),
    },
  )
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
  all: ['workspace', 'payments'] as const,
  options: (workspaceId: string) => ['workspace', 'payments', 'purchase-options', workspaceId] as const,
  pending: (workspaceId: string) => ['workspace', 'payments', 'pending-orders', workspaceId] as const,
  history: (workspaceId: string) => ['workspace', 'payments', 'recharges', workspaceId] as const,
  returnStatus: (workspaceId: string, publicToken: string) =>
    ['workspace', 'payments', 'orders', 'return', workspaceId, publicToken] as const,
}
