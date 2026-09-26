export type AdminPaymentOrderStatus =
  | 'creating'
  | 'checkout_open'
  | 'async_payment_pending'
  | 'paid_pending_credit'
  | 'credited'
  | 'cancelled'
  | 'expired'
  | 'payment_failed'

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
  paidAmountMinor?: number | undefined
  paidCurrency?: string | undefined
  status: AdminPaymentOrderStatus
  stripeStatus: string
  creditStatus: string
  createdAt: string
  paidAt?: string | undefined
  creditedAt?: string | undefined
  lastCheckedAt?: string | undefined
  anomalousAt?: string | undefined
}

export type AdminPaymentOrderPage = {
  orders: AdminPaymentOrderSummary[]
  total: number
  nextCursor?: string | undefined
}

export type AdminPaymentStripeSummary = {
  accountId?: string | undefined
  customerId?: string | undefined
  checkoutSessionId?: string | undefined
  paymentIntentId?: string | undefined
  chargeId?: string | undefined
}

export type AdminPaymentReconciliation = {
  anomalous: boolean
  anomalousAt?: string | undefined
  lastCheckedAt?: string | undefined
  nextAttemptAt?: string | undefined
  lastErrorCode?: string | undefined
  lastErrorSummary?: string | undefined
}

export type AdminPaymentRefundSummary = {
  id?: string | undefined
  status: string
  ideaQuantity?: number | undefined
  amountMinor?: number | undefined
  currency?: string | undefined
  stripeStatus?: string | undefined
  updatedAt?: string | undefined
  errorSummary?: string | undefined
}

export type AdminPaymentDisputeSummary = {
  id?: string | undefined
  status: string
  providerStatus?: string | undefined
  amountMinor?: number | undefined
  currency?: string | undefined
  ratioBasisPoints?: number | undefined
  targetIdeaQuantity?: number | undefined
  debitedIdeaQuantity?: number | undefined
  unrecoveredCredits?: number | undefined
  stripeDashboardUrl?: string | undefined
  openedAt?: string | undefined
  closedAt?: string | undefined
  updatedAt?: string | undefined
}

export type AdminPaymentOrderDetail = {
  publicOrderNumber: string
  user: AdminPaymentOrderUser
  ideaQuantity: number
  baseAmountMinor: number
  baseCurrency: string
  paidSubtotalMinor?: number | undefined
  paidTaxMinor?: number | undefined
  paidTotalMinor?: number | undefined
  paidCurrency?: string | undefined
  status: AdminPaymentOrderStatus
  stripeStatus: string
  creditStatus: string
  stripe: AdminPaymentStripeSummary
  termsVersion: string
  refundPolicyVersion: string
  receiptUrl?: string | undefined
  expiresAt: string
  createdAt: string
  updatedAt: string
  paidAt?: string | undefined
  creditedAt?: string | undefined
  reconciliation: AdminPaymentReconciliation
  refunds: AdminPaymentRefundSummary[]
  disputes: AdminPaymentDisputeSummary[]
  refundableIdeaQuantity?: number | undefined
}

export type SearchAdminPaymentOrdersInput = {
  orderNumber?: string | undefined
  user?: string | undefined
  status?: AdminPaymentOrderStatus | undefined
  anomalyOnly?: boolean
  cursor?: string | undefined
  limit?: number | undefined
  signal?: AbortSignal
}

export type AdminPaymentOrderResource = {
  search: (input: SearchAdminPaymentOrdersInput) => Promise<AdminPaymentOrderPage>
  get: (publicOrderNumber: string, signal?: AbortSignal) => Promise<AdminPaymentOrderDetail>
  refund: (
    publicOrderNumber: string,
    input: { ideaQuantity: number; idempotencyKey: string; operatorReason: string },
  ) => Promise<AdminPaymentOrderDetail>
}
