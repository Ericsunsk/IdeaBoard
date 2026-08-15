<template>
  <Card :class="adminMasonryCardClass" :pt="adminCardPt">
    <template #title>
      <div class="flex min-w-0 items-center justify-between gap-3">
        <span class="truncate text-admin-title-sm font-bold text-admin-title-color">
          {{ item.displayName }}
        </span>
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
    </template>

    <template #content>
      <div class="flex min-w-0 flex-col gap-5">
        <div class="flex items-end justify-between gap-4">
          <dl class="grid min-w-0 flex-1 grid-cols-3 gap-3">
            <div
              v-for="resolution in resolutions"
              :key="resolution"
              class="min-w-0 rounded-lg border border-admin-border-soft bg-admin-surface-muted/50 px-2.5 py-2"
            >
              <dt class="text-admin-caption text-ib-text-muted">{{ resolution.toUpperCase() }}</dt>
              <dd class="mt-1 truncate text-admin-meta font-bold text-admin-title-color">
                {{ priceLabel(resolution) }}
              </dd>
            </div>
          </dl>
          <div
            v-if="item.updatedAt"
            class="shrink-0 text-right text-admin-caption text-ib-text-muted"
          >
            {{ updatedAtLabel }}
          </div>
          <Button
            v-if="item.status !== 'archived'"
            size="small"
            :class="adminActionButtonClass"
            severity="contrast"
            :label="item.status === 'unpriced' ? '设置价格' : '调整价格'"
            @click="emit('publish', item)"
          />
        </div>

        <div
          v-if="item.status !== 'archived'"
          class="flex min-h-8 items-center justify-between gap-3"
        >
          <div class="min-w-0">
            <div class="text-admin-meta font-medium text-admin-title-color">功能状态</div>
            <div class="text-admin-caption text-ib-text-muted">
              {{ item.status === 'unpriced' ? '发布价格后自动启用' : '关闭后将停止新任务调用' }}
            </div>
          </div>
          <ToggleSwitch
            :model-value="item.status === 'active'"
            :disabled="item.status === 'unpriced' || availabilityPending"
            :aria-label="`${item.displayName}功能状态`"
            @update:model-value="emit('availability', item, $event)"
          />
        </div>

        <div
          v-if="item.status === 'archived'"
          class="flex min-h-8 items-center justify-end gap-2"
        >
          <Button
            size="small"
            severity="danger"
            text
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
import { formatIdeaAmount } from '../../../../shared/utils/ideaCurrency'
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
import type { AdminBillableItemCard, AdminBillableItemStatus, OutputResolution } from './billableCatalogTypes'

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
const resolutions: OutputResolution[] = ['1k', '2k', '4k']
function priceLabel(resolution: OutputResolution) {
  const price = props.item.currentPrices[resolution]
  return price ? formatIdeaAmount(price.unitCredits) : '未设置'
}
const updatedAtLabel = computed(() => {
  if (!props.item.updatedAt) return ''
  const date = new Date(props.item.updatedAt)
  if (Number.isNaN(date.getTime())) return ''
  return new Intl.DateTimeFormat('zh-CN', {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  }).format(date)
})
</script>
