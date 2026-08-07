<template>
  <section
    class="rounded-[var(--radius-ib-panel)] border border-ib-border-soft bg-ib-surface p-5 shadow-[var(--shadow-ib-control)] max-[640px]:p-4"
    aria-labelledby="recharge-purchase-heading"
  >
    <div
      v-if="pending"
      class="grid gap-4"
      role="status"
      aria-live="polite"
      aria-label="正在加载充值选项"
    >
      <span class="sr-only">正在加载充值选项</span>
      <Skeleton height="4.5rem" />
      <Skeleton height="2.75rem" />
      <Skeleton height="2.75rem" />
    </div>

    <div
      v-else-if="!options"
      class="flex min-h-40 flex-col items-center justify-center text-center"
    >
      <p id="recharge-purchase-heading" class="m-0 font-semibold text-ib-danger">
        充值选项加载失败
      </p>
      <p class="mt-1 mb-0 text-sm text-ib-text-muted">{{ errorMessage(error) }}</p>
      <Button
        label="重新加载"
        severity="secondary"
        size="small"
        text
        class="mt-3"
        @click="emit('retry')"
      />
    </div>

    <div
      v-else-if="!options.purchases_available"
      class="flex min-h-36 flex-col items-center justify-center text-center"
      role="status"
    >
      <p id="recharge-purchase-heading" class="m-0 font-semibold text-ib-text">暂时无法充值</p>
      <p class="mt-1 mb-0 text-sm text-ib-text-muted">
        {{ unavailableMessage(options.unavailable_reason) }}
      </p>
    </div>

    <form v-else class="grid gap-5" novalidate @submit.prevent="submitCheckout">
      <div>
        <div class="flex items-end justify-between gap-4">
          <h3 id="recharge-purchase-heading" class="m-0 text-base font-bold text-ib-text">
            选择充值数量
          </h3>
          <span class="text-[12px] font-semibold text-ib-text-subtle">$1.00 USD = 100 Ideas</span>
        </div>
        <div
          class="mt-3 grid grid-cols-5 gap-2.5 max-[900px]:grid-cols-3 max-[640px]:grid-cols-2"
          aria-label="快捷充值套餐"
        >
          <button
            v-for="quantity in options.quick_idea_quantities"
            :key="quantity"
            type="button"
            v-bind="packageControlState(quantity)"
            class="flex min-h-[72px] min-w-0 flex-col items-start justify-center rounded-[var(--radius-ib-control)] border px-3 py-2 text-left transition-[background-color,border-color,box-shadow,color] duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] focus-visible:outline-none focus-visible:shadow-[var(--shadow-ib-focus)] disabled:cursor-not-allowed disabled:opacity-60"
            :class="
              quantity === selectedQuantity && selectionMode === 'package'
                ? 'border-ib-creative-accent bg-ib-creative-accent-soft text-ib-creative-accent-pressed'
                : 'border-ib-border-soft bg-ib-surface text-ib-text hover:border-ib-creative-accent/45 hover:bg-ib-surface-hover'
            "
            @click="selectPackage(quantity)"
          >
            <span class="text-sm font-bold">{{ ideaFormatter.format(quantity) }} Ideas</span>
            <span class="mt-1 font-mono text-[12px] text-ib-text-muted">{{
              formatUsdForIdeas(quantity)
            }}</span>
          </button>
        </div>
      </div>

      <IbFormField
        v-slot="{ fieldId }"
        label="自定义数量"
        field-id="account-recharge-custom"
        field-class="grid gap-2"
        label-class="text-[13px] font-semibold text-ib-text-muted"
      >
        <div class="grid grid-cols-[minmax(0,1fr)_auto] items-center gap-3 max-[640px]:grid-cols-1">
          <input
            :id="fieldId"
            v-model="customQuantity"
            name="custom-recharge-quantity"
            type="text"
            inputmode="numeric"
            autocomplete="off"
            :disabled="checkoutPending"
            :class="ibFormInputClass"
            placeholder="500–500,000"
            @focus="selectionMode = 'custom'"
            @input="selectionMode = 'custom'"
            @blur="normalizeCustomQuantity"
          />
          <span class="whitespace-nowrap font-mono text-sm font-semibold text-ib-text-muted">{{
            selectedPrice
          }}</span>
        </div>
      </IbFormField>

      <div>
        <Button
          unstyled
          type="submit"
          label="立即支付"
          :disabled="!canCheckout"
          :loading="checkoutPending"
          :class="accountPrimaryActionButtonClass"
          :pt="accountButtonPt"
        />
        <p class="mt-2 mb-0 text-center text-[12px] leading-5 text-ib-text-subtle">
          点击“立即支付”即表示你同意
          <a
            :href="options.terms.url"
            target="_blank"
            rel="noopener noreferrer"
            class="rounded-[var(--radius-ib-control)] font-semibold text-ib-creative-accent-pressed focus-visible:outline-none focus-visible:shadow-[var(--shadow-ib-focus)]"
            >服务条款</a
          >
          和
          <a
            :href="options.refund_policy.url"
            target="_blank"
            rel="noopener noreferrer"
            class="rounded-[var(--radius-ib-control)] font-semibold text-ib-creative-accent-pressed focus-visible:outline-none focus-visible:shadow-[var(--shadow-ib-focus)]"
            >退款政策</a
          >
        </p>
        <p
          v-if="checkoutError"
          class="mt-2 mb-0 text-center text-[13px] font-semibold text-ib-danger"
          role="alert"
        >
          {{ errorMessage(checkoutError) }}
        </p>
      </div>
    </form>
  </section>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import Button from 'primevue/button'
