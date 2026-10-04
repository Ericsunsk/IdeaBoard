<template>
  <section :aria-label="BILLING_PRICING_MESSAGES.tableAria(props.title)">
    <div v-if="props.title" class="mb-2 flex items-center gap-2">
      <h3 class="text-sm font-bold text-foreground">{{ props.title }}</h3>
      <span class="text-xs text-muted-foreground">{{
        BILLING_PRICING_MESSAGES.tableCount(props.items.length)
      }}</span>
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

        <AdminColumn
          field="status"
          header="Status"
          :style="{ width: billingPricingColumnWidths.status }"
        >
          <template #body="{ data }">
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
              <AdminButton
                size="small"
                :class="adminActionButtonClass"
                @click="emit('publish', data)"
              >
                {{ BILLING_PRICING_MESSAGES.tablePublishLabel(data.status) }}
              </AdminButton>
            </div>
            <div v-else>
              <AdminButton
                size="small"
                variant="destructive"
                :class="adminActionButtonClass"
                :loading="props.deletePending[data.sku] === true"
                :disabled="props.deletePending[data.sku] === true"
                @click="emit('delete', data)"
              >
                {{ BILLING_PRICING_MESSAGES.tableDelete }}
              </AdminButton>
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
import AdminTable from '../../shared/components/AdminTable.vue'
import AdminColumn from '../../shared/components/AdminColumn.vue'
import AdminEmptyState from '../../shared/components/AdminEmptyState.vue'
import AdminButton from '../../shared/components/AdminButton.vue'
import AdminSwitch from '../../shared/components/AdminSwitch.vue'
import {
  adminActionButtonClass,
  adminTableCardClass,
  adminTableClass,
  adminTableFrameClass,
  adminTableStyle,
} from '../../shared/lib/uiClasses'
import BillingStatusCell from './BillingStatusCell.vue'
import { billingPricingColumnWidths, formatPricingCellValue } from './billingPricingColumns'
import BillingServiceCell from './BillingServiceCell.vue'
import ModelPoolSummaryCell from './ModelPoolSummaryCell.vue'
import type { AdminBillableItemCard, AdminModelPool } from './billableCatalogTypes'
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

// 未设置态与 Model 列未配置同样式（灰 mono 非粗体），有值才高亮。
function priceValueClass(value: string): string {
  return value === '—' || value === '未设置'
    ? 'truncate font-mono text-xs text-muted-foreground'
    : 'truncate font-mono text-xs font-bold text-foreground'
}
</script>
