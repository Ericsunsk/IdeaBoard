<template>
  <AdminDialogShell
    :visible="visible"
    :header="dialogHeader"
    :description="dialogDescription"
    :size="supportsPool ? 'form' : 'compact'"
    content-layout="plain"
    :busy="isBusy"
    @update:visible="emit('update:visible', $event)"
  >
    <AdminTabs v-model="activeTab" class="w-full">
      <AdminTabsList v-if="supportsPool" class="mb-4 h-9">
        <AdminTabsTrigger value="pricing">
          {{ BILLING_PRICING_MESSAGES.tabPricing }}
        </AdminTabsTrigger>
        <AdminTabsTrigger value="model-pool">
          {{ BILLING_PRICING_MESSAGES.tabModelPool }}
        </AdminTabsTrigger>
      </AdminTabsList>

      <AdminTabsContent value="pricing">
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
                :placeholder="
                  BILLING_PRICING_MESSAGES.pricePlaceholder(variant === requiredVariant)
                "
                :class="
                  invalid ? 'border-destructive focus-visible:ring-destructive/20' : undefined
                "
                :aria-describedby="describedbyId"
                @update:model-value="showError = false"
              />
            </AdminFormField>
          </div>

          <AdminAlert v-if="errorMessage" severity="error">
            {{ errorMessage }}
          </AdminAlert>
        </div>
      </AdminTabsContent>

      <AdminTabsContent v-if="supportsPool" value="model-pool">
        <ModelPoolEditorPanel
          ref="poolPanelRef"
          :version="poolVersion"
          :entries="poolEntries"
          :catalog-models="poolCatalogModels"
          :catalog-loading="poolCatalogLoading"
          :pending="poolSavePending"
          :error-message="poolSaveError"
          :stale="poolSaveStale"
          :save-disabled="poolSaveDisabled"
          :catalog-error="poolCatalogError"
          @submit="emit('submitPool', $event)"
          @reload="emit('reloadPool')"
          @reload-catalog="emit('reloadCatalog')"
        />
      </AdminTabsContent>
    </AdminTabs>

    <template #footer>
      <AdminButton
        variant="outline"
        :class="adminActionButtonClass"
        :disabled="isBusy"
        @click="close"
      >
        {{ BILLING_PRICING_MESSAGES.cancel }}
      </AdminButton>

      <AdminButton
        v-if="activeTab === 'pricing'"
        :class="adminActionButtonClass"
        :loading="pending"
        :disabled="pending"
        @click="submitPricing"
      >
        {{ BILLING_PRICING_MESSAGES.save }}
      </AdminButton>

      <AdminButton
        v-else-if="activeTab === 'model-pool'"
        :class="adminActionButtonClass"
        :loading="poolSavePending"
        :disabled="poolSavePending || poolSaveDisabled"
        @click="poolPanelRef?.submit()"
      >
        {{ BILLING_PRICING_MESSAGES.poolSaveConfirm }}
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
import {
  AdminTabs,
  AdminTabsContent,
  AdminTabsList,
  AdminTabsTrigger,
} from '../../shared/components/adminTabsAtoms'
import { adminActionButtonClass } from '../../shared/lib/uiClasses'
import { formatLocaleNumber } from '../../../../shared/i18n/formatters'
import { OUTPUT_RESOLUTION_OPTIONS } from '../../../../shared/domain/generationContract'
import type {
  AdminBillableItemCard,
  AdminModelPoolEntry,
  AdminPoolCatalogModel,
  PricingVariant,
} from './billableCatalogTypes'
import { serviceOfSku } from './skuFeature'
import { BILLING_PRICING_MESSAGES } from './messages'
import ModelPoolEditorPanel from './ModelPoolEditorPanel.vue'

const props = withDefaults(
  defineProps<{
    visible: boolean
    item?: AdminBillableItemCard | null
    pending?: boolean
    errorMessage?: string
    poolVersion?: number | undefined
    poolEntries?: AdminModelPoolEntry[] | undefined
    poolCatalogModels?: AdminPoolCatalogModel[] | undefined
    poolCatalogLoading?: boolean | undefined
    poolSavePending?: boolean | undefined
    poolSaveError?: string | undefined
    poolSaveStale?: boolean | undefined
    poolSaveDisabled?: boolean | undefined
    poolCatalogError?: string | undefined
    initialTab?: 'pricing' | 'model-pool'
  }>(),
  {
    item: null,
    pending: false,
    errorMessage: '',
    poolVersion: 0,
    poolEntries: () => [],
    poolCatalogModels: () => [],
    poolCatalogLoading: false,
    poolSavePending: false,
    poolSaveError: '',
    poolSaveStale: false,
    poolSaveDisabled: false,
    poolCatalogError: '',
    initialTab: 'pricing',
  },
)

const emit = defineEmits<{
  'update:visible': [visible: boolean]
  submit: [prices: Record<PricingVariant, number | undefined>]
  submitPool: [entries: AdminModelPoolEntry[]]
  reloadPool: []
  reloadCatalog: []
}>()

const maxPriceCredits = 9_000_000_000_000_000

const supportsPool = computed(() => (props.item ? Boolean(serviceOfSku(props.item.sku)) : false))
const activeTab = ref<'pricing' | 'model-pool'>('pricing')
const poolPanelRef = ref<InstanceType<typeof ModelPoolEditorPanel> | null>(null)

const isBusy = computed(() =>
  activeTab.value === 'pricing' ? props.pending : Boolean(props.poolSavePending),
)

const dialogHeader = computed(() => {
  if (activeTab.value === 'model-pool') {
    return BILLING_PRICING_MESSAGES.poolDialogTitle(props.item?.displayName)
  }
  return BILLING_PRICING_MESSAGES.priceHeader(props.item?.displayName)
})

const dialogDescription = computed(() => {
  if (activeTab.value === 'model-pool') {
    return BILLING_PRICING_MESSAGES.poolDialogDescription
  }
  return BILLING_PRICING_MESSAGES.priceDescription
})

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
    activeTab.value =
      props.initialTab === 'model-pool' && supportsPool.value ? 'model-pool' : 'pricing'
    for (const variant of OUTPUT_RESOLUTION_OPTIONS) {
      prices.value[variant] = props.item?.currentPrices[variant]?.unitCredits ?? null
    }
    showError.value = false
  },
)

function close() {
  if (!isBusy.value) emit('update:visible', false)
}

function submitPricing() {
  showError.value = true
  if (props.pending || Object.values(validationErrors.value).some(Boolean)) return
  const nextPrices = {} as Record<PricingVariant, number | undefined>
  for (const variant of resolutions.value) nextPrices[variant] = prices.value[variant] ?? undefined
  emit('submit', nextPrices)
}
</script>
