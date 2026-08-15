<template>
  <AdminDialogShell
    :visible="visible"
    :header="`发布价格 · ${item?.displayName ?? ''}`"
    size="compact"
    :busy="pending"
    @update:visible="emit('update:visible', $event)"
  >
    <div class="grid grid-cols-3 gap-3">
      <AdminFormField
        v-for="resolution in resolutions"
        :key="resolution"
        v-slot="{ describedbyId, invalid }"
        :label="resolution.toUpperCase()"
        :for="`billing-feature-price-${resolution}`"
        :error="showError ? validationErrors[resolution] : ''"
        :required="resolution === '1k'"
      >
        <InputNumber
          v-model="prices[resolution]"
          :input-id="`billing-feature-price-${resolution}`"
          fluid
          :min="0"
          :max="maxPriceCredits"
          :use-grouping="true"
          :placeholder="resolution === '1k' ? '必填' : '未配置'"
          :suffix="prices[resolution] === 1 ? ' Idea' : ' Ideas'"
          :invalid="invalid"
          :aria-describedby="describedbyId"
          @update:model-value="showError = false"
        />
      </AdminFormField>
    </div>

    <dl class="grid grid-cols-3 gap-3 border-y border-ib-border-soft py-3">
      <div v-for="resolution in resolutions" :key="resolution" class="min-w-0">
        <dt class="text-admin-caption text-ib-text-muted">{{ resolution.toUpperCase() }}</dt>
        <dd class="mt-1 truncate text-admin-meta font-semibold text-admin-title-color">
          {{ currentPriceLabel(resolution) }} → {{ newPriceLabel(resolution) }}
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
import type { AdminBillableItemCard, OutputResolution } from './billableCatalogTypes'

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
  submit: [prices: Record<OutputResolution, number | undefined>]
}>()

const maxPriceCredits = 9_000_000_000_000_000

const resolutions: OutputResolution[] = ['1k', '2k', '4k']
const prices = ref<Record<OutputResolution, number | null>>({ '1k': null, '2k': null, '4k': null })
const showError = ref(false)
const validationErrors = computed<Record<OutputResolution, string>>(() => {
  const errors = {} as Record<OutputResolution, string>
  for (const resolution of resolutions) {
    const value = prices.value[resolution]
    if (resolution === '1k' && value == null) errors[resolution] = '请输入价格'
    else if (value != null && (!Number.isSafeInteger(value) || value < 0 || value > maxPriceCredits)) {
      errors[resolution] = `价格必须是 0 到 ${maxPriceCredits.toLocaleString('zh-CN')} 之间的整数`
    } else errors[resolution] = ''
  }
  return errors
})
function currentPriceLabel(resolution: OutputResolution) {
  const value = props.item?.currentPrices[resolution]?.unitCredits
  return value == null ? '未设置' : formatIdeaAmount(value)
}
function newPriceLabel(resolution: OutputResolution) {
  const value = prices.value[resolution]
  return value == null ? '未配置' : formatIdeaAmount(value)
}

watch(
  () => props.visible,
  (visible) => {
    if (!visible) return
    for (const resolution of resolutions) {
      prices.value[resolution] = props.item?.currentPrices[resolution]?.unitCredits ?? null
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
  emit('submit', {
    '1k': prices.value['1k'] as number,
    '2k': prices.value['2k'] ?? undefined,
    '4k': prices.value['4k'] ?? undefined,
  })
}
</script>
