<template>
  <AdminDialogShell
    :visible="visible"
    :header="
      detail
        ? PAYMENT_ORDERS_MESSAGES.detailHeader(detail.publicOrderNumber)
        : PAYMENT_ORDERS_MESSAGES.detailHeaderFallback
    "
    :description="PAYMENT_ORDERS_MESSAGES.detailDescription"
    size="wide"
    content-layout="scrollable"
    :busy="loading"
    @update:visible="$emit('update:visible', $event)"
  >
    <div v-if="loading" class="grid min-h-56 place-items-center" role="status">
      <span class="text-admin-body text-admin-text-3">{{
        PAYMENT_ORDERS_MESSAGES.detailLoading
      }}</span>
    </div>

    <AdminAlert v-else-if="error" severity="error">
      <div class="flex flex-wrap items-center justify-between gap-3">
        <span>{{ PAYMENT_ORDERS_MESSAGES.detailLoadFailed }}</span>
        <AdminButton variant="outline" :class="adminActionButtonClass" @click="$emit('retry')">
          {{ PAYMENT_ORDERS_MESSAGES.retry }}
        </AdminButton>
      </div>
    </AdminAlert>

    <div v-else-if="detail" class="flex min-w-0 flex-col gap-5">
      <section class="rounded-ib-control border border-admin-border-soft p-4">
        <div class="flex flex-wrap items-center justify-between gap-3">
          <div class="min-w-0">
            <div class="font-mono text-admin-content font-bold text-admin-title-color">
              {{ detail.publicOrderNumber }}
            </div>
            <div class="mt-1 text-admin-meta text-admin-text-3">{{ detail.user.displayName }}</div>
          </div>
          <AdminBadge :severity="getPaymentOrderStatusPresentation(detail.status).severity">
            {{ getPaymentOrderStatusPresentation(detail.status).label }}
          </AdminBadge>
        </div>
        <dl class="mt-4 grid grid-cols-1 gap-x-5 gap-y-3 sm:grid-cols-2">
          <div v-for="row in overviewRows" :key="row.label" class="min-w-0">
            <dt class="text-admin-caption font-[650] text-admin-text-3">{{ row.label }}</dt>
            <dd class="mt-0.5 break-words text-admin-body font-semibold text-admin-text-1">
              {{ row.value }}
            </dd>
          </div>
        </dl>
      </section>

      <section aria-labelledby="payment-stripe-identities">
        <h3 id="payment-stripe-identities" class="text-admin-body font-bold text-admin-title-color">
          {{ PAYMENT_ORDERS_MESSAGES.stripeSummaryTitle }}
        </h3>
        <dl class="mt-3 grid grid-cols-1 gap-x-5 gap-y-3 sm:grid-cols-2">
          <div v-for="row in stripeRows" :key="row.label" class="min-w-0">
            <dt class="text-admin-caption font-[650] text-admin-text-3">{{ row.label }}</dt>
            <dd class="mt-0.5 break-all font-mono text-admin-meta text-admin-text-1">
              {{ row.value }}
            </dd>
          </div>
        </dl>
      </section>

      <section aria-labelledby="payment-reconciliation">
        <div class="flex flex-wrap items-center gap-2">
          <h3 id="payment-reconciliation" class="text-admin-body font-bold text-admin-title-color">
            {{ PAYMENT_ORDERS_MESSAGES.reconciliationTitle }}
          </h3>
          <AdminBadge :severity="detail.reconciliation.anomalous ? 'danger' : 'secondary'">
            {{ PAYMENT_ORDERS_MESSAGES.anomalyState(detail.reconciliation.anomalous) }}
          </AdminBadge>
        </div>
        <dl class="mt-3 grid grid-cols-1 gap-x-5 gap-y-3 sm:grid-cols-2">
          <div v-for="row in reconciliationRows" :key="row.label" class="min-w-0">
            <dt class="text-admin-caption font-[650] text-admin-text-3">{{ row.label }}</dt>
            <dd class="mt-0.5 break-words text-admin-body text-admin-text-1">{{ row.value }}</dd>
          </div>
        </dl>
      </section>

      <section class="grid grid-cols-1 gap-5 md:grid-cols-2">
        <div aria-labelledby="payment-refunds">
          <h3 id="payment-refunds" class="text-admin-body font-bold text-admin-title-color">
            {{ PAYMENT_ORDERS_MESSAGES.refundsTitle }}
          </h3>
          <p v-if="detail.refunds.length === 0" class="mt-2 text-admin-meta text-admin-text-3">
            {{ PAYMENT_ORDERS_MESSAGES.refundsEmpty }}
          </p>
          <ul v-else class="mt-2 flex flex-col gap-2">
            <li
              v-for="(refund, index) in detail.refunds"
              :key="refund.id || index"
              class="rounded-ib-control border border-admin-border-soft p-3 text-admin-meta"
            >
              <div class="font-bold text-admin-text-1">{{ refundStatusLabel(refund.status) }}</div>
              <div v-if="refund.stripeStatus" class="mt-1 text-admin-text-3">
                Stripe: {{ refund.stripeStatus }}
              </div>
              <div class="mt-1 text-admin-text-3">
                {{ formatRefund(refund) }} · {{ formatPaymentDateTime(refund.updatedAt) }}
              </div>
              <div v-if="refund.errorSummary" class="mt-1 break-words text-admin-danger">
                {{ refund.errorSummary }}
              </div>
            </li>
          </ul>
        </div>

        <div aria-labelledby="payment-disputes">
          <h3 id="payment-disputes" class="text-admin-body font-bold text-admin-title-color">
            Dispute
          </h3>
          <p v-if="detail.disputes.length === 0" class="mt-2 text-admin-meta text-admin-text-3">
            {{ PAYMENT_ORDERS_MESSAGES.disputesEmpty }}
          </p>
          <ul v-else class="mt-2 flex flex-col gap-2">
            <li
              v-for="(dispute, index) in detail.disputes"
              :key="dispute.id || index"
              class="rounded-ib-control border border-admin-border-soft p-3 text-admin-meta"
            >
              <div class="flex flex-wrap items-center justify-between gap-2">
                <div class="font-bold text-admin-text-1">{{ dispute.status }}</div>
                <a
                  v-if="dispute.stripeDashboardUrl"
                  :href="dispute.stripeDashboardUrl"
                  target="_blank"
                  rel="noopener noreferrer"
                  class="font-bold text-admin-accent hover:underline focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2"
                >
                  Stripe Dashboard
                </a>
              </div>
              <div class="mt-1 text-admin-text-3">{{ formatDispute(dispute) }}</div>
            </li>
          </ul>
        </div>
      </section>

      <section
        v-if="canRefund"
        aria-labelledby="payment-refund-action"
        class="rounded-ib-control border border-admin-border-soft p-4"
      >
        <div class="flex flex-wrap items-start justify-between gap-3">
          <div>
            <h3 id="payment-refund-action" class="text-admin-body font-bold text-admin-title-color">
              {{ PAYMENT_ORDERS_MESSAGES.refundActionTitle }}
            </h3>
          </div>
          <AdminBadge severity="info">
            {{ PAYMENT_ORDERS_MESSAGES.refundableBadge(formatIdeas(refundableIdeas)) }}
          </AdminBadge>
        </div>
        <form class="mt-3 flex flex-wrap items-end gap-3" novalidate @submit.prevent="submitRefund">
          <label
            for="admin-payment-refund-ideas"
            class="flex min-w-44 flex-1 flex-col gap-1.5 text-admin-meta font-[650] text-admin-text-2"
          >
            <span>{{ PAYMENT_ORDERS_MESSAGES.refundQuantityLabel }}</span>
            <AdminNumberInput
              id="admin-payment-refund-ideas"
              v-model="refundIdeaQuantity"
              :aria-label="PAYMENT_ORDERS_MESSAGES.refundQuantityLabel"
              :min="1"
              :max="refundableIdeas"
              :disabled="refunding"
              :use-grouping="true"
              :invalid="Boolean(refundValidationError)"
            />
            <small v-if="refundValidationError" class="text-admin-danger">{{
              refundValidationError
            }}</small>
          </label>
          <AdminButton
            type="submit"
            :class="adminActionButtonClass"
            variant="destructive"
            :loading="refunding"
            :disabled="refunding || !canSubmitRefund"
          >
            {{ PAYMENT_ORDERS_MESSAGES.refundSubmit }}
          </AdminButton>
        </form>
        <AdminAlert v-if="refundError" class="mt-3" severity="error">
          {{ refundError }}
        </AdminAlert>
      </section>

      <div v-if="detail.receiptUrl" class="flex justify-end">
        <AdminButton
          variant="outline"
          :href="detail.receiptUrl"
          target="_blank"
          rel="noopener noreferrer"
        >
          {{ PAYMENT_ORDERS_MESSAGES.receiptLink }}
        </AdminButton>
      </div>
    </div>
  </AdminDialogShell>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import AdminAlert from '../../shared/components/AdminAlert.vue'
