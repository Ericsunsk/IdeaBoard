<template>
  <section
    class="mb-4 min-w-0 overflow-hidden rounded-[var(--radius-ib-panel)] border border-admin-border-soft bg-ib-surface"
    aria-labelledby="billing-coverage-title"
  >
    <div
      class="flex flex-wrap items-start justify-between gap-3 border-b border-admin-border-soft px-4 py-3"
    >
      <div class="min-w-0">
        <div class="flex flex-wrap items-center gap-2">
          <h2
            id="billing-coverage-title"
            class="m-0 text-admin-title-sm font-bold text-admin-title-color"
          >
            AI Workflow 计费覆盖
          </h2>
          <Tag :value="`${items.length} 个组合`" severity="info" rounded />
          <Tag v-if="gapCount > 0" :value="`${gapCount} 个缺口`" severity="warn" rounded />
          <Tag v-else-if="items.length > 0" value="全部可计价" severity="success" rounded />
        </div>
        <p class="m-0 mt-0.5 text-admin-caption text-admin-text-3">
          服务端解析可执行的 feature × model，并核对 canonical SKU 与 current price。
        </p>
      </div>
      <Button
        text
        :class="adminFramedIconButtonClass"
        aria-label="刷新 AI Workflow 计费覆盖"
        :loading="loading"
        @click="emit('refresh')"
      >
        <template #icon><Refresh class="size-4 text-admin-text-2" /></template>
      </Button>
    </div>

    <Message v-if="error" severity="error" :closable="false" class="!m-4 !mb-0">
      {{ error }}。Catalog 仍可独立操作，请刷新覆盖检查重试。
    </Message>

    <DataTableFrame
      :value="items"
      :loading="loading"
      data-key="sku"
      scrollable
      scroll-height="320px"
      frame-class="min-h-[180px]"
      :table-style="{ minWidth: '980px' }"
    >
      <Column header="Workflow" :style="{ minWidth: '190px' }">
        <template #body="{ data: item }">
          <span class="block text-admin-body font-bold text-admin-text-1">
            {{ featureLabel(item) }}
          </span>
          <code class="mt-0.5 block text-admin-caption text-admin-text-3">
            {{ item.feature }}
          </code>
        </template>
      </Column>
      <Column field="model" header="Model" :style="{ minWidth: '180px' }">
        <template #body="{ data: item }">
          <code class="text-admin-body font-semibold text-admin-text-1">{{ item.model }}</code>
        </template>
      </Column>
      <Column header="Canonical SKU" :style="{ minWidth: '310px' }">
        <template #body="{ data: item }">
          <code
            class="block max-w-[340px] truncate text-admin-caption text-admin-text-2"
            :title="item.sku"
          >
            {{ item.sku }}
          </code>
        </template>
      </Column>
      <Column header="单价" :style="{ width: '110px' }">
        <template #body="{ data: item }">
          <span
            v-if="item.unitCredits !== undefined"
            class="font-mono text-admin-body font-bold text-admin-text-1"
          >
            {{ formatIdeaAmount(item.unitCredits) }}
          </span>
          <span v-else class="text-admin-meta text-admin-text-3">—</span>
        </template>
      </Column>
      <Column header="覆盖状态" :style="{ width: '190px' }">
        <template #body="{ data: item }">
          <Tag
            :value="coverageStatus(item).label"
            :severity="coverageStatus(item).severity"
            rounded
            :class="adminCompactTagClass"
          />
        </template>
      </Column>
      <Column header="操作" :style="{ width: '150px' }">
        <template #body="{ data: item }">
          <Button
            v-if="!item.skuConfigured"
            label="配置 SKU"
            size="small"
            severity="secondary"
            outlined
            :aria-label="`为 ${item.model} 配置 SKU`"
            @click="emit('configure-sku', item)"
          />
          <span v-else class="text-admin-meta text-admin-text-3">—</span>
        </template>
      </Column>
      <template #empty>
        <div :class="adminEmptyStateClass">
          <span class="font-semibold text-admin-title-color">没有可执行的 AI Workflow × model</span>
          <span class="mt-1 text-admin-meta"
            >启用 Provider Account 与 model 后再刷新覆盖检查。</span
          >
        </div>
      </template>
    </DataTableFrame>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { Refresh } from '@iconoir/vue'
import Button from 'primevue/button'
import Column from 'primevue/column'
import Message from 'primevue/message'
import Tag from 'primevue/tag'
import { WORKFLOW_FEATURE_REGISTRY_MAP } from '../../../../shared/domain/workflowFeatureRegistry'
import { formatIdeaAmount } from '../../../../shared/utils/ideaCurrency'
import DataTableFrame from '../../../../shared/components/DataTableFrame.vue'
import {
  adminCompactTagClass,
  adminEmptyStateClass,
  adminFramedIconButtonClass,
} from '../../shared/lib/adminUiClasses'
import type { AdminBillingCoverageItem } from './billingPricingTypes'

const props = defineProps<{
  items: AdminBillingCoverageItem[]
  loading: boolean
  error: string
}>()

const emit = defineEmits<{
  refresh: []
  'configure-sku': [item: AdminBillingCoverageItem]
}>()

const gapCount = computed(
  () => props.items.filter((item) => coverageStatus(item).label !== '可计价').length,
)

function coverageStatus(item: AdminBillingCoverageItem) {
  if (!item.skuConfigured) return { label: 'SKU 未创建', severity: 'danger' as const }
  if (!item.skuEnabled) return { label: 'SKU disabled', severity: 'secondary' as const }
  if (!item.priceConfigured) return { label: '无 current price', severity: 'warn' as const }
  if (!item.priceEnabled) return { label: 'current price disabled', severity: 'danger' as const }
  return { label: '可计价', severity: 'success' as const }
}

function featureLabel(item: AdminBillingCoverageItem) {
  return WORKFLOW_FEATURE_REGISTRY_MAP[item.feature].label
}
</script>
