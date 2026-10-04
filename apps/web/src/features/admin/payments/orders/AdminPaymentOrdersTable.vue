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
      field="createdAt"
      :header="PAYMENT_ORDERS_MESSAGES.createdAtColumn"
      :style="{ width: '160px' }"
    >
      <template #body="{ data }">
        <time
          :datetime="data.createdAt"
          class="whitespace-nowrap font-mono text-xs text-muted-foreground"
        >
          {{ formatPaymentDateTime(data.createdAt) }}
        </time>
      </template>
    </AdminColumn>

    <AdminColumn
      field="publicOrderNumber"
      :header="PAYMENT_ORDERS_MESSAGES.orderNumberColumn"
      :style="{ minWidth: '220px' }"
    >
      <template #body="{ data }">
        <button
          type="button"
          class="cursor-pointer rounded font-mono text-xs font-semibold text-foreground transition-colors hover:text-primary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/20"
          :aria-label="PAYMENT_ORDERS_MESSAGES.viewOrderAria(data.publicOrderNumber)"
          @click="emit('open-detail', data.publicOrderNumber)"
        >
          {{ data.publicOrderNumber }}
        </button>
      </template>
    </AdminColumn>

    <AdminColumn :header="PAYMENT_ORDERS_MESSAGES.userColumn" :style="{ width: '160px' }">
      <template #body="{ data }">
        <div class="truncate font-medium text-foreground">{{ data.user.displayName }}</div>
      </template>
    </AdminColumn>

    <AdminColumn
      field="ideaQuantity"
      :header="PAYMENT_ORDERS_MESSAGES.ideaColumn"
      :style="{ width: '120px' }"
    >
      <template #body="{ data }">
        <span class="font-mono text-xs font-semibold text-foreground">{{
          formatLocaleNumber(data.ideaQuantity)
        }}</span>
      </template>
    </AdminColumn>

    <AdminColumn :header="PAYMENT_ORDERS_MESSAGES.paidColumn" :style="{ width: '130px' }">
      <template #body="{ data }">
        <span class="font-mono text-xs font-semibold text-foreground">{{
          formatPaymentMoney(data.paidAmountMinor, data.paidCurrency)
        }}</span>
      </template>
    </AdminColumn>

    <AdminColumn
      field="stripeStatus"
      :header="PAYMENT_ORDERS_MESSAGES.stripeColumn"
      :style="{ width: '120px' }"
    >
      <template #body="{ data }">
        <AdminBadge
          :value="getStripeStatusLabel(data.stripeStatus)"
          :severity="stripeStatusSeverity(data.stripeStatus)"
          rounded
        />
      </template>
    </AdminColumn>

    <AdminColumn
      field="creditStatus"
      :header="PAYMENT_ORDERS_MESSAGES.creditColumn"
      :style="{ width: '120px' }"
    >
      <template #body="{ data }">
        <AdminBadge
          :value="getCreditStatusLabel(data.creditStatus)"
          :severity="creditStatusSeverity(data.creditStatus)"
          rounded
        />
      </template>
    </AdminColumn>

    <AdminColumn :header="PAYMENT_ORDERS_MESSAGES.anomalyColumn" :style="{ width: '110px' }">
      <template #body="{ data }">
        <AdminBadge
          v-if="data.anomalousAt"
          :value="PAYMENT_ORDERS_MESSAGES.anomalyBadge"
          severity="danger"
          rounded
        />
        <span v-else class="text-xs text-muted-foreground">—</span>
      </template>
    </AdminColumn>

    <AdminColumn :header="PAYMENT_ORDERS_MESSAGES.detailColumn" :style="{ width: '64px' }">
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
import AdminBadge from '../../shared/components/AdminBadge.vue'
import AdminColumn from '../../shared/components/AdminColumn.vue'
import AdminEmptyState from '../../shared/components/AdminEmptyState.vue'
import AdminTable from '../../shared/components/AdminTable.vue'
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

function stripeStatusSeverity(status: string) {
  if (status === 'paid') return 'success'
  if (status === 'payment_failed') return 'danger'
  if (status === 'open' || status === 'async_payment_pending') return 'warn'
  return 'secondary'
}

function creditStatusSeverity(status: string) {
  if (status === 'credited') return 'success'
  if (status === 'pending') return 'warn'
  return 'secondary'
}
</script>
