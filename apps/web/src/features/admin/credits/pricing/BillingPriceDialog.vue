<template>
  <AdminDialogShell
    :visible="visible"
    :header="BILLING_PRICING_MESSAGES.priceHeader(item?.displayName)"
    :description="BILLING_PRICING_MESSAGES.priceDescription"
    size="compact"
    content-layout="plain"
    :busy="pending"
    @update:visible="emit('update:visible', $event)"
  >
    <div class="flex flex-col gap-4">
      <div class="grid gap-3" :class="resolutions.length === 1 ? 'grid-cols-1' : 'grid-cols-3'">
        <AdminFormField
          v-for="variant in resolutions"
          :key="variant"
          v-slot="{ describedbyId, invalid }"
          :label="BILLING_PRICING_MESSAGES.priceVariantLabel(variant)"
          :for="`billing-feature-price-${variant}`"
          :error="showError ? validationErrors[variant] : ''"
          :required="variant === requiredVariant"
        >
          <AdminNumberInput
            :id="`billing-feature-price-${variant}`"
            v-model="prices[variant]"
            :min="0"
            :max="maxPriceCredits"
            :suffix="prices[variant] === 1 ? ' Idea' : ' Ideas'"
            :placeholder="BILLING_PRICING_MESSAGES.pricePlaceholder(variant === requiredVariant)"
            :class="invalid ? 'border-admin-danger focus-visible:ring-admin-danger/20' : undefined"
            :aria-describedby="describedbyId"
            @update:model-value="showError = false"
          />
        </AdminFormField>
      </div>

      <AdminAlert v-if="errorMessage" severity="error">
        {{ errorMessage }}
      </AdminAlert>
    </div>

    <template #footer>
      <AdminButton
        variant="outline"
        :class="adminActionButtonClass"
        :disabled="pending"
        @click="close"
      >
        {{ BILLING_PRICING_MESSAGES.cancel }}
      </AdminButton>
      <AdminButton
        :class="adminActionButtonClass"
        :loading="pending"
        :disabled="pending"
        @click="submit"
      >
        {{ BILLING_PRICING_MESSAGES.save }}
      </AdminButton>
    </template>
  </AdminDialogShell>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import AdminAlert from '../../shared/components/AdminAlert.vue'
import AdminButton from '../../shared/components/AdminButton.vue'
import AdminDialogShell from '../../shared/components/AdminDialogShell.vue'
import AdminFormField from '../../shared/components/AdminFormField.vue'
import AdminNumberInput from '../../shared/components/AdminNumberInput.vue'
import { adminActionButtonClass } from '../../shared/lib/uiClasses'
import { formatLocaleNumber } from '../../../../shared/i18n/formatters'
import type { AdminBillableItemCard, PricingVariant } from './billableCatalogTypes'
import { BILLING_PRICING_MESSAGES } from './messages'

const props = withDefaults(
  defineProps<{
    visible: boolean
    item?: AdminBillableItemCard | null
    pending?: boolean
    errorMessage?: string
  }>(),
  {
    item: null,
    pending: false,
    errorMessage: '',
  },
)

const emit = defineEmits<{
  'update:visible': [visible: boolean]
  submit: [prices: Record<PricingVariant, number | undefined>]
}>()

const maxPriceCredits = 9_000_000_000_000_000

const resolutions = computed<PricingVariant[]>(
  () => props.item?.pricingVariants ?? ['1k', '2k', '4k'],
)
const requiredVariant = computed(() => resolutions.value[0] ?? '1k')
const prices = ref<Record<PricingVariant, number | null>>({
  '1k': null,
  '2k': null,
  '4k': null,
})
const showError = ref(false)
const validationErrors = computed<Record<PricingVariant, string>>(() => {
  const errors = {} as Record<PricingVariant, string>
  for (const variant of resolutions.value) {
    const value = prices.value[variant]
    if (variant === requiredVariant.value && value == null)
      errors[variant] = BILLING_PRICING_MESSAGES.priceRequired
    else if (
      value != null &&
      (!Number.isSafeInteger(value) || value < 0 || value > maxPriceCredits)
    ) {
      errors[variant] = BILLING_PRICING_MESSAGES.priceRange(formatLocaleNumber(maxPriceCredits))
    } else errors[variant] = ''
  }
  return errors
})

watch(
  () => props.visible,
  (visible) => {
    if (!visible) return
    for (const variant of ['1k', '2k', '4k'] as PricingVariant[]) {
      prices.value[variant] = props.item?.currentPrices[variant]?.unitCredits ?? null
    }
    showError.value = false
  },
)

function close() {
  if (!props.pending) emit('update:visible', false)
}

function submit() {
  showError.value = true
  if (props.pending || Object.values(validationErrors.value).some(Boolean)) return
  const nextPrices = {} as Record<PricingVariant, number | undefined>
  for (const variant of resolutions.value) nextPrices[variant] = prices.value[variant] ?? undefined
  emit('submit', nextPrices)
}
</script>