import AdminBadge from '../../shared/components/AdminBadge.vue'
import AdminButton from '../../shared/components/AdminButton.vue'
import AdminNumberInput from '../../shared/components/AdminNumberInput.vue'
import AdminDialogShell from '../../shared/components/AdminDialogShell.vue'
import { adminActionButtonClass } from '../../shared/lib/uiClasses'
import {
  formatPaymentDateTime,
  formatPaymentMoney,
  getCreditStatusLabel,
  getPaymentOrderStatusPresentation,
  getStripeStatusLabel,
} from './paymentOrderPresentation'
import { formatLocaleNumber } from '../../../../shared/i18n/formatters'
import { PAYMENT_ORDERS_MESSAGES } from './messages'
import type {
  AdminPaymentDisputeSummary,
  AdminPaymentOrderDetail,
  AdminPaymentRefundSummary,
} from './paymentOrderTypes'

const props = defineProps<{
  visible: boolean
  detail: AdminPaymentOrderDetail | null
  loading: boolean
  error: boolean
  refunding: boolean
  refundError: string
}>()

const emit = defineEmits<{
  'update:visible': [visible: boolean]
  retry: []
  refund: [ideaQuantity: number]
}>()

const refundIdeaQuantity = ref<number | null>(null)
watch(
  () => props.detail?.publicOrderNumber,
  () => {
    refundIdeaQuantity.value = null
  },
)

