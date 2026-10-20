<template>
  <section :class="accountCardPanelClass" aria-labelledby="recharge-purchase-heading">
    <div
      v-if="pending"
      class="grid gap-4"
      role="status"
      aria-live="polite"
      :aria-label="t('account.topUp.optionsLoading')"
    >
      <span class="sr-only">{{ t('account.topUp.optionsLoading') }}</span>
      <Skeleton height="4.5rem" />
      <Skeleton height="2.75rem" />
      <Skeleton height="2.75rem" />
    </div>

    <LoadErrorPanel
      v-else-if="!options"
      id="recharge-purchase-heading"
      class="min-h-40 justify-center"
      align="center"
      :message="t('account.errors.optionsLoadFailed')"
      :detail="errorMessage(error)"
      :retry-label="t('account.reload')"
      retry-variant="ghost"
      @retry="emit('retry')"
    />

    <div
      v-else-if="!options.purchases_available"
      class="flex min-h-36 flex-col items-center justify-center text-center"
      role="status"
    >
      <p id="recharge-purchase-heading" class="m-0 font-semibold text-foreground">
        {{ t('account.topUp.unavailableTitle') }}
      </p>
      <p class="mt-1 mb-0 text-sm text-muted-foreground">
        {{ unavailableMessage(options.unavailable_reason) }}
      </p>
    </div>

    <form v-else class="grid gap-5" novalidate @submit.prevent="submitCheckout">
      <div>
        <div class="flex items-end justify-between gap-4">
          <h3 id="recharge-purchase-heading" class="m-0 text-base font-bold text-foreground">
            {{ t('account.topUp.chooseQuantity') }}
          </h3>
          <span class="text-admin-meta font-semibold text-muted-foreground"
            >$1.00 USD = 100 Ideas</span
          >
        </div>
        <div
          class="mt-3 grid grid-cols-2 gap-2.5 sm:grid-cols-3 lg:grid-cols-5"
          :aria-label="t('account.topUp.packagesAria')"
        >
          <button
            v-for="quantity in options.quick_idea_quantities"
            :key="quantity"
            type="button"
            v-bind="packageControlState(quantity)"
            class="flex min-h-[72px] min-w-0 flex-col items-start justify-center rounded-control border px-3 py-2 text-left transition-[background-color,border-color,box-shadow,color] duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] focus-visible:outline-none focus-visible:shadow-focus-ring disabled:cursor-not-allowed disabled:opacity-60"
            :class="
              quantity === selectedQuantity && selectionMode === 'package'
                ? 'border-primary bg-primary/10 text-primary'
                : 'border-border bg-card text-foreground hover:border-primary/45 hover:bg-muted'
            "
            @click="selectPackage(quantity)"
          >
            <span class="text-sm font-bold">{{ formatLocaleNumber(quantity) }} Ideas</span>
            <span class="mt-1 font-mono text-admin-meta text-muted-foreground">{{
              formatUsdForIdeas(quantity)
            }}</span>
          </button>
        </div>
      </div>

      <IbFormField
        v-slot="{ fieldId }"
        :label="t('account.topUp.customQuantity')"
        field-id="account-recharge-custom"
        field-class="grid gap-2"
        label-class="text-admin-body font-semibold text-muted-foreground"
      >
        <div class="grid grid-cols-1 items-center gap-3 sm:grid-cols-[minmax(0,1fr)_auto]">
          <Input
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
            @update:model-value="selectionMode = 'custom'"
            @blur="normalizeCustomQuantity"
          />
          <span class="whitespace-nowrap font-mono text-sm font-semibold text-muted-foreground">{{
            selectedPrice
          }}</span>
        </div>
      </IbFormField>

      <div>
        <Button
          type="submit"
          :is-disabled="!canCheckout"
          :is-loading="checkoutPending"
          :class="accountPrimaryActionButtonClass"
          >{{ t('account.topUp.payNow') }}</Button
        >
        <p class="mt-2 mb-0 text-center text-admin-meta leading-5 text-muted-foreground">
          {{ t('account.topUp.termsPrefix') }}
          <a
            :href="options.terms.url"
            target="_blank"
            rel="noopener noreferrer"
            class="rounded-control font-semibold text-primary focus-visible:outline-none focus-visible:shadow-focus-ring"
            >{{ t('account.topUp.termsLink') }}</a
          >{{ t('account.topUp.termsAnd') }}
          <a
            :href="options.refund_policy.url"
            target="_blank"
            rel="noopener noreferrer"
            class="rounded-control font-semibold text-primary focus-visible:outline-none focus-visible:shadow-focus-ring"
            >{{ t('account.topUp.refundLink') }}</a
          >
        </p>
        <p
          v-if="checkoutError"
          class="mt-2 mb-0 text-center text-admin-body font-semibold text-destructive"
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
import { useI18n } from 'vue-i18n'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import IbFormField from '../../../../core/components/IbFormField.vue'
import LoadErrorPanel from '../../../../core/components/LoadErrorPanel.vue'
import { userFacingErrorCopy } from '../../../../core/errors/errorCatalog'
import { ACCOUNT_GUIDANCE_COPY_KEYS } from '../../session/workspaceBilling'
import { resolveInteractiveControlState } from '../../../../core/interaction/interactiveControlState'
import { ibFormInputClass } from '../../../../theme/formControls'
import { formatLocaleNumber } from '../../../../core/i18n/formatters'
import { MIN_IDEA_QUANTITY, normalizeIdeaQuantity } from '../../core/money'
import { formatUsdForIdeas } from '../../session/money'
import type { PaymentPurchaseOptions } from '../../session/payments/resource'
import { accountCardPanelClass, accountPrimaryActionButtonClass } from '../shared/ui'

const props = defineProps<{
  options?: PaymentPurchaseOptions | undefined
  pending: boolean
  error: unknown
  checkoutPending: boolean
  checkoutError: unknown
}>()
const emit = defineEmits<{
  retry: []
  checkout: [ideaQuantity: number]
}>()
const { t } = useI18n()
const selectionMode = ref<'package' | 'custom'>('package')
const selectedQuantity = ref(1000)
const customQuantity = ref('')

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
    isSelected: selectionMode.value === 'package' && selectedQuantity.value === quantity,
    isDisabled: props.checkoutPending,
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

function unavailableMessage(reason?: PaymentPurchaseOptions['unavailable_reason']): string {
  switch (reason) {
    case 'purchases_disabled':
      return t('account.topUp.reasonDisabled')
    case 'wallet_restricted':
      return props.options?.restriction_type === 'payment_dispute'
        ? t('account.topUp.reasonDispute')
        : t('account.topUp.reasonRestricted')
    default:
      return t('account.topUp.reasonDefault')
  }
}

function errorMessage(error: unknown): string {
  return userFacingErrorCopy(error, t('account.errors.networkRetry'), ACCOUNT_GUIDANCE_COPY_KEYS)
}
</script>
