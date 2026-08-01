<template>
  <section
    ref="statusElement"
    tabindex="-1"
    class="rounded-[var(--radius-ib-panel)] border px-4 py-3 focus-visible:outline-none focus-visible:shadow-[var(--shadow-ib-focus)]"
    :class="statusClass"
    :role="showError ? 'alert' : 'status'"
    aria-live="polite"
  >
    <template v-if="status?.status === 'credited'">
      <p class="m-0 font-semibold text-ib-success">
        {{ formatIdeaAmount(status.idea_quantity) }} 已到账
      </p>
      <p class="mt-1 mb-0 text-sm text-ib-text-muted">余额与充值记录已更新。</p>
    </template>
    <template v-else-if="source === 'cancel'">
      <p class="m-0 font-semibold text-ib-text">已返回 IdeaBoard</p>
      <p class="mt-1 mb-0 text-sm text-ib-text-muted">订单未到账，不会仅因页面返回而增加 Idea。</p>
    </template>
    <template v-else-if="pending">
      <p class="m-0 font-semibold text-ib-text">正在确认支付结果</p>
      <p class="mt-1 mb-0 text-sm text-ib-text-muted">支付结果以 Stripe 确认状态为准，请稍候。</p>
    </template>
    <template v-else-if="error">
      <p class="m-0 font-semibold text-ib-danger">订单状态加载失败</p>
      <Button
        label="重新查询"
        severity="secondary"
        size="small"
        text
        class="mt-2"
        @click="emit('retry')"
      />
    </template>
    <template v-else-if="status?.status === 'failed'">
      <p class="m-0 font-semibold text-ib-danger">支付未完成</p>
      <p class="mt-1 mb-0 text-sm text-ib-text-muted">本次没有 Idea 到账，请重新发起支付。</p>
    </template>
    <template v-else-if="delayed">
      <p class="m-0 font-semibold text-ib-text">到账处理中</p>
      <p class="mt-1 mb-0 text-sm text-ib-text-muted">我们仍在等待 Stripe 确认，无需重复支付。</p>
    </template>
    <template v-else>
      <p class="m-0 font-semibold text-ib-text">正在确认支付结果</p>
      <p class="mt-1 mb-0 text-sm text-ib-text-muted">支付结果以 Stripe 确认状态为准，请稍候。</p>
    </template>
  </section>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import Button from 'primevue/button'
import { formatIdeaAmount } from '../../../shared/utils/ideaCurrency'
import type { PaymentReturnSource } from '../paymentReturnRoute'
import type { UserPaymentOrderReturnStatus } from '../userPaymentsResource'

const props = defineProps<{
  pending: boolean
  error: unknown
  source: PaymentReturnSource
  status?: UserPaymentOrderReturnStatus
}>()
const emit = defineEmits<{ retry: [] }>()
const statusElement = ref<HTMLElement>()
const delayed = ref(false)
let delayedTimer: ReturnType<typeof setTimeout> | undefined
const showError = computed(
  () =>
    props.status?.status !== 'credited' &&
    props.source !== 'cancel' &&
    props.error !== null &&
    props.error !== undefined,
)
const statusClass = computed(() => {
  if (props.status?.status === 'credited') {
    return 'border-ib-success/25 bg-ib-surface text-ib-text'
  }
  if (props.source === 'cancel') {
    return 'border-ib-border-soft bg-ib-surface text-ib-text'
  }
  if (props.error || props.status?.status === 'failed') {
    return 'border-ib-danger/25 bg-ib-surface text-ib-text'
  }
  return 'border-ib-border-soft bg-ib-surface text-ib-text'
})

onMounted(async () => {
  delayedTimer = setTimeout(() => {
    delayed.value = true
  }, 60_000)
  await nextTick()
  statusElement.value?.focus()
})

onBeforeUnmount(() => {
  if (delayedTimer) clearTimeout(delayedTimer)
})
</script>
