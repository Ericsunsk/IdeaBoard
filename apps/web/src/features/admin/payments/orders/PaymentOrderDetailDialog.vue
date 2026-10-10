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
    <div
      v-if="loading"
      class="flex min-h-56 flex-col items-center justify-center gap-3"
      role="status"
    >
      <AdminSpinner
        class="size-9 text-primary"
        :aria-label="PAYMENT_ORDERS_MESSAGES.detailLoading"
      />
      <span class="text-admin-body text-muted-foreground">{{
        PAYMENT_ORDERS_MESSAGES.detailLoading
      }}</span>
    </div>

    <AdminAlert v-else-if="error" severity="error">
      <div class="flex flex-wrap items-center justify-between gap-3">
        <span>{{ PAYMENT_ORDERS_MESSAGES.detailLoadFailed }}</span>
        <ActionButton variant="outline" @click="$emit('retry')">
          {{ PAYMENT_ORDERS_MESSAGES.retry }}
        </ActionButton>
      </div>
    </AdminAlert>

    <div v-else-if="detail" class="flex min-w-0 flex-col gap-5">
      <section class="rounded-control border border-border p-4">
        <div class="flex flex-wrap items-center justify-between gap-3">
          <div class="min-w-0">
            <div class="font-mono text-admin-content font-bold text-foreground">
              {{ detail.publicOrderNumber }}
            </div>
            <div class="mt-1 text-admin-meta text-muted-foreground">
              {{ detail.user.displayName }}
            </div>
          </div>
          <AdminStatusPill
            :label="getPaymentOrderStatusPresentation(detail.status).label"
            :variant="getPaymentOrderStatusPresentation(detail.status).variant"
            size="small"
          />
        </div>
        <dl class="mt-4 grid grid-cols-1 gap-x-5 gap-y-3 sm:grid-cols-2">
          <div v-for="row in overviewRows" :key="row.label" class="min-w-0">
            <dt class="text-admin-caption font-admin-strong text-muted-foreground">
              {{ row.label }}
            </dt>
            <dd class="mt-0.5 break-words text-admin-body font-semibold text-foreground">
              {{ row.value }}
            </dd>
          </div>
        </dl>
      </section>

      <section aria-labelledby="payment-stripe-identities">
        <h3 id="payment-stripe-identities" class="text-admin-body font-bold text-foreground">
          {{ PAYMENT_ORDERS_MESSAGES.stripeSummaryTitle }}
        </h3>
        <dl class="mt-3 grid grid-cols-1 gap-x-5 gap-y-3 sm:grid-cols-2">
          <div v-for="row in stripeRows" :key="row.label" class="min-w-0">
            <dt class="text-admin-caption font-admin-strong text-muted-foreground">
              {{ row.label }}
            </dt>
            <dd class="mt-0.5 break-all font-mono text-admin-meta text-foreground">
              {{ row.value }}
            </dd>
          </div>
        </dl>
      </section>

      <section aria-labelledby="payment-reconciliation">
        <div class="flex flex-wrap items-center gap-2">
          <h3 id="payment-reconciliation" class="text-admin-body font-bold text-foreground">
            {{ PAYMENT_ORDERS_MESSAGES.reconciliationTitle }}
          </h3>
          <AdminStatusPill
            :label="PAYMENT_ORDERS_MESSAGES.anomalyState(detail.reconciliation.anomalous)"
            :variant="detail.reconciliation.anomalous ? 'danger' : 'neutral'"
            size="small"
          />
        </div>
        <dl class="mt-3 grid grid-cols-1 gap-x-5 gap-y-3 sm:grid-cols-2">
          <div v-for="row in reconciliationRows" :key="row.label" class="min-w-0">
            <dt class="text-admin-caption font-admin-strong text-muted-foreground">
              {{ row.label }}
            </dt>
            <dd class="mt-0.5 break-words text-admin-body text-foreground">{{ row.value }}</dd>
          </div>
        </dl>
      </section>

      <section class="grid grid-cols-1 gap-5 md:grid-cols-2">
        <div aria-labelledby="payment-refunds">
          <h3 id="payment-refunds" class="text-admin-body font-bold text-foreground">
            {{ PAYMENT_ORDERS_MESSAGES.refundsTitle }}
          </h3>
          <p v-if="detail.refunds.length === 0" class="mt-2 text-admin-meta text-muted-foreground">
            {{ PAYMENT_ORDERS_MESSAGES.refundsEmpty }}
          </p>
          <ul v-else class="mt-2 flex flex-col gap-2">
            <li
              v-for="(refund, index) in detail.refunds"
              :key="refund.id || index"
              class="rounded-control border border-border p-3 text-admin-meta"
            >
              <div class="font-bold text-foreground">{{ refundStatusLabel(refund.status) }}</div>
              <div v-if="refund.stripeStatus" class="mt-1 text-muted-foreground">
                Stripe: {{ refund.stripeStatus }}
              </div>
              <div class="mt-1 text-muted-foreground">
                {{ formatRefund(refund) }} · {{ formatPaymentDateTime(refund.updatedAt) }}
              </div>
              <div v-if="refund.errorSummary" class="mt-1 break-words text-destructive">
                {{ refund.errorSummary }}
              </div>
            </li>
          </ul>
        </div>

        <div aria-labelledby="payment-disputes">
          <h3 id="payment-disputes" class="text-admin-body font-bold text-foreground">Dispute</h3>
          <p v-if="detail.disputes.length === 0" class="mt-2 text-admin-meta text-muted-foreground">
            {{ PAYMENT_ORDERS_MESSAGES.disputesEmpty }}
          </p>
          <ul v-else class="mt-2 flex flex-col gap-2">
            <li
              v-for="(dispute, index) in detail.disputes"
              :key="dispute.id || index"
              class="rounded-control border border-border p-3 text-admin-meta"
            >
              <div class="flex flex-wrap items-center justify-between gap-2">
                <div class="font-bold text-foreground">{{ dispute.status }}</div>
                <a
                  v-if="dispute.stripeDashboardUrl"
                  :href="dispute.stripeDashboardUrl"
                  target="_blank"
                  rel="noopener noreferrer"
                  class="font-bold text-primary hover:underline focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2"
                >
                  Stripe Dashboard
                </a>
              </div>
              <div class="mt-1 text-muted-foreground">{{ formatDispute(dispute) }}</div>
            </li>
          </ul>
        </div>
      </section>

      <section
        v-if="canRefund"
        aria-labelledby="payment-refund-action"
        class="rounded-control border border-border p-4"
      >
        <div class="flex flex-wrap items-start justify-between gap-3">
          <div>
            <h3 id="payment-refund-action" class="text-admin-body font-bold text-foreground">
              {{ PAYMENT_ORDERS_MESSAGES.refundActionTitle }}
            </h3>
          </div>
          <AdminStatusPill
            :label="PAYMENT_ORDERS_MESSAGES.refundableBadge(formatIdeas(refundableIdeas))"
            variant="info"
            size="small"
          />
        </div>
        <form class="mt-3 flex flex-wrap items-end gap-3" novalidate @submit.prevent="submitRefund">
          <label
            for="admin-payment-refund-ideas"
            class="flex min-w-44 flex-1 flex-col gap-1.5 text-admin-meta font-admin-strong text-muted-foreground"
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
            <small v-if="refundValidationError" class="text-destructive">{{
              refundValidationError
            }}</small>
          </label>
          <ActionButton
            type="submit"
            variant="destructive"
            :loading="refunding"
            :disabled="refunding || !canSubmitRefund"
          >
            {{ PAYMENT_ORDERS_MESSAGES.refundSubmit }}
          </ActionButton>
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
import AdminStatusPill from '../../shared/components/AdminStatusPill.vue'
import AdminButton from '../../shared/components/AdminButton.vue'
import ActionButton from '../../../../core/components/ActionButton.vue'
import AdminNumberInput from '../../shared/components/AdminNumberInput.vue'
import AdminSpinner from '../../shared/components/AdminSpinner.vue'
import AdminDialogShell from '../../shared/components/AdminDialogShell.vue'
import {
  formatPaymentDateTime,
  formatPaymentMoney,
  getCreditStatusLabel,
  getPaymentOrderStatusPresentation,
  getStripeStatusLabel,
} from './paymentOrderPresentation'
import { formatLocaleNumber } from '../../../../core/i18n/formatters'
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
    pending: 'Refund Pending',
    succeeded: 'Refund Succeeded',
    failed: 'Refund Failed',
    anomaly: 'Needs Manual Review',
    'refund.updated': 'Stripe Refund Updated',
    'refund.failed': 'Stripe Refund Failed',
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
