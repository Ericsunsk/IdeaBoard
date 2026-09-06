<template>
  <section class="grid min-w-0 gap-3" aria-labelledby="recharge-history-heading">
    <h3 id="recharge-history-heading" class="m-0 text-lg font-bold text-ib-text">
      {{ t('account.topUp.historyTitle') }}
    </h3>
    <DataTableFrame
      :value="tableRows"
      scrollable
      :table-style="{ minWidth: '720px' }"
      :table-aria-label="t('account.topUp.historyTableAria')"
      :scroll-aria-label="t('account.topUp.historyTableAria')"
      frame-class="rounded-[var(--radius-ib-panel)] border border-ib-border-soft bg-ib-surface shadow-[var(--shadow-ib-control)]"
      :empty-title="t('account.topUp.historyEmptyTitle')"
    >
      <Column
        field="occurred_at"
        :header="t('account.topUp.columnTime')"
        :style="{ width: '180px' }"
      >
        <template #body="{ data }"
          ><Skeleton v-if="isSkeleton(data)" width="8rem" /><time
            v-else
            class="whitespace-nowrap font-mono text-[13px] text-ib-text-muted"
            >{{ formatDateTime(data.occurred_at) }}</time
          ></template
        >
      </Column>
      <Column field="idea_quantity" header="Ideas" :style="{ width: '130px' }">
        <template #body="{ data }"
          ><Skeleton v-if="isSkeleton(data)" width="4rem" /><span
            v-else
            class="font-mono font-bold text-ib-text"
            >{{ formatLocaleNumber(data.idea_quantity) }}</span
          ></template
        >
      </Column>
      <Column
        field="paid_amount_minor"
        :header="t('account.topUp.columnPaid')"
        :style="{ width: '170px' }"
      >
        <template #body="{ data }"
          ><Skeleton v-if="isSkeleton(data)" width="6rem" /><span
            v-else-if="data.paid_amount_minor !== undefined && data.paid_currency"
            class="whitespace-nowrap font-mono font-semibold text-ib-text-muted"
            >{{ formatCurrencyMinor(data.paid_amount_minor, data.paid_currency) }}</span
          ><span v-else class="font-semibold text-ib-text-subtle">{{
            t('account.topUp.unpaid')
          }}</span></template
        >
      </Column>
      <Column field="status" :header="t('account.topUp.columnStatus')" :style="{ width: '190px' }">
        <template #body="{ data }"
          ><Skeleton v-if="isSkeleton(data)" width="4rem" /><span
            v-else
            class="font-semibold"
            :class="data.status === 'credited' ? 'text-ib-success' : 'text-ib-text-muted'"
            >{{ rechargeStatusLabel(data) }}</span
          ></template
        >
      </Column>
      <Column :header="t('account.topUp.columnReceipt')" :style="{ width: '100px' }">
        <template #body="{ data }"
          ><Skeleton v-if="isSkeleton(data)" width="3rem" /><a
            v-else-if="data.receipt_url"
            :href="data.receipt_url"
            target="_blank"
            rel="noopener noreferrer"
            class="font-semibold text-ib-creative-accent-pressed"
            >{{ t('account.topUp.viewReceipt') }}</a
          ><span v-else class="text-ib-text-subtle">—</span></template
        >
      </Column>

      <template #empty>
        <div
          v-if="initialError"
          class="flex min-h-40 flex-col items-center justify-center px-6 py-10 text-center"
          role="alert"
        >
          <p class="m-0 font-semibold text-ib-danger">
            {{ t('account.errors.historyLoadFailed') }}
          </p>
          <Button
            :label="t('account.reload')"
            severity="secondary"
            size="small"
            text
            class="mt-3"
            @click="emit('retry')"
          />
        </div>
        <div
          v-else
          class="flex min-h-40 items-center justify-center px-6 py-10 text-center"
          role="status"
        >
          <span class="font-semibold text-ib-text">{{ t('account.topUp.historyEmptyTitle') }}</span>
        </div>
      </template>

      <template #after>
        <p
          v-if="initialPending"
          class="sr-only"
          role="status"
          :aria-label="t('account.topUp.loadingHistory')"
        >
          {{ t('account.topUp.loadingHistory') }}
        </p>
        <div
          v-if="loadMoreError"
          class="flex items-center justify-center gap-3 border-t border-ib-border-soft px-4 py-3"
          role="alert"
        >
          <span class="text-sm text-ib-danger">{{ t('account.topUp.loadMoreFailed') }}</span>
          <Button
            :label="t('account.topUp.retryLoad')"
            severity="secondary"
            size="small"
            text
            @click="emit('load-more')"
          />
        </div>
        <div
          v-else-if="hasNextPage"
          class="flex justify-center border-t border-ib-border-soft px-4 py-3"
        >
          <Button
            :label="t('account.topUp.loadMore')"
            severity="secondary"
            size="small"
            outlined
            :loading="fetchingNextPage"
            @click="emit('load-more')"
          />
        </div>
      </template>
    </DataTableFrame>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Button from 'primevue/button'
import Column from 'primevue/column'
import Skeleton from 'primevue/skeleton'
import DataTableFrame from '../../../../shared/components/DataTableFrame.vue'
import { formatLocaleNumber } from '../../../../shared/i18n/formatters'
import { formatAccountRechargeDateTime, formatCurrencyMinor } from '../../domain'
import type { RechargeHistoryItem } from '../../infrastructure'

type SkeletonRow = { occurred_at: string; skeleton: true }
type TableRow = RechargeHistoryItem | SkeletonRow

const props = defineProps<{
  items: RechargeHistoryItem[]
  initialPending: boolean
  initialError: unknown
  loadMoreError: unknown
  hasNextPage: boolean
  fetchingNextPage: boolean
}>()
const emit = defineEmits<{ retry: []; 'load-more': [] }>()
const { t } = useI18n()
const skeletonRows: SkeletonRow[] = Array.from({ length: 4 }, (_, index) => ({
  occurred_at: `recharge-skeleton-${index}`,
  skeleton: true,
}))
const tableRows = computed<TableRow[]>(() => (props.initialPending ? skeletonRows : props.items))
function isSkeleton(row: TableRow): row is SkeletonRow {
  return 'skeleton' in row
}
function rechargeStatusLabel(item: RechargeHistoryItem): string {
  if (item.status === 'cancelled') return t('account.topUp.statusHistoryCancelled')
  if (item.status === 'refunded') return t('account.topUp.statusHistoryRefunded')
  if (item.refunded_amount_minor && item.paid_currency) {
    return t('account.topUp.statusHistoryCreditedWithRefund', {
      refunded: formatCurrencyMinor(item.refunded_amount_minor, item.paid_currency),
    })
  }
  return t('account.topUp.statusHistoryCredited')
}
function formatDateTime(value: string): string {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return t('account.topUp.unknownTime')
  return formatAccountRechargeDateTime(date)
}
</script>
