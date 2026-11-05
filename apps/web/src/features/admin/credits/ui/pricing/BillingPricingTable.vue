<template>
  <section :aria-label="BILLING_PRICING_MESSAGES.tableAria(props.title)">
    <div v-if="props.title" class="mb-2 flex items-center">
      <h3 class="text-sm font-bold text-foreground">{{ props.title }}</h3>
    </div>
    <div :class="adminTableCardClass">
      <AdminTable
        :value="props.items"
        :loading="props.loading"
        data-key="sku"
        scrollable
        :frame-class="adminTableFrameClass"
        :table-style="adminTableStyle"
        :class="adminTableClass"
      >
        <template #empty>
          <AdminEmptyState :title="BILLING_PRICING_MESSAGES.tableEmpty" />
        </template>

        <AdminColumn
          field="service"
          header="Service"
          :style="{ width: billingPricingColumnWidths.service }"
        >
          <template #body="{ data }">
            <BillingServiceCell :sku="data.sku" :namespace="data.namespace" />
          </template>
        </AdminColumn>

        <AdminColumn field="status" header="Status" :style="{ width: '112px', minWidth: '112px' }">
          <template #body="{ data }">
            <!-- Status 固定槽 84px（见 BillingStatusCell）+ 单元格 px-3×2=24px：
              token 的 105px 会溢出 3px 挤占 enabled 列，此处锁定 112px。
              其他列仍以 billingPricingColumnWidths 为单源。 -->
            <BillingStatusCell :status="data.status" />
          </template>
        </AdminColumn>

        <!-- 价格档位列必须显式展开：AdminTable 只认直系 AdminColumn vnode，v-for 会被 Fragment 包一层整体过滤。 -->
        <AdminColumn
          field="price-1k"
          header="1K"
          :style="{ width: billingPricingColumnWidths.variant }"
        >
          <template #body="{ data }">
            <span :class="priceValueClass(formatPricingCellValue(data, '1k'))">{{
              formatPricingCellValue(data, '1k')
            }}</span>
          </template>
        </AdminColumn>

        <AdminColumn
          field="price-2k"
          header="2K"
          :style="{ width: billingPricingColumnWidths.variant }"
        >
          <template #body="{ data }">
            <span :class="priceValueClass(formatPricingCellValue(data, '2k'))">{{
              formatPricingCellValue(data, '2k')
            }}</span>
          </template>
        </AdminColumn>

        <AdminColumn
          field="price-4k"
          header="4K"
          :style="{ width: billingPricingColumnWidths.variant }"
        >
          <template #body="{ data }">
            <span :class="priceValueClass(formatPricingCellValue(data, '4k'))">{{
              formatPricingCellValue(data, '4k')
            }}</span>
          </template>
        </AdminColumn>

        <AdminColumn
          field="model-pool"
          :header="BILLING_PRICING_MESSAGES.poolColumn"
          :style="{ width: billingPricingColumnWidths.pool }"
        >
          <template #body="{ data }">
            <ModelPoolSummaryCell
              :display-name="data.displayName"
              :pool="props.poolBySku[data.sku]"
              :failed="props.poolFailed[data.sku] === true"
              class="min-w-0"
            />
          </template>
        </AdminColumn>

        <AdminColumn
          field="actions"
          :header="BILLING_PRICING_MESSAGES.tableActionsColumn"
          :style="{ width: billingPricingColumnWidths.actions }"
        >
          <template #body="{ data }">
            <div v-if="data.status !== 'archived'" class="flex items-center gap-2">
              <ActionButton @click="emit('publish', data)">
                {{ BILLING_PRICING_MESSAGES.tablePublishLabel(data.status) }}
              </ActionButton>
            </div>
            <div v-else>
              <ActionButton
                variant="destructive"
                :loading="props.deletePending[data.sku] === true"
                :disabled="props.deletePending[data.sku] === true"
                @click="emit('delete', data)"
              >
                {{ BILLING_PRICING_MESSAGES.tableDelete }}
              </ActionButton>
            </div>
          </template>
        </AdminColumn>

        <AdminColumn
          field="enabled"
          :header="BILLING_PRICING_MESSAGES.enable"
          :style="{ width: billingPricingColumnWidths.enabled }"
        >
          <template #body="{ data }">
            <AdminSwitch
              :model-value="data.status === 'active'"
              :disabled="
                (data.status !== 'active' && data.status !== 'disabled') ||
                props.availabilityPending[data.sku] === true
              "
              :aria-label="BILLING_PRICING_MESSAGES.tableStatusAria(data.displayName)"
              @update:model-value="emit('availability', data, $event)"
            />
          </template>
        </AdminColumn>
      </AdminTable>
    </div>
  </section>
</template>

<script setup lang="ts">
import { AdminTable } from '../../../shared'
import { AdminColumn } from '../../../shared'
import { AdminEmptyState } from '../../../shared'
import ActionButton from '@/core/components/ActionButton.vue'
import { AdminSwitch } from '../../../shared'
import {
  adminTableCardClass,
  adminTableClass,
  adminTableFrameClass,
  adminTableStyle,
  adminTableMutedValueClass,
} from '../../../shared'
import BillingStatusCell from './BillingStatusCell.vue'
import {
  billingPricingColumnWidths,
  formatPricingCellValue,
} from '../../core/pricing/billingPricingColumns'
import BillingServiceCell from './BillingServiceCell.vue'
import ModelPoolSummaryCell from './ModelPoolSummaryCell.vue'
import type { AdminBillableItemCard, AdminModelPool } from '../../core/pricing/billableCatalogTypes'
import { BILLING_PRICING_MESSAGES } from './messages'

// 对齐任务监控页事件表：同壳、同列模式；分组与状态语义归 pricingColumns 单源。
const props = withDefaults(
  defineProps<{
    title?: string | undefined
    items: AdminBillableItemCard[]
    loading?: boolean | undefined
    availabilityPending?: Record<string, boolean | undefined> | undefined
    deletePending?: Record<string, boolean | undefined> | undefined
    poolBySku?: Record<string, AdminModelPool | undefined> | undefined
    poolFailed?: Record<string, boolean | undefined> | undefined
  }>(),
  {
    loading: false,
    availabilityPending: () => ({}),
    deletePending: () => ({}),
    title: undefined,
    poolBySku: () => ({}),
    poolFailed: () => ({}),
  },
)

const emit = defineEmits<{
  publish: [item: AdminBillableItemCard]
  availability: [item: AdminBillableItemCard, enabled: boolean]
  delete: [item: AdminBillableItemCard]
}>()

// 未设置态与 Model 列未设置同样式（灰 mono 非粗体），有值才高亮。
// 数值列按 E6 追加 tabular-nums + whitespace-nowrap（ledger/orders 范本）。
// 「未设置 / —」的档位语义区分保留，不收敛（见 billingPricingColumns.ts）。
function priceValueClass(value: string): string {
  return value === '—' || value === '未设置'
    ? adminTableMutedValueClass
    : 'truncate font-mono text-xs font-bold tabular-nums whitespace-nowrap text-foreground'
}
</script>
