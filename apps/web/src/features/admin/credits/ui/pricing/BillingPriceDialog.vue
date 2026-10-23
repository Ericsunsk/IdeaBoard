<template>
  <AdminDialogShell
    :visible="visible"
    :header="dialogHeader"
    :description="dialogDescription"
    :size="supportsPool ? 'medium' : 'compact'"
    content-layout="scrollable"
    :busy="isBusy"
    @update:visible="emit('update:visible', $event)"
  >
    <div class="flex flex-col gap-6">
      <!-- 1. 定价设置 -->
      <section class="flex flex-col gap-3">
        <h4 v-if="supportsPool" class="text-xs font-semibold text-muted-foreground">
          {{ BILLING_PRICING_MESSAGES.tabPricing }}
        </h4>

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
              :class="invalid ? 'border-destructive focus-visible:ring-destructive/20' : undefined"
              :aria-describedby="describedbyId"
              @update:model-value="showError = false"
            />
          </AdminFormField>
        </div>

        <AdminAlert v-if="errorMessage" severity="error">
          {{ errorMessage }}
        </AdminAlert>
      </section>

      <!-- 2. 模型池设置 (同页平铺展示，无分割线) -->
      <section v-if="supportsPool" class="flex flex-col gap-3">
        <h4 class="text-xs font-semibold text-muted-foreground">
          {{ BILLING_PRICING_MESSAGES.tabModelPool }}
        </h4>

        <ModelPoolEditorPanel
          ref="poolPanelRef"
          :entries="poolEntries"
          :catalog-models="poolCatalogModels"
          :catalog-loading="poolCatalogLoading"
          :pending="poolSavePending"
          :error-message="poolSaveError"
          :stale="poolSaveStale"
          :save-disabled="poolSaveDisabled"
          :catalog-error="poolCatalogError"
          @reload="emit('reloadPool')"
          @reload-catalog="emit('reloadCatalog')"
        />
      </section>
    </div>

    <template #footer>
      <AdminDialogActions
        :confirm-label="BILLING_PRICING_MESSAGES.save"
        :loading="isBusy"
        :disabled="isBusy || (supportsPool && poolSaveDisabled)"
        :cancel-disabled="isBusy"
        @cancel="close"
        @confirm="handleSave"
      />
    </template>
  </AdminDialogShell>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { AdminAlert } from '../../../shared'
import { AdminDialogActions } from '../../../shared'
import { AdminDialogShell } from '../../../shared'
import { AdminFormField } from '../../../shared'
import { AdminNumberInput } from '../../../shared'
import { formatLocaleNumber } from '@/core/i18n/formatters'
import { OUTPUT_RESOLUTION_OPTIONS } from '@/core/contracts/generationContract'
import type {
  AdminBillableItemCard,
  AdminModelPoolEntry,
  AdminPoolCatalogModel,
  PricingVariant,
} from '../../core/pricing/billableCatalogTypes'
import { serviceOfSku } from '../../core/pricing/skuFeature'
import { BILLING_PRICING_MESSAGES } from './messages'
import ModelPoolEditorPanel from './ModelPoolEditorPanel.vue'

const props = withDefaults(
  defineProps<{
    visible: boolean
    item?: AdminBillableItemCard | null
    pending?: boolean
    errorMessage?: string
    poolEntries?: AdminModelPoolEntry[] | undefined
    poolCatalogModels?: AdminPoolCatalogModel[] | undefined
    poolCatalogLoading?: boolean | undefined
    poolSavePending?: boolean | undefined
    poolSaveError?: string | undefined
    poolSaveStale?: boolean | undefined
    poolSaveDisabled?: boolean | undefined
    poolCatalogError?: string | undefined
  }>(),
  {
    item: null,
    pending: false,
    errorMessage: '',
    poolEntries: () => [],
    poolCatalogModels: () => [],
    poolCatalogLoading: false,
    poolSavePending: false,
    poolSaveError: '',
    poolSaveStale: false,
    poolSaveDisabled: false,
    poolCatalogError: '',
  },
)

const emit = defineEmits<{
  'update:visible': [visible: boolean]
  save: [
    payload: {
      prices: Record<PricingVariant, number | undefined>
      poolEntries?: AdminModelPoolEntry[] | undefined
    },
  ]
  reloadPool: []
  reloadCatalog: []
}>()

const maxPriceCredits = 9_000_000_000_000_000

const supportsPool = computed(() => (props.item ? Boolean(serviceOfSku(props.item.sku)) : false))
const poolPanelRef = ref<InstanceType<typeof ModelPoolEditorPanel> | null>(null)

const isBusy = computed(() => Boolean(props.pending || props.poolSavePending))

const dialogHeader = computed(() => BILLING_PRICING_MESSAGES.priceHeader(props.item?.displayName))

const dialogDescription = computed(() =>
  supportsPool.value
    ? BILLING_PRICING_MESSAGES.settingsDescription
    : BILLING_PRICING_MESSAGES.priceDescription,
)

const resolutions = computed<PricingVariant[]>(
  () => props.item?.pricingVariants ?? [...OUTPUT_RESOLUTION_OPTIONS],
)
const requiredVariant = computed(() => resolutions.value[0] ?? OUTPUT_RESOLUTION_OPTIONS[0])
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
    for (const variant of OUTPUT_RESOLUTION_OPTIONS) {
      prices.value[variant] = props.item?.currentPrices[variant]?.unitCredits ?? null
    }
    showError.value = false
  },
)

function close() {
  if (!isBusy.value) emit('update:visible', false)
}

function handleSave() {
  showError.value = true
  if (props.pending || props.poolSavePending) return

  const hasPricingError = Object.values(validationErrors.value).some(Boolean)
  let poolEntries: AdminModelPoolEntry[] | undefined
  let hasPoolError = false

  if (supportsPool.value && poolPanelRef.value) {
    const isValid = poolPanelRef.value.checkPoolValidity()
    if (!isValid) {
      hasPoolError = true
    } else {
      poolEntries = poolPanelRef.value.getDraftEntries()
    }
  }

  if (hasPricingError || hasPoolError) return

  const nextPrices = {} as Record<PricingVariant, number | undefined>
  for (const variant of resolutions.value) {
    nextPrices[variant] = prices.value[variant] ?? undefined
  }

  emit('save', { prices: nextPrices, poolEntries })
}
</script>
