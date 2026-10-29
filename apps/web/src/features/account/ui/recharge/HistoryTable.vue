<template>
  <section class="grid min-w-0 gap-3" aria-labelledby="recharge-history-heading">
    <h3 id="recharge-history-heading" class="m-0 text-lg font-bold text-foreground">
      {{ t('account.topUp.historyTitle') }}
    </h3>
    <DataTableFrame
      :value="tableRows"
      scrollable
      :table-style="{ minWidth: '720px' }"
      :table-aria-label="t('account.topUp.historyTableAria')"
      :scroll-aria-label="t('account.topUp.historyTableAria')"
      frame-class="rounded-panel border border-border bg-card shadow-raised"
      :empty-title="t('account.topUp.historyEmptyTitle')"
    >
      <DataTableColumn
        field="occurred_at"
        :header="t('account.topUp.columnTime')"
        :style="{ width: '180px' }"
      >
        <template #body="{ data }"
          ><Skeleton v-if="isSkeleton(data)" width="8rem" /><time
            v-else
            class="whitespace-nowrap font-mono text-admin-body text-muted-foreground"
            >{{ formatDateTime(data.occurred_at) }}</time
          ></template
        >
      </DataTableColumn>
      <DataTableColumn field="idea_quantity" header="Ideas" :style="{ width: '130px' }">
        <template #body="{ data }"
          ><Skeleton v-if="isSkeleton(data)" width="4rem" /><span
            v-else
            class="font-mono font-bold text-foreground"
            >{{ formatLocaleNumber(data.idea_quantity) }}</span
          ></template
        >
      </DataTableColumn>
      <DataTableColumn
        field="paid_amount_minor"
        :header="t('account.topUp.columnPaid')"
        :style="{ width: '170px' }"
      >
        <template #body="{ data }"
          ><Skeleton v-if="isSkeleton(data)" width="6rem" /><span
            v-else-if="data.paid_amount_minor !== undefined && data.paid_currency"
            class="whitespace-nowrap font-mono font-semibold text-muted-foreground"
            >{{ formatCurrencyMinor(data.paid_amount_minor, data.paid_currency) }}</span
          ><span v-else class="font-semibold text-muted-foreground">{{
            t('account.topUp.unpaid')
          }}</span></template
        >
      </DataTableColumn>
      <DataTableColumn
        field="status"
        :header="t('account.topUp.columnStatus')"
        :style="{ width: '190px' }"
      >
        <template #body="{ data }"
          ><Skeleton v-if="isSkeleton(data)" width="4rem" /><span
            v-else
            class="font-semibold"
            :class="data.status === 'credited' ? 'text-success' : 'text-muted-foreground'"
            >{{ rechargeStatusLabel(data) }}</span
          ></template
        >
      </DataTableColumn>
      <DataTableColumn :header="t('account.topUp.columnReceipt')" :style="{ width: '100px' }">
        <template #body="{ data }"
          ><Skeleton v-if="isSkeleton(data)" width="3rem" /><a
            v-else-if="data.receipt_url"
            :href="data.receipt_url"
            target="_blank"
            rel="noopener noreferrer"
            class="font-semibold text-primary"
            >{{ t('account.topUp.viewReceipt') }}</a
          ><span v-else class="text-muted-foreground">—</span></template
        >
      </DataTableColumn>

      <template #empty>
        <AccountEmptyRetry
          v-if="initialError"
          :title="t('account.topUp.historyEmptyTitle')"
          :retry-label="t('account.reload')"
          @retry="emit('retry')"
        />
        <div
          v-else
          class="flex min-h-40 items-center justify-center px-6 py-10 text-center"
          role="status"
        >
          <span :class="accountEmptyTitleClass">{{ t('account.topUp.historyEmptyTitle') }}</span>
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
        <LoadErrorPanel
          v-if="loadMoreError"
          align="center"
          :message="t('account.topUp.loadMoreFailed')"
          :retry-label="t('account.topUp.retryLoad')"
          :is-retrying="fetchingNextPage"
          retry-variant="ghost"
          @retry="emit('load-more')"
        />
        <div v-else-if="hasNextPage" class="flex justify-center border-t border-border px-4 py-3">
          <Button
            variant="outline"
            size="sm"
            :is-loading="fetchingNextPage"
            @click="emit('load-more')"
            >{{ t('account.topUp.loadMore') }}</Button
          >
        </div>
      </template>
    </DataTableFrame>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { Button } from '@/components/ui/button'
import DataTableColumn from '../../../../core/components/DataTableColumn.vue'
import { Skeleton } from '@/components/ui/skeleton'
import DataTableFrame from '../../../../core/components/DataTableFrame.vue'
import LoadErrorPanel from '../../../../core/components/LoadErrorPanel.vue'
import AccountEmptyRetry from '../shared/AccountEmptyRetry.vue'
import { accountEmptyTitleClass } from '../shared/ui'
import { parseDateGuard } from '../../../../core/datetime/parse'
import { formatLocaleNumber } from '../../../../core/i18n/formatters'
import { formatAccountRechargeDateTime } from '../../session/formatters'
import { formatCurrencyMinor } from '../../session/money'
import type { RechargeHistoryItem } from '../../session/payments/resource'

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
  const date = parseDateGuard(value)
  // Invalid-guard stays presentation-local (needs the `t()` fallback); the
  // format itself converges on the account family (`formatters.ts`).
  if (!date) return t('account.topUp.unknownTime')
  return formatAccountRechargeDateTime(date)
}
</script>
