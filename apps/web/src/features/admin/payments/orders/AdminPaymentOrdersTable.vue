<template>
  <AdminTable
    :value="orders"
    :loading="loading"
    data-key="publicOrderNumber"
    scrollable
    :frame-class="adminTableFrameClass"
    :table-aria-label="PAYMENT_ORDERS_MESSAGES.tableAria"
    :scroll-aria-label="PAYMENT_ORDERS_MESSAGES.tableScrollAria"
    :table-style="adminTableStyle"
    :class="adminTableClass"
  >
    <template #empty>
      <AdminEmptyState :title="emptyTitle" />
    </template>

    <AdminColumn
      field="publicOrderNumber"
      :header="PAYMENT_ORDERS_MESSAGES.orderNumberColumn"
      :style="{ minWidth: '220px' }"
    >
      <template #body="{ data }">
        <button
          type="button"
          :class="adminInlineJumpClass"
          :aria-label="PAYMENT_ORDERS_MESSAGES.viewOrderAria(data.publicOrderNumber)"
          @click="emit('open-detail', data.publicOrderNumber)"
        >
          {{ data.publicOrderNumber }}
        </button>
      </template>
    </AdminColumn>

    <AdminColumn :header="PAYMENT_ORDERS_MESSAGES.userColumn" :style="{ width: '160px' }">
      <template #body="{ data }">
        <div class="truncate font-medium text-foreground" :title="data.user.displayName">
          {{ data.user.displayName }}
        </div>
      </template>
    </AdminColumn>

    <AdminColumn
      field="ideaQuantity"
      :header="PAYMENT_ORDERS_MESSAGES.ideaColumn"
      :style="{ width: '120px' }"
    >
      <template #body="{ data }">
        <AdminMono class="text-xs font-semibold whitespace-nowrap text-foreground tabular-nums">{{
          formatLocaleNumber(data.ideaQuantity)
        }}</AdminMono>
      </template>
    </AdminColumn>

    <AdminColumn :header="PAYMENT_ORDERS_MESSAGES.paidColumn" :style="{ width: '130px' }">
      <template #body="{ data }">
        <AdminMono class="text-xs font-semibold whitespace-nowrap text-foreground tabular-nums">{{
          formatPaymentMoney(data.paidAmountMinor, data.paidCurrency)
        }}</AdminMono>
      </template>
    </AdminColumn>

    <AdminColumn
      field="stripeStatus"
      :header="PAYMENT_ORDERS_MESSAGES.stripeColumn"
      :style="{ width: '120px' }"
    >
      <template #body="{ data }">
        <AdminStatusPill
          :label="getStripeStatusLabel(data.stripeStatus)"
          :variant="stripeStatusVariant(data.stripeStatus)"
          size="small"
        />
      </template>
    </AdminColumn>

    <AdminColumn
      field="creditStatus"
      :header="PAYMENT_ORDERS_MESSAGES.creditColumn"
      :style="{ width: '120px' }"
    >
      <template #body="{ data }">
        <AdminStatusPill
          :label="getCreditStatusLabel(data.creditStatus)"
          :variant="creditStatusVariant(data.creditStatus)"
          size="small"
        />
      </template>
    </AdminColumn>

    <AdminColumn :header="PAYMENT_ORDERS_MESSAGES.anomalyColumn" :style="{ width: '110px' }">
      <template #body="{ data }">
        <AdminStatusPill
          v-if="data.anomalousAt"
          :label="PAYMENT_ORDERS_MESSAGES.anomalyBadge"
          variant="danger"
          size="small"
        />
        <span v-else class="text-xs text-muted-foreground">{{ ADMIN_NUMERIC_EMPTY }}</span>
      </template>
    </AdminColumn>

    <AdminColumn
      field="createdAt"
      :header="PAYMENT_ORDERS_MESSAGES.createdAtColumn"
      :style="{ width: '160px' }"
    >
      <template #body="{ data }">
        <AdminMono
          as="time"
          :datetime="data.createdAt"
          class="whitespace-nowrap text-xs text-muted-foreground tabular-nums"
        >
          {{ formatPaymentDateTime(data.createdAt) }}
        </AdminMono>
      </template>
    </AdminColumn>

    <AdminColumn
      :header="PAYMENT_ORDERS_MESSAGES.detailColumn"
      :style="{ width: ADMIN_TABLE_MORE_COLUMN_WIDTH }"
    >
      <template #body="{ data }">
        <div class="flex items-center justify-start">
          <button
            type="button"
            :class="adminTableRowActionButtonClass"
            :aria-label="PAYMENT_ORDERS_MESSAGES.viewOrderAria(data.publicOrderNumber)"
            @click="emit('open-detail', data.publicOrderNumber)"
          >
            <Eye class="size-4" />
          </button>
        </div>
      </template>
    </AdminColumn>
  </AdminTable>
</template>

<script setup lang="ts">
import { Eye } from '@iconoir/vue'
import AdminStatusPill, {
  type AdminStatusPillVariant,
} from '../../shared/components/AdminStatusPill.vue'
import AdminColumn from '../../shared/components/AdminColumn.vue'
import AdminEmptyState from '../../shared/components/AdminEmptyState.vue'
import AdminMono from '../../shared/components/AdminMono.vue'
import AdminTable from '../../shared/components/AdminTable.vue'
import { ADMIN_NUMERIC_EMPTY } from '../../shared/lib/cellPlaceholders'
import { adminInlineJumpClass } from '../../shared/lib/cellClasses'
import { ADMIN_TABLE_MORE_COLUMN_WIDTH } from '../../shared/lib/tableColumns'
import {
  adminTableClass,
  adminTableFrameClass,
  adminTableRowActionButtonClass,
  adminTableStyle,
} from '../../shared/lib/uiClasses'
import { formatLocaleNumber } from '../../../../shared/i18n/formatters'
import {
  formatPaymentDateTime,
  formatPaymentMoney,
  getCreditStatusLabel,
  getStripeStatusLabel,
} from './paymentOrderPresentation'
import { PAYMENT_ORDERS_MESSAGES } from './messages'
import type { AdminPaymentOrderSummary } from './paymentOrderTypes'

defineProps<{
  orders: readonly AdminPaymentOrderSummary[]
  loading: boolean
  emptyTitle: string
}>()

const emit = defineEmits<{ 'open-detail': [publicOrderNumber: string] }>()

function stripeStatusVariant(status: string): AdminStatusPillVariant {
  if (status === 'paid') return 'success'
  if (status === 'payment_failed') return 'danger'
  if (status === 'open' || status === 'async_payment_pending') return 'warning'
  return 'neutral'
}

function creditStatusVariant(status: string): AdminStatusPillVariant {
  if (status === 'credited') return 'success'
  if (status === 'pending') return 'warning'
  return 'neutral'
}
</script>
