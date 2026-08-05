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
            暂无 Dispute
          </p>
          <ul v-else class="mt-2 flex flex-col gap-2">
            <li
              v-for="(dispute, index) in detail.disputes"
              :key="dispute.id || index"
              class="rounded-[var(--radius-ib-control)] border border-admin-border-soft p-3 text-admin-meta"
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
        class="rounded-[var(--radius-ib-control)] border border-admin-border-soft p-4"
      >
        <div class="flex flex-wrap items-start justify-between gap-3">
          <div>
            <h3 id="payment-refund-action" class="text-admin-body font-bold text-admin-title-color">
              发起退款
            </h3>
            <p class="mt-1 text-admin-caption text-admin-text-3">
              仅可退回未消费 Idea，最终可退额度由服务端按 Wallet 可用余额再次校验。
            </p>
          </div>
          <Tag :value="`${formatIdeas(refundableIdeas)} 可退`" severity="info" rounded />
        </div>
        <form class="mt-3 flex flex-wrap items-end gap-3" novalidate @submit.prevent="submitRefund">
          <label
            for="admin-payment-refund-ideas"
            class="flex min-w-44 flex-1 flex-col gap-1.5 text-admin-meta font-[650] text-admin-text-2"
          >
            <span>退款 Idea 数量</span>
            <InputNumber
              v-model="refundIdeaQuantity"
              input-id="admin-payment-refund-ideas"
              aria-label="退款 Idea 数量"
              :min="1"
              :max="refundableIdeas"
              :disabled="refunding"
              :use-grouping="true"
              fluid
              :invalid="Boolean(refundValidationError)"
            />
            <small v-if="refundValidationError" class="text-admin-danger">{{
              refundValidationError
            }}</small>
          </label>
          <Button
            type="submit"
            label="确认退款"
            severity="danger"
            :loading="refunding"
            :disabled="refunding || !canSubmitRefund"
          />
        </form>
        <Message v-if="refundError" class="mt-3" severity="error" :closable="false">
          {{ refundError }}
        </Message>
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
import { computed, ref, watch } from 'vue'
import Button from 'primevue/button'
import InputNumber from 'primevue/inputnumber'
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
    return '请输入正整数 Idea 数量'
  }
  if (quantity > refundableIdeas.value)
    return `最多可退 ${formatIdeas(refundableIdeas.value)} Ideas`
  return ''
})
const canSubmitRefund = computed(() => canRefund.value && !refundValidationError.value)

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
  return new Intl.NumberFormat('zh-CN').format(value)
}

function submitRefund() {
  if (!canSubmitRefund.value || refundIdeaQuantity.value === null) return
  emit('refund', refundIdeaQuantity.value)
}

function formatDispute(dispute: AdminPaymentDisputeSummary): string {
  const amount = formatPaymentMoney(dispute.amountMinor, dispute.currency)
  const ratio = dispute.ratioBasisPoints !== undefined
    ? ` / 比例 ${(dispute.ratioBasisPoints / 100).toLocaleString('zh-CN', { maximumFractionDigits: 2 })}%`
    : ''
  const target = formatIdeas(dispute.targetIdeaQuantity ?? 0)
  const debited = formatIdeas(dispute.debitedIdeaQuantity ?? 0)
  const unrecovered = formatIdeas(dispute.unrecoveredCredits ?? 0)
  const provider = dispute.providerStatus ? ` / Stripe ${dispute.providerStatus}` : ''
  const opened = `开启 ${formatPaymentDateTime(dispute.openedAt)}`
  const closed = dispute.closedAt ? ` / 关闭 ${formatPaymentDateTime(dispute.closedAt)}` : ''
  return `${amount}${ratio} / 目标 ${target} Ideas / 已扣回 ${debited} Ideas / 未追回 ${unrecovered} Ideas${provider} / ${opened}${closed}`
}
</script>
