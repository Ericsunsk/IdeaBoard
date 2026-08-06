export const adminPaymentOrderStatuses = [
  'creating',
  'checkout_open',
  'async_payment_pending',
  'paid_pending_credit',
  'credited',
  'cancelled',
  'expired',
  'payment_failed',
] as const

export type AdminPaymentOrderStatus = (typeof adminPaymentOrderStatuses)[number]

export type AdminPaymentOrderUser = {
  publicId: string
  displayName: string
}

export type AdminPaymentOrderSummary = {
  publicOrderNumber: string
  user: AdminPaymentOrderUser
  ideaQuantity: number
  baseAmountMinor: number
  baseCurrency: string
  paidAmountMinor?: number
  paidCurrency?: string
  status: AdminPaymentOrderStatus
  stripeStatus: string
  creditStatus: string
  createdAt: string
  paidAt?: string
  creditedAt?: string
  lastCheckedAt?: string
  anomalousAt?: string
}

export type AdminPaymentOrderPage = {
  orders: AdminPaymentOrderSummary[]
  total: number
  nextCursor?: string
}

export type AdminPaymentStripeSummary = {
  accountId?: string
  customerId?: string
  checkoutSessionId?: string
  paymentIntentId?: string
  chargeId?: string
}

export type AdminPaymentReconciliation = {
  anomalous: boolean
  anomalousAt?: string
  lastCheckedAt?: string
  nextAttemptAt?: string
  lastErrorCode?: string
  lastErrorSummary?: string
}

export type AdminPaymentRefundSummary = {
  id?: string
  status: string
  ideaQuantity?: number
  amountMinor?: number
  currency?: string
  stripeStatus?: string
  updatedAt?: string
  errorSummary?: string
}

export type AdminPaymentDisputeSummary = {
  id?: string
  status: string
  providerStatus?: string
  amountMinor?: number
  currency?: string
  ratioBasisPoints?: number
  targetIdeaQuantity?: number
  debitedIdeaQuantity?: number
  unrecoveredCredits?: number
  stripeDashboardUrl?: string
  openedAt?: string
  closedAt?: string
  updatedAt?: string
}

export type AdminPaymentOrderDetail = {
  publicOrderNumber: string
  user: AdminPaymentOrderUser
  ideaQuantity: number
  baseAmountMinor: number
  baseCurrency: string
  paidSubtotalMinor?: number
  paidTaxMinor?: number
  paidTotalMinor?: number
  paidCurrency?: string
  status: AdminPaymentOrderStatus
  stripeStatus: string
  creditStatus: string
  stripe: AdminPaymentStripeSummary
  termsVersion: string
  refundPolicyVersion: string
  receiptUrl?: string
  expiresAt: string
  createdAt: string
  updatedAt: string
  paidAt?: string
  creditedAt?: string
  reconciliation: AdminPaymentReconciliation
  refunds: AdminPaymentRefundSummary[]
  disputes: AdminPaymentDisputeSummary[]
  refundableIdeaQuantity?: number
}

export type SearchAdminPaymentOrdersInput = {
  orderNumber?: string
  user?: string
  status?: AdminPaymentOrderStatus
  anomalyOnly?: boolean
  cursor?: string
  limit?: number
  signal?: AbortSignal
}

export type AdminPaymentOrderResource = {
  search: (input: SearchAdminPaymentOrdersInput) => Promise<AdminPaymentOrderPage>
  get: (publicOrderNumber: string, signal?: AbortSignal) => Promise<AdminPaymentOrderDetail>
  refund: (
    publicOrderNumber: string,
    input: { ideaQuantity: number; idempotencyKey: string },
  ) => Promise<AdminPaymentOrderDetail>
}