import Skeleton from 'primevue/skeleton'
import IbFormField from '../../../shared/components/IbFormField.vue'
import { resolveInteractiveControlState } from '../../../shared/interaction/interactiveControlState'
import { ibFormInputClass } from '../../../shared/theme/formControls'
import { formatUsdForIdeas, MIN_IDEA_QUANTITY, normalizeIdeaQuantity } from '../paymentMoney'
import { accountButtonPt, accountPrimaryActionButtonClass } from './accountManagementUi'
import type { UserPaymentPurchaseOptions } from '../userPaymentsResource'

const props = defineProps<{
  options?: UserPaymentPurchaseOptions
  pending: boolean
  error: unknown
  checkoutPending: boolean
  checkoutError: unknown
}>()
const emit = defineEmits<{
  retry: []
  checkout: [ideaQuantity: number]
}>()

const selectionMode = ref<'package' | 'custom'>('package')
const selectedQuantity = ref(1000)
const customQuantity = ref('')
const ideaFormatter = new Intl.NumberFormat('en-US')

watch(
  () => props.options,
  (options) => {
    if (!options || options.quick_idea_quantities.length === 0) return
    if (!options.quick_idea_quantities.includes(selectedQuantity.value)) {
      selectedQuantity.value = options.quick_idea_quantities[0]!
    }
  },
  { immediate: true },
)

const normalizedCustomQuantity = computed(() => normalizeIdeaQuantity(customQuantity.value))
const checkoutQuantity = computed(() => {
  if (selectionMode.value === 'package') return selectedQuantity.value
  return normalizedCustomQuantity.value
})
const selectedPrice = computed(() =>
  checkoutQuantity.value == null ? '—' : formatUsdForIdeas(checkoutQuantity.value),
)
const canCheckout = computed(() => checkoutQuantity.value != null && !props.checkoutPending)

function packageControlState(quantity: number) {
  return resolveInteractiveControlState({
    selected: selectionMode.value === 'package' && selectedQuantity.value === quantity,
    disabled: props.checkoutPending,
  })
}

function selectPackage(quantity: number) {
  selectionMode.value = 'package'
  selectedQuantity.value = quantity
  customQuantity.value = ''
}

function normalizeCustomQuantity() {
  if (selectionMode.value !== 'custom') return
  customQuantity.value = String(normalizedCustomQuantity.value ?? MIN_IDEA_QUANTITY)
}

function submitCheckout() {
  normalizeCustomQuantity()
  const quantity = checkoutQuantity.value
  if (!canCheckout.value || quantity == null) return
  emit('checkout', quantity)
}

function unavailableMessage(reason?: UserPaymentPurchaseOptions['unavailable_reason']): string {
  switch (reason) {
    case 'purchases_disabled':
      return '充值功能暂未开放'
    case 'wallet_restricted':
      return props.options?.restriction_type === 'payment_dispute'
        ? '银行卡 Dispute 处理中，当前账户暂时无法充值或使用 Idea'
        : '当前账户暂时无法充值'
    default:
      return '充值服务尚未准备好，请稍后再试'
  }
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : '网络错误，请重试'
}
</script>
