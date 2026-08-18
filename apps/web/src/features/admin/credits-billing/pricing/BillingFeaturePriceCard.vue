<template>
  <Card :class="adminMasonryCardClass" :pt="adminCardPt">
    <template #title>
      <div class="flex min-w-0 items-center justify-between gap-3">
        <span class="truncate text-admin-title-sm font-bold text-admin-title-color">
          {{ item.displayName }}
        </span>
        <div class="flex shrink-0 items-center gap-2">
          <Tag
            v-if="workflowPresentation"
            :value="`${workflowPresentation.groupTitle} 节点`"
            severity="secondary"
            :class="adminCompactTagClass"
            rounded
          />
          <Tag
            :value="statusMeta.label"
            :severity="statusMeta.severity"
            :class="adminCompactTagClass"
            rounded
          />
        </div>
      </div>
    </template>

    <template #content>
      <div class="flex min-w-0 flex-col gap-5">
        <dl
          class="grid w-full min-w-0 gap-3"
          :class="resolutions.length === 1 ? 'grid-cols-1' : 'grid-cols-2 sm:grid-cols-4'"
        >
          <div
            v-for="variant in resolutions"
            :key="variant"
            class="min-w-0 rounded-lg border border-admin-border-soft bg-admin-surface-muted/50 px-2.5 py-2"
          >
            <dt class="text-admin-caption text-ib-text-muted">{{ variantLabel(variant) }}</dt>
            <dd class="mt-1 truncate text-admin-meta font-bold text-admin-title-color">
              {{ priceLabel(variant) }}
            </dd>
          </div>
        </dl>

        <div
          v-if="item.status !== 'archived'"
          class="flex min-h-8 items-center justify-between gap-3"
        >
          <Button
            size="small"
            :class="adminActionButtonClass"
            severity="contrast"
            :label="item.status === 'unpriced' ? '设置价格' : '调整价格'"
            @click="emit('publish', item)"
          />
          <ToggleSwitch
            :model-value="item.status === 'active'"
            :disabled="item.status === 'unpriced' || availabilityPending"
            :aria-label="`${item.displayName}功能状态`"
            @update:model-value="emit('availability', item, $event)"
          />
        </div>

        <div v-if="item.status === 'archived'" class="flex min-h-8 items-center justify-end gap-2">
          <Button
            size="small"
            severity="danger"
            text
            :class="adminActionButtonClass"
            label="删除"
            :loading="deletePending"
            :disabled="deletePending"
            @click="emit('delete', item)"
          />
        </div>
      </div>
    </template>
  </Card>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import Card from 'primevue/card'
import Button from 'primevue/button'
import Tag from 'primevue/tag'
import ToggleSwitch from 'primevue/toggleswitch'
import {
  getWorkflowFeatureRegistryItem,
  WORKFLOW_FEATURE_GROUP_LIST,
} from '../../../../shared/domain/workflowFeatureRegistry'
import { adminCardPt } from '../../shared/lib/adminCardPt'
import {
  adminActionButtonClass,
  adminCompactTagClass,
  adminMasonryCardClass,
} from '../../shared/lib/adminUiClasses'
import type {
  AdminBillableItemCard,
  AdminBillableItemStatus,
  PricingVariant,
} from './billableCatalogTypes'

const props = defineProps<{
  item: AdminBillableItemCard
  availabilityPending?: boolean
  deletePending?: boolean
}>()
const emit = defineEmits<{
  publish: [item: AdminBillableItemCard]
  availability: [item: AdminBillableItemCard, enabled: boolean]
  delete: [item: AdminBillableItemCard]
}>()

const statusPresentation: Record<
  AdminBillableItemStatus,
  { label: string; severity: 'info' | 'secondary' | 'success' | 'warn' }
> = {
  unpriced: { label: '未定价', severity: 'warn' },
  active: { label: '生效中', severity: 'success' },
  disabled: { label: '已停用', severity: 'secondary' },
  archived: { label: '已归档', severity: 'info' },
}

const statusMeta = computed(() => statusPresentation[props.item.status])
const workflowPresentation = computed(() => {
  const prefix = 'ai-image:'
  if (!props.item.sku.startsWith(prefix)) return null
  const feature = getWorkflowFeatureRegistryItem(props.item.sku.slice(prefix.length))
  if (!feature) return null
  const group = WORKFLOW_FEATURE_GROUP_LIST.find((item) => item.key === feature.groupKey)
  if (!group) return null
  return { groupTitle: group.title }
})
const resolutions = computed(() => props.item.pricingVariants)
const priceNumberFormat = new Intl.NumberFormat('zh-CN')
function variantLabel(variant: PricingVariant) {
  return variant === 'default' ? '固定' : variant.toUpperCase()
}
function priceLabel(variant: PricingVariant) {
  const price = props.item.currentPrices[variant]
  return price ? priceNumberFormat.format(price.unitCredits) : '未设置'
}
</script>
