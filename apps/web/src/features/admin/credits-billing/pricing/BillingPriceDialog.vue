<template>
  <AdminDialogShell
    :visible="visible"
    :header="`发布价格 · ${item?.displayName ?? ''}`"
    size="compact"
    :busy="pending"
    @update:visible="emit('update:visible', $event)"
  >
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
        <InputNumber
          v-model="prices[variant]"
          :input-id="`billing-feature-price-${variant}`"
          fluid
          :min="0"
          :max="maxPriceCredits"
          :use-grouping="true"
          :placeholder="variant === requiredVariant ? '必填' : '未配置'"
          :suffix="prices[variant] === 1 ? ' Idea' : ' Ideas'"
          :invalid="invalid"
          :aria-describedby="describedbyId"
          @update:model-value="showError = false"
        />
      </AdminFormField>
    </div>

    <dl class="grid gap-3 border-y border-ib-border-soft py-3" :class="resolutions.length === 1 ? 'grid-cols-1' : 'grid-cols-3'">
      <div v-for="variant in resolutions" :key="variant" class="min-w-0">
        <dt class="text-admin-caption text-ib-text-muted">{{ variantLabel(variant) }}</dt>
        <dd class="mt-1 truncate text-admin-meta font-semibold text-admin-title-color">
          {{ currentPriceLabel(variant) }} → {{ newPriceLabel(variant) }}
        </dd>
      </div>
    </dl>

    <div class="flex items-center justify-between gap-4 text-admin-meta">
      <span class="text-ib-text-muted">版本影响</span>
      <span class="text-right text-admin-title-color">创建新版本，历史版本不变</span>
    </div>

    <Message v-if="errorMessage" severity="error" :closable="false" role="alert">
      {{ errorMessage }}
    </Message>

    <template #footer>
      <Button label="取消" severity="secondary" outlined :disabled="pending" @click="close" />
      <Button label="发布价格" :loading="pending" :disabled="pending" @click="submit" />
    </template>
  </AdminDialogShell>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import Button from 'primevue/button'
import InputNumber from 'primevue/inputnumber'
import Message from 'primevue/message'
import AdminDialogShell from '../../shared/components/AdminDialogShell.vue'
import AdminFormField from '../../shared/components/AdminFormField.vue'
import { formatIdeaAmount } from '../../../../shared/utils/ideaCurrency'
import type { AdminBillableItemCard, PricingVariant } from './billableCatalogTypes'

const props = withDefaults(
  defineProps<{
    visible: boolean
    item?: AdminBillableItemCard | null
    pending?: boolean
    errorMessage?: string
  }>(),
  { item: null, pending: false, errorMessage: '' },
)

const emit = defineEmits<{
  'update:visible': [visible: boolean]
  submit: [prices: Record<PricingVariant, number | undefined>]
}>()

const maxPriceCredits = 9_000_000_000_000_000

const resolutions = computed<PricingVariant[]>(() => props.item?.pricingVariants ?? ['1k', '2k', '4k'])
const requiredVariant = computed(() => resolutions.value[0] ?? '1k')
const prices = ref<Record<PricingVariant, number | null>>({ default: null, '1k': null, '2k': null, '4k': null })
const showError = ref(false)
const validationErrors = computed<Record<PricingVariant, string>>(() => {
  const errors = {} as Record<PricingVariant, string>
  for (const variant of resolutions.value) {
    const value = prices.value[variant]
    if (variant === requiredVariant.value && value == null) errors[variant] = '请输入价格'
    else if (value != null && (!Number.isSafeInteger(value) || value < 0 || value > maxPriceCredits)) {
      errors[variant] = `价格必须是 0 到 ${maxPriceCredits.toLocaleString('zh-CN')} 之间的整数`
    } else errors[variant] = ''
  }
  return errors
})
function variantLabel(variant: PricingVariant) {
  return variant === 'default' ? '固定' : variant.toUpperCase()
}
function currentPriceLabel(variant: PricingVariant) {
  const value = props.item?.currentPrices[variant]?.unitCredits
  return value == null ? '未设置' : formatIdeaAmount(value)
}
function newPriceLabel(variant: PricingVariant) {
  const value = prices.value[variant]
  return value == null ? '未配置' : formatIdeaAmount(value)
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
