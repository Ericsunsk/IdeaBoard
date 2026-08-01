<template>
  <section
    v-if="pending || error || items.length"
    class="grid gap-3"
    aria-labelledby="pending-payment-orders-heading"
  >
    <div class="flex items-center justify-between gap-3">
      <h3 id="pending-payment-orders-heading" class="m-0 text-lg font-bold text-ib-text">
        待支付订单
      </h3>
      <span class="text-[12px] font-semibold text-ib-text-subtle">不会计入充值记录</span>
    </div>

    <div
      v-if="pending"
      class="grid gap-2 rounded-[var(--radius-ib-panel)] border border-ib-border-soft bg-ib-surface p-4 shadow-[var(--shadow-ib-control)]"
      role="status"
      aria-label="正在加载待支付订单"
    >
      <Skeleton height="1.25rem" width="42%" />
      <Skeleton height="1rem" width="68%" />
    </div>

    <div
      v-else-if="error"
      class="rounded-[var(--radius-ib-panel)] border border-ib-danger/25 bg-ib-surface p-4 shadow-[var(--shadow-ib-control)]"
      role="alert"
    >
      <p class="m-0 font-semibold text-ib-danger">待支付订单加载失败</p>
      <p class="mt-1 mb-0 text-sm text-ib-text-muted">{{ errorMessage(error) }}</p>
      <Button label="重新加载" severity="secondary" size="small" text class="mt-2" @click="emit('retry')" />
    </div>

    <div v-else class="grid gap-2">
      <article
        v-for="item in items"
        :key="item.public_token"
        :data-testid="`pending-payment-order-${item.public_token}`"
        class="rounded-[var(--radius-ib-panel)] border border-ib-border-soft bg-ib-surface p-4 shadow-[var(--shadow-ib-control)] max-[640px]:p-3.5"
      >
        <div class="flex min-w-0 items-start justify-between gap-4 max-[640px]:flex-col">
          <div class="min-w-0">
            <p class="m-0 font-mono text-lg font-bold text-ib-text">
              {{ formatIdeaAmount(item.idea_quantity) }}
            </p>
            <p class="mt-1 mb-0 text-sm font-semibold" :class="statusTone(item.status)">
              {{ statusText(item) }}
            </p>
            <p class="mt-1 mb-0 text-[13px] text-ib-text-muted">
              {{ expiryText(item) }}
            </p>
          </div>

          <div
            v-if="item.status === 'checkout_open'"
            class="flex shrink-0 items-center gap-2 max-[640px]:w-full max-[640px]:flex-wrap"
          >
            <template v-if="abandonConfirmationToken === item.public_token">
              <span class="text-[12px] font-semibold text-ib-text-muted">放弃后需重新创建订单</span>
              <Button
                label="确认放弃"
                severity="danger"
                size="small"
                :loading="isActionPending(item.public_token, 'abandon')"
                :disabled="actionPending"
                @click="emit('abandon', item.public_token)"
              />
              <Button
                label="取消"
                severity="secondary"
                size="small"
                text
                :disabled="actionPending"
                @click="abandonConfirmationToken = null"
              />
            </template>
            <template v-else>
              <Button
                unstyled
                label="继续支付"
                size="small"
                :loading="isActionPending(item.public_token, 'continue')"
                :disabled="actionPending"
                :class="accountPrimaryActionButtonClass"
                :pt="accountButtonPt"
                @click="emit('continue', item.public_token)"
              />
              <Button
                label="放弃订单"
                severity="secondary"
                size="small"
                outlined
                :disabled="actionPending"
                @click="abandonConfirmationToken = item.public_token"
              />
            </template>
          </div>
        </div>

        <div
          v-if="actionError && actionTarget?.publicToken === item.public_token"
          class="mt-3 flex flex-wrap items-center gap-x-3 gap-y-1 rounded-[var(--radius-ib-control)] border border-ib-danger/25 bg-ib-surface-muted px-3 py-2"
          role="alert"
        >
          <span class="text-sm font-semibold text-ib-danger">{{ errorMessage(actionError) }}</span>
          <Button
            :label="actionTarget.kind === 'continue' ? '重试继续支付' : '重试放弃订单'"
            severity="secondary"
            size="small"
            text
            class="!p-0"
            :disabled="actionPending"
            @click="retryAction(item.public_token)"
          />
        </div>
      </article>
    </div>
  </section>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import Button from 'primevue/button'
import Skeleton from 'primevue/skeleton'
import { formatIdeaAmount } from '../../../shared/utils/ideaCurrency'
import { accountButtonPt, accountPrimaryActionButtonClass } from './accountManagementUi'
import type { UserPendingPaymentOrder } from '../userPaymentsResource'

type PendingOrderAction = { publicToken: string; kind: 'continue' | 'abandon' }

const props = defineProps<{
  items: UserPendingPaymentOrder[]
  pending: boolean
  error: unknown
  actionPending: boolean
  actionTarget: PendingOrderAction | null
  actionError: unknown
}>()
const emit = defineEmits<{
  retry: []
  continue: [publicToken: string]
  abandon: [publicToken: string]
}>()

const abandonConfirmationToken = ref<string | null>(null)
const now = ref(Date.now())
let refreshTimer: ReturnType<typeof setInterval> | undefined

onMounted(() => {
  refreshTimer = setInterval(() => {
    now.value = Date.now()
  }, 5_000)
})
onBeforeUnmount(() => {
  if (refreshTimer) clearInterval(refreshTimer)
})

function isActionPending(publicToken: string, kind: PendingOrderAction['kind']) {
  return (
    props.actionPending &&
    props.actionTarget?.publicToken === publicToken &&
    props.actionTarget.kind === kind
  )
}

function retryAction(publicToken: string) {
  if (props.actionTarget?.kind === 'continue') {
    emit('continue', publicToken)
    return
  }
  emit('abandon', publicToken)
}

function statusText(item: UserPendingPaymentOrder) {
  switch (item.status) {
    case 'checkout_open':
      return '待完成支付'
    case 'async_payment_pending':
      return '正在等待支付确认'
    case 'paid_pending_credit':
      return isCreditProcessingDelayed(item.updated_at) ? '到账处理中' : '支付已完成，正在到账'
    case 'expired':
      return '支付链接已过期'
    case 'cancelled':
      return '订单已放弃'
    case 'payment_failed':
      return '支付未完成'
    case 'credited':
      return 'Ideas 已到账'
  }
}

function statusTone(status: UserPendingPaymentOrder['status']) {
  if (status === 'credited') return 'text-ib-success'
  if (status === 'expired' || status === 'cancelled' || status === 'payment_failed') {
    return 'text-ib-text-muted'
  }
  return 'text-ib-creative-accent-pressed'
}

function isCreditProcessingDelayed(updatedAt: string) {
  const updatedAtMs = new Date(updatedAt).getTime()
  return Number.isFinite(updatedAtMs) && now.value - updatedAtMs > 60_000
}

function expiryText(item: UserPendingPaymentOrder) {
  const date = new Date(item.expires_at)
  if (Number.isNaN(date.getTime())) return '有效期未知'
  const formatted = new Intl.DateTimeFormat('zh-CN', {
    dateStyle: 'medium',
    timeStyle: 'short',
    hour12: false,
  }).format(date)
  return item.status === 'expired' ? `已于 ${formatted} 过期` : `有效至 ${formatted}`
}

function errorMessage(error: unknown) {
  return error instanceof Error ? error.message : '网络错误，请重试'
}
</script>
