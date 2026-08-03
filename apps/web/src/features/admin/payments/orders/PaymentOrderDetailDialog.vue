<template>
  <AdminDialogShell
    :visible="visible"
    :header="detail ? `支付订单 · ${detail.publicOrderNumber}` : '支付订单详情'"
    size="wide"
    content-layout="scrollable"
    :busy="loading"
    @update:visible="$emit('update:visible', $event)"
  >
    <div v-if="loading" class="grid min-h-56 place-items-center" role="status">
      <span class="text-admin-body text-admin-text-3">正在加载订单详情…</span>
    </div>

    <Message v-else-if="error" severity="error" :closable="false">
      <div class="flex flex-wrap items-center justify-between gap-3">
        <span>支付订单详情加载失败，请重试。</span>
        <Button label="重试" size="small" severity="secondary" outlined @click="$emit('retry')" />
      </div>
    </Message>

    <div v-else-if="detail" class="flex min-w-0 flex-col gap-5">
      <section class="rounded-[var(--radius-ib-control)] border border-admin-border-soft p-4">
        <div class="flex flex-wrap items-center justify-between gap-3">
          <div class="min-w-0">
            <div class="font-mono text-admin-content font-bold text-admin-title-color">
              {{ detail.publicOrderNumber }}
            </div>
            <div class="mt-1 text-admin-meta text-admin-text-3">
              用户 {{ detail.user.username }} · ID {{ detail.user.id }}
            </div>
          </div>
          <Tag
            :value="getPaymentOrderStatusPresentation(detail.status).label"
            :severity="getPaymentOrderStatusPresentation(detail.status).severity"
            rounded
          />
        </div>
        <dl class="mt-4 grid grid-cols-2 gap-x-5 gap-y-3 max-[640px]:grid-cols-1">
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
          Stripe 安全摘要
        </h3>
        <p class="mt-1 text-admin-caption text-admin-text-3">
          仅显示服务端脱敏 identity，不包含 API Key、Webhook Payload 或账单地址。
        </p>
        <dl class="mt-3 grid grid-cols-2 gap-x-5 gap-y-3 max-[640px]:grid-cols-1">
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
            对账与异常
          </h3>
          <Tag
            :value="detail.reconciliation.anomalous ? '超过 24 小时未收敛' : '无异常标记'"
            :severity="detail.reconciliation.anomalous ? 'danger' : 'secondary'"
            rounded
          />
        </div>
        <dl class="mt-3 grid grid-cols-2 gap-x-5 gap-y-3 max-[640px]:grid-cols-1">
          <div v-for="row in reconciliationRows" :key="row.label" class="min-w-0">
            <dt class="text-admin-caption font-[650] text-admin-text-3">{{ row.label }}</dt>
            <dd class="mt-0.5 break-words text-admin-body text-admin-text-1">{{ row.value }}</dd>
          </div>
        </dl>
      </section>

      <section class="grid grid-cols-2 gap-5 max-[720px]:grid-cols-1">
        <div aria-labelledby="payment-refunds">
          <h3 id="payment-refunds" class="text-admin-body font-bold text-admin-title-color">
            退款
          </h3>
          <p v-if="detail.refunds.length === 0" class="mt-2 text-admin-meta text-admin-text-3">
            暂无退款记录
          </p>
          <ul v-else class="mt-2 flex flex-col gap-2">
            <li
              v-for="(refund, index) in detail.refunds"
              :key="refund.id || index"
              class="rounded-[var(--radius-ib-control)] border border-admin-border-soft p-3 text-admin-meta"
            >
              <div class="font-bold text-admin-text-1">{{ refund.status }}</div>
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
            暂无 Dispute
          </p>
          <ul v-else class="mt-2 flex flex-col gap-2">
            <li
              v-for="(dispute, index) in detail.disputes"
              :key="dispute.id || index"
              class="rounded-[var(--radius-ib-control)] border border-admin-border-soft p-3 text-admin-meta"
            >
              <div class="font-bold text-admin-text-1">{{ dispute.status }}</div>
              <div class="mt-1 text-admin-text-3">{{ formatDispute(dispute) }}</div>
            </li>
          </ul>
        </div>
      </section>

      <div v-if="detail.receiptUrl" class="flex justify-end">
        <Button
          as="a"
          :href="detail.receiptUrl"
          target="_blank"
          rel="noopener noreferrer"
          label="打开 Stripe 收据"
          size="small"
          severity="secondary"
          outlined
        />
      </div>
    </div>
  </AdminDialogShell>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import Button from 'primevue/button'