const refundableIdeas = computed(() => {
  const value = props.detail?.refundableIdeaQuantity
  if (value !== undefined) return Math.max(0, Math.floor(value))
  const detail = props.detail
  if (!detail) return 0
  const refunded = detail.refunds.reduce((sum, item) => sum + (item.ideaQuantity ?? 0), 0)
  return Math.max(0, detail.ideaQuantity - refunded)
})
const canRefund = computed(() => {
  const detail = props.detail
  if (!detail?.paidAt || detail.status !== 'credited') return false
  const paidTime = Date.parse(detail.paidAt)
  return (
    Number.isFinite(paidTime) &&
    Date.now() - paidTime <= 14 * 24 * 60 * 60 * 1000 &&
    refundableIdeas.value > 0
  )
})
const refundValidationError = computed(() => {
  const quantity = refundIdeaQuantity.value
  if (quantity === null || quantity === undefined || !Number.isInteger(quantity) || quantity <= 0) {
    return PAYMENT_ORDERS_MESSAGES.refundQuantityInvalid
  }
  if (quantity > refundableIdeas.value)
    return PAYMENT_ORDERS_MESSAGES.refundQuantityExceeded(formatIdeas(refundableIdeas.value))
  return ''
})
const canSubmitRefund = computed(() => canRefund.value && !refundValidationError.value)

const overviewRows = computed(() => {
  const detail = props.detail
  if (!detail) return []
  return [
    {
      label: PAYMENT_ORDERS_MESSAGES.overviewIdeaLabel,
      value: formatLocaleNumber(detail.ideaQuantity),
    },
    {
      label: PAYMENT_ORDERS_MESSAGES.overviewPaidTotalLabel,
      value: formatPaymentMoney(detail.paidTotalMinor, detail.paidCurrency),
    },
    {
      label: PAYMENT_ORDERS_MESSAGES.overviewSubtotalLabel,
      value: formatPaymentMoney(detail.paidSubtotalMinor, detail.paidCurrency),
    },
    {
      label: PAYMENT_ORDERS_MESSAGES.overviewTaxLabel,
      value: formatPaymentMoney(detail.paidTaxMinor, detail.paidCurrency),
    },
    {
      label: PAYMENT_ORDERS_MESSAGES.overviewBaseAmountLabel,
      value: formatPaymentMoney(detail.baseAmountMinor, detail.baseCurrency),
    },
    {
      label: PAYMENT_ORDERS_MESSAGES.overviewStripeStatusLabel,
      value: getStripeStatusLabel(detail.stripeStatus),
    },
    {
      label: PAYMENT_ORDERS_MESSAGES.overviewCreditStatusLabel,
      value: getCreditStatusLabel(detail.creditStatus),
    },
    { label: PAYMENT_ORDERS_MESSAGES.overviewTermsLabel, value: detail.termsVersion || '—' },
    {
      label: PAYMENT_ORDERS_MESSAGES.overviewRefundPolicyLabel,
      value: detail.refundPolicyVersion || '—',
    },
    {
      label: PAYMENT_ORDERS_MESSAGES.overviewCreatedLabel,
      value: formatPaymentDateTime(detail.createdAt),
    },
    {
      label: PAYMENT_ORDERS_MESSAGES.overviewPaidLabel,
      value: formatPaymentDateTime(detail.paidAt),
    },
    {
      label: PAYMENT_ORDERS_MESSAGES.overviewCreditedLabel,
      value: formatPaymentDateTime(detail.creditedAt),
    },
    {
      label: PAYMENT_ORDERS_MESSAGES.overviewExpiresLabel,
      value: formatPaymentDateTime(detail.expiresAt),
    },
    {
      label: PAYMENT_ORDERS_MESSAGES.overviewUpdatedLabel,
      value: formatPaymentDateTime(detail.updatedAt),
    },
  ]
})

