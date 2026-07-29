<template>
  <Card :class="adminMasonryCardClass" :pt="adminCardPt">
    <template #title>
      <div class="flex min-w-0 items-center justify-between gap-3">
        <span class="truncate text-admin-title-sm font-bold text-admin-title-color">
          {{ item.displayName }}
        </span>
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
        <p class="m-0 min-h-10 text-admin-body leading-5 text-ib-text-muted">
          {{ item.description || '暂无功能说明' }}
        </p>

        <div class="flex items-end justify-between gap-4 border-t border-ib-border-soft pt-4">
          <div class="min-w-0">
            <div class="text-admin-meta text-ib-text-muted">当前价格</div>
            <div class="mt-1 text-admin-title font-bold text-admin-title-color">
              {{ priceLabel }}
            </div>
          </div>
          <div
            v-if="item.updatedAt"
            class="shrink-0 text-right text-admin-caption text-ib-text-muted"
          >
            {{ updatedAtLabel }}
          </div>
          <Button
            v-if="item.status !== 'archived'"
            size="small"
            severity="secondary"
            outlined
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
          class="flex min-h-8 items-center justify-end gap-2 border-t border-ib-border-soft pt-4"
        >
          <Button
            size="small"
            severity="secondary"
            text
            label="价格历史"
            @click="emit('history', item)"
          />
          <Button
            v-if="item.status === 'archived'"
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
import { adminCardPt } from '../../shared/lib/adminCardPt'
import { adminCompactTagClass, adminMasonryCardClass } from '../../shared/lib/adminUiClasses'
import type { AdminBillableItemCard, AdminBillableItemStatus } from './billableCatalogTypes'

const props = defineProps<{
  item: AdminBillableItemCard
  availabilityPending?: boolean
  deletePending?: boolean
}>()
const emit = defineEmits<{
  publish: [item: AdminBillableItemCard]
  availability: [item: AdminBillableItemCard, enabled: boolean]
  history: [item: AdminBillableItemCard]
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
const priceLabel = computed(() =>
  props.item.currentPrice ? formatIdeaAmount(props.item.currentPrice.unitCredits) : '未设置',
)
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