import Message from 'primevue/message'
import Tag from 'primevue/tag'
import AdminDialogShell from '../../shared/components/AdminDialogShell.vue'
import {
  formatPaymentDateTime,
  formatPaymentMoney,
  getCreditStatusLabel,
  getPaymentOrderStatusPresentation,
  getStripeStatusLabel,
} from './paymentOrderPresentation'
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
}>()

defineEmits<{
  'update:visible': [visible: boolean]
  retry: []
}>()

const overviewRows = computed(() => {
  const detail = props.detail
  if (!detail) return []
  return [
    { label: 'Idea 数量', value: new Intl.NumberFormat('zh-CN').format(detail.ideaQuantity) },
    {
      label: '原始实付',
      value: formatPaymentMoney(detail.paidTotalMinor, detail.paidCurrency),
    },
    { label: 'Stripe 状态', value: getStripeStatusLabel(detail.stripeStatus) },
    { label: '本地到账状态', value: getCreditStatusLabel(detail.creditStatus) },
    { label: '商品金额', value: formatPaymentMoney(detail.paidSubtotalMinor, detail.paidCurrency) },
    { label: '税费', value: formatPaymentMoney(detail.paidTaxMinor, detail.paidCurrency) },
    { label: '基准金额', value: formatPaymentMoney(detail.baseAmountMinor, detail.baseCurrency) },
    { label: '条款版本', value: detail.termsVersion || '—' },
    { label: '退款政策版本', value: detail.refundPolicyVersion || '—' },
    { label: '创建时间', value: formatPaymentDateTime(detail.createdAt) },
    { label: '支付时间', value: formatPaymentDateTime(detail.paidAt) },
    { label: '到账时间', value: formatPaymentDateTime(detail.creditedAt) },
    { label: '过期时间', value: formatPaymentDateTime(detail.expiresAt) },
    { label: '最近更新', value: formatPaymentDateTime(detail.updatedAt) },
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
    { label: '异常标记时间', value: formatPaymentDateTime(reconciliation.anomalousAt) },
    { label: '最近对账时间', value: formatPaymentDateTime(reconciliation.lastCheckedAt) },
    { label: '下次尝试时间', value: formatPaymentDateTime(reconciliation.nextAttemptAt) },
    { label: '错误代码', value: reconciliation.lastErrorCode || '—' },
    { label: '脱敏错误摘要', value: reconciliation.lastErrorSummary || '—' },
  ]
})

function formatRefund(refund: AdminPaymentRefundSummary): string {
  const ideas =
    refund.ideaQuantity === undefined
      ? 'Idea 数量未知'
      : `${new Intl.NumberFormat('zh-CN').format(refund.ideaQuantity)} Ideas`
  return `${ideas} / ${formatPaymentMoney(refund.amountMinor, refund.currency)}`
}

function formatDispute(dispute: AdminPaymentDisputeSummary): string {
  const amount = formatPaymentMoney(dispute.amountMinor, dispute.currency)
  const unrecovered = dispute.unrecoveredIdeas
    ? ` / 未追回 ${new Intl.NumberFormat('zh-CN').format(dispute.unrecoveredIdeas)} Ideas`
    : ''
  return `${amount}${unrecovered} / ${formatPaymentDateTime(dispute.openedAt)}`
}
</script>
