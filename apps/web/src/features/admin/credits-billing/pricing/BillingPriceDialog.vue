<template>
  <AdminDialogShell
    :visible="visible"
    :header="`发布价格 · ${item?.displayName ?? ''}`"
    size="compact"
    :busy="pending"
    @update:visible="emit('update:visible', $event)"
  >
    <AdminFormField
      v-slot="{ describedbyId, invalid }"
      label="价格"
      for="billing-feature-price"
      :error="showError ? validationError : ''"
      required
    >
      <InputNumber
        v-model="unitCredits"
        input-id="billing-feature-price"
        fluid
        :min="0"
        :max="maxPriceCredits"
        :use-grouping="true"
        :suffix="unitCredits === 1 ? ' Idea' : ' Ideas'"
        :invalid="invalid"
        :aria-describedby="describedbyId"
        @update:model-value="showError = false"
      />
    </AdminFormField>

    <dl class="grid grid-cols-3 gap-3 border-y border-ib-border-soft py-3">
      <div class="min-w-0">
        <dt class="text-admin-caption text-ib-text-muted">当前价格</dt>
        <dd class="mt-1 truncate text-admin-meta font-semibold text-admin-title-color">
          {{ currentPriceLabel }}
        </dd>
      </div>
      <div class="min-w-0">
        <dt class="text-admin-caption text-ib-text-muted">新价格</dt>
        <dd class="mt-1 truncate text-admin-meta font-semibold text-admin-title-color">
          {{ newPriceLabel }}
        </dd>
      </div>
      <div class="min-w-0">
        <dt class="text-admin-caption text-ib-text-muted">差值</dt>
        <dd class="mt-1 truncate text-admin-meta font-semibold text-admin-title-color">
          {{ priceDeltaLabel }}
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
import type { AdminBillableItemCard } from './billableCatalogTypes'

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
  submit: [unitCredits: number]
}>()

const maxPriceCredits = 9_000_000_000_000_000

const unitCredits = ref<number | null>(null)
const showError = ref(false)
const validationError = computed(() => {
  const value = unitCredits.value
  if (value == null) return '请输入价格'
  if (!Number.isSafeInteger(value) || value < 0 || value > maxPriceCredits) {
    return `价格必须是 0 到 ${maxPriceCredits.toLocaleString('zh-CN')} 之间的整数`
  }
  return ''
})
const currentPrice = computed(() => props.item?.currentPrice?.unitCredits)
const currentPriceLabel = computed(() =>
  currentPrice.value == null ? '未设置' : formatIdeaAmount(currentPrice.value),
)
const newPriceLabel = computed(() =>
  unitCredits.value == null || validationError.value ? '待输入' : formatIdeaAmount(unitCredits.value),
)
const priceDeltaLabel = computed(() => {
  if (currentPrice.value == null || unitCredits.value == null || validationError.value) return '—'
  const delta = unitCredits.value - currentPrice.value
  if (delta === 0) return '0 Ideas'
  return `${delta > 0 ? '+' : '-'}${formatIdeaAmount(Math.abs(delta))}`
})

watch(
  () => props.visible,
  (visible) => {
    if (!visible) return
    unitCredits.value = props.item?.currentPrice?.unitCredits ?? null
    showError.value = false
  },
)

function close() {
  if (!props.pending) emit('update:visible', false)
}

function submit() {
  showError.value = true
  if (props.pending || validationError.value || unitCredits.value == null) return
  emit('submit', unitCredits.value)
}
</script>