const stripeRows = computed(() => {
  const stripe = props.detail?.stripe
  if (!stripe) return []
  return [
    { label: 'Account', value: stripe.accountId || '—' },
    { label: 'Customer', value: stripe.customerId || '—' },
    { label: 'Checkout Session', value: stripe.checkoutSessionId || '—' },
    { label: 'Payment Intent', value: stripe.paymentIntentId || '—' },
    { label: 'Charge', value: stripe.chargeId || '—' },
  ]
})

const reconciliationRows = computed(() => {
  const reconciliation = props.detail?.reconciliation
  if (!reconciliation) return []
  return [
    {
      label: PAYMENT_ORDERS_MESSAGES.reconciliationAnomalousLabel,
      value: formatPaymentDateTime(reconciliation.anomalousAt),
    },
    {
      label: PAYMENT_ORDERS_MESSAGES.reconciliationCheckedLabel,
      value: formatPaymentDateTime(reconciliation.lastCheckedAt),
    },
    {
      label: PAYMENT_ORDERS_MESSAGES.reconciliationNextLabel,
      value: formatPaymentDateTime(reconciliation.nextAttemptAt),
    },
    {
      label: PAYMENT_ORDERS_MESSAGES.reconciliationErrorCodeLabel,
      value: reconciliation.lastErrorCode || '—',
    },
    {
      label: PAYMENT_ORDERS_MESSAGES.reconciliationErrorSummaryLabel,
      value: reconciliation.lastErrorSummary || '—',
    },
  ]
})

function formatRefund(refund: AdminPaymentRefundSummary): string {
  const ideas =
    refund.ideaQuantity === undefined
      ? PAYMENT_ORDERS_MESSAGES.refundUnknownQuantity
      : PAYMENT_ORDERS_MESSAGES.refundQuantityIdeas(formatLocaleNumber(refund.ideaQuantity))
  return PAYMENT_ORDERS_MESSAGES.refundSummary(
    ideas,
    formatPaymentMoney(refund.amountMinor, refund.currency),
  )
}

function refundStatusLabel(status: string): string {
  const labels: Record<string, string> = {
    pending: '退款处理中',
    succeeded: '退款成功',
    failed: '退款失败',
    anomaly: '需要人工处理',
    'refund.updated': 'Stripe 退款更新',
    'refund.failed': 'Stripe 退款失败',
  }
  return labels[status] ?? status
}

function formatIdeas(value: number): string {
  return formatLocaleNumber(value)
}

function submitRefund() {
  if (!canSubmitRefund.value || refundIdeaQuantity.value === null) return
  emit('refund', refundIdeaQuantity.value)
}

function formatDispute(dispute: AdminPaymentDisputeSummary): string {
  const amount = formatPaymentMoney(dispute.amountMinor, dispute.currency)
  const ratio =
    dispute.ratioBasisPoints !== undefined
      ? PAYMENT_ORDERS_MESSAGES.disputeRatio(
          formatLocaleNumber(dispute.ratioBasisPoints / 100, { maximumFractionDigits: 2 }),
        )
      : ''
  const target = formatIdeas(dispute.targetIdeaQuantity ?? 0)
  const debited = formatIdeas(dispute.debitedIdeaQuantity ?? 0)
  const unrecovered = formatIdeas(dispute.unrecoveredCredits ?? 0)
  const provider = dispute.providerStatus
    ? PAYMENT_ORDERS_MESSAGES.disputeStripeProvider(dispute.providerStatus)
    : ''
  const opened = PAYMENT_ORDERS_MESSAGES.disputeOpened(formatPaymentDateTime(dispute.openedAt))
  const closed = dispute.closedAt
    ? PAYMENT_ORDERS_MESSAGES.disputeClosed(formatPaymentDateTime(dispute.closedAt))
    : ''
  return PAYMENT_ORDERS_MESSAGES.disputeSummary(
    amount,
    ratio,
    target,
    debited,
    unrecovered,
    provider,
    opened,
    closed,
  )
}
</script>
