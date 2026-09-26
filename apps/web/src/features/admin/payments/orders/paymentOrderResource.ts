import {
  createAdminPaymentOrderApiAdapter,
  type AdminPaymentOrderApiAdapter,
  type PaymentOrderApiDetail,
  type PaymentOrderApiDispute,
  type PaymentOrderApiRefund,
  type PaymentOrderApiSummary,
} from './paymentOrderApiAdapter'
import type {
  AdminPaymentDisputeSummary,
  AdminPaymentOrderDetail,
  AdminPaymentOrderPage,
  AdminPaymentOrderResource,
  AdminPaymentOrderSummary,
  AdminPaymentRefundSummary,
} from './paymentOrderTypes'

export function createAdminPaymentOrderResource(
  adapter: AdminPaymentOrderApiAdapter = createAdminPaymentOrderApiAdapter(),
): AdminPaymentOrderResource {
  return {
    search: async (input) => {
      const page = await adapter.search(input)
      return {
        orders: page.orders.map(toPaymentOrderSummary),
        total: page.total,
        nextCursor: page.next_cursor,
      } satisfies AdminPaymentOrderPage
    },
    get: async (publicOrderNumber, signal) =>
      toPaymentOrderDetail(await adapter.get(publicOrderNumber, signal)),
    refund: async (publicOrderNumber, input) => {
      await adapter.refund(publicOrderNumber, {
        idea_quantity: input.ideaQuantity,
        idempotency_key: input.idempotencyKey,
        operatorReason: input.operatorReason,
      })
      return toPaymentOrderDetail(await adapter.get(publicOrderNumber))
    },
  }
}

function toPaymentOrderSummary(value: PaymentOrderApiSummary): AdminPaymentOrderSummary {
  return {
    publicOrderNumber: value.public_order_number,
    user: { publicId: value.user.public_id, displayName: value.user.display_name },
    ideaQuantity: value.idea_quantity,
    baseAmountMinor: value.base_amount_minor,
    baseCurrency: value.base_currency,
    paidAmountMinor: value.paid_amount_minor,
    paidCurrency: value.paid_currency,
    status: value.status,
    stripeStatus: value.stripe_status,
    creditStatus: value.credit_status,
    createdAt: value.created_at,
    paidAt: value.paid_at,
    creditedAt: value.credited_at,
    lastCheckedAt: value.last_checked_at,
    anomalousAt: value.anomalous_at,
  }
}

function toPaymentOrderDetail(value: PaymentOrderApiDetail): AdminPaymentOrderDetail {
  return {
    publicOrderNumber: value.public_order_number,
    user: { publicId: value.user.public_id, displayName: value.user.display_name },
    ideaQuantity: value.idea_quantity,
    baseAmountMinor: value.base_amount_minor,
    baseCurrency: value.base_currency,
    paidSubtotalMinor: value.paid_subtotal_minor,
    paidTaxMinor: value.paid_tax_minor,
    paidTotalMinor: value.paid_total_minor,
    paidCurrency: value.paid_currency,
    status: value.status,
    stripeStatus: value.stripe_status,
    creditStatus: value.credit_status,
    stripe: {
      accountId: value.stripe.account_id,
      customerId: value.stripe.customer_id,
      checkoutSessionId: value.stripe.checkout_session_id,
      paymentIntentId: value.stripe.payment_intent_id,
      chargeId: value.stripe.charge_id,
    },
    termsVersion: value.terms_version,
    refundPolicyVersion: value.refund_policy_version,
    receiptUrl: value.receipt_url,
    expiresAt: value.expires_at,
    createdAt: value.created_at,
    updatedAt: value.updated_at,
    paidAt: value.paid_at,
    creditedAt: value.credited_at,
    reconciliation: {
      anomalous: value.reconciliation.anomalous,
      anomalousAt: value.reconciliation.anomalous_at,
      lastCheckedAt: value.reconciliation.last_checked_at,
      nextAttemptAt: value.reconciliation.next_attempt_at,
      lastErrorCode: value.reconciliation.last_error_code,
      lastErrorSummary: value.reconciliation.last_error_summary,
    },
    refunds: (value.refunds ?? []).map(toRefundSummary),
    disputes: (value.disputes ?? []).map(toDisputeSummary),
    refundableIdeaQuantity: value.refundable_idea_quantity,
  }
}

function toRefundSummary(value: PaymentOrderApiRefund): AdminPaymentRefundSummary {
  return {
    id: value.id,
    status: value.status,
    ideaQuantity: value.idea_quantity,
    amountMinor: value.amount_minor,
    currency: value.currency,
    stripeStatus: value.stripe_status,
    updatedAt: value.updated_at,
    errorSummary: value.error_summary,
  }
}

function toDisputeSummary(value: PaymentOrderApiDispute): AdminPaymentDisputeSummary {
  return {
    id: value.id,
    status: value.status,
    providerStatus: value.provider_status,
    amountMinor: value.amount_minor,
    currency: value.currency,
    ratioBasisPoints: value.ratio_basis_points,
    targetIdeaQuantity: value.target_idea_quantity,
    debitedIdeaQuantity: value.debited_idea_quantity,
    unrecoveredCredits: value.unrecovered_credits,
    stripeDashboardUrl: value.stripe_dashboard_url,
    openedAt: value.opened_at,
    closedAt: value.closed_at,
    updatedAt: value.updated_at,
  }
}
