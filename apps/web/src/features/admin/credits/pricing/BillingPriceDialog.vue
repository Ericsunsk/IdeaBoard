<template>
  <AdminDialogShell
    :visible="visible"
    :header="`编辑 · ${item?.displayName ?? ''}`"
    description="调整该计费项的价格配置后保存。"
    size="compact"
    content-layout="plain"
    :busy="pending"
    @update:visible="emit('update:visible', $event)"
  >
    <div class="flex flex-col gap-4">
      <div v-if="modelFeature && item" class="flex flex-col gap-1.5">
        <div class="flex items-center justify-between">
          <label
            class="text-admin-meta font-[680] text-admin-text-2"
            for="model-select-dialog-model"
          >
            模型
          </label>
        </div>
        <ModelMappingRow
          id="dialog-model"
          :title="item.displayName"
          :model-value="modelValue"
          :model-pending="modelPending"
          :model-error="modelError"
          :available-models="availableModels"
          @select="emit('save-model', $event)"
        />
      </div>

      <div class="grid gap-3" :class="resolutions.length === 1 ? 'grid-cols-1' : 'grid-cols-3'">
        <AdminFormField
          v-for="variant in resolutions"
          :key="variant"
          v-slot="{ describedbyId, invalid }"
          :label="variantLabel(variant)"
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
            :placeholder="variant === requiredVariant ? '必填' : '未配置'"
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
        取消
      </AdminButton>
      <AdminButton
        :class="adminActionButtonClass"
        :loading="pending"
        :disabled="pending"
        @click="submit"
      >
        保存
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
import ModelMappingRow from './ModelMappingRow.vue'
import { adminActionButtonClass } from '../../shared/lib/uiClasses'
import { pricingMappingFeature } from './groups'
import { formatLocaleNumber } from '../../../../shared/i18n/formatters'
import type { AdminModelItem } from '../../models/domain/catalogData'
import type { AdminBillableItemCard, PricingVariant } from './billableCatalogTypes'

const props = withDefaults(
  defineProps<{
    visible: boolean
    item?: AdminBillableItemCard | null
    pending?: boolean
    errorMessage?: string
    modelValue?: string | undefined
    modelPending?: boolean | undefined
    modelError?: string | undefined
    availableModels?: AdminModelItem[] | undefined
  }>(),
  {
    item: null,
    pending: false,
    errorMessage: '',
    modelValue: undefined,
    modelPending: undefined,
    modelError: undefined,
    availableModels: () => [],
  },
)

const emit = defineEmits<{
  'update:visible': [visible: boolean]
  submit: [prices: Record<PricingVariant, number | undefined>]
  'save-model': [modelId: string]
}>()

// 映射特性：ai-image 看后缀约定，ai-search 看冒号后缀；null（如已归档 SKU）无映射行，只设价格。
const modelFeature = computed(() => (props.item ? pricingMappingFeature(props.item.sku) : null))

const maxPriceCredits = 9_000_000_000_000_000

const resolutions = computed<PricingVariant[]>(
  () => props.item?.pricingVariants ?? ['1k', '2k', '4k'],
)
const requiredVariant = computed(() => resolutions.value[0] ?? '1k')
const prices = ref<Record<PricingVariant, number | null>>({
  default: null,
  '1k': null,
  '2k': null,
  '4k': null,
})
const showError = ref(false)
const validationErrors = computed<Record<PricingVariant, string>>(() => {
  const errors = {} as Record<PricingVariant, string>
  for (const variant of resolutions.value) {
    const value = prices.value[variant]
    if (variant === requiredVariant.value && value == null) errors[variant] = '请输入价格'
    else if (
      value != null &&
      (!Number.isSafeInteger(value) || value < 0 || value > maxPriceCredits)
    ) {
      errors[variant] = `价格必须是 0 到 ${formatLocaleNumber(maxPriceCredits)} 之间的整数`
    } else errors[variant] = ''
  }
  return errors
})
function variantLabel(variant: PricingVariant) {
  return variant === 'default' ? '固定' : variant.toUpperCase()
}

watch(
  () => props.visible,
  (visible) => {
    if (!visible) return
    for (const variant of ['default', '1k', '2k', '4k'] as PricingVariant[]) {
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
