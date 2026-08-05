<template>
  <section class="grid min-w-0 gap-3" aria-labelledby="recharge-history-heading">
    <h3 id="recharge-history-heading" class="m-0 text-lg font-bold text-ib-text">充值记录</h3>
    <DataTableFrame
      :value="tableRows"
      scrollable
      :table-style="{ minWidth: '720px' }"
      table-aria-label="充值记录表格"
      scroll-aria-label="充值记录表格"
      frame-class="rounded-[var(--radius-ib-panel)] border border-ib-border-soft bg-ib-surface shadow-[var(--shadow-ib-control)]"
      empty-title="暂无充值记录"
    >
      <Column field="occurred_at" header="时间" :style="{ width: '180px' }">
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
            >{{ ideaFormatter.format(data.idea_quantity) }}</span
          ></template
        >
      </Column>
      <Column field="paid_amount_minor" header="实付金额" :style="{ width: '170px' }">
        <template #body="{ data }"
          ><Skeleton v-if="isSkeleton(data)" width="6rem" /><span
            v-else
            class="whitespace-nowrap font-mono font-semibold text-ib-text-muted"
            >{{ formatCurrencyMinor(data.paid_amount_minor, data.paid_currency) }}</span
          ></template
        >
      </Column>
      <Column field="status" header="状态" :style="{ width: '190px' }">
        <template #body="{ data }"
          ><Skeleton v-if="isSkeleton(data)" width="4rem" /><span
            v-else
            class="font-semibold"
            :class="data.status === 'credited' ? 'text-ib-success' : 'text-ib-text-muted'"
            >{{ rechargeStatusLabel(data) }}</span
          ></template
        >
      </Column>
      <Column header="收据" :style="{ width: '100px' }">
        <template #body="{ data }"
          ><Skeleton v-if="isSkeleton(data)" width="3rem" /><a
            v-else-if="data.receipt_url"
            :href="data.receipt_url"
            target="_blank"
            rel="noopener noreferrer"
            class="font-semibold text-ib-creative-accent-pressed"
            >查看收据</a
          ><span v-else class="text-ib-text-subtle">—</span></template
        >
      </Column>

      <template #empty>
        <div
          v-if="initialError"
          class="flex min-h-40 flex-col items-center justify-center px-6 py-10 text-center"
          role="alert"
        >
          <p class="m-0 font-semibold text-ib-danger">充值记录加载失败</p>
          <Button
            label="重新加载"
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
          <span class="font-semibold text-ib-text">暂无充值记录</span>
        </div>
      </template>

      <template #after>
        <p v-if="initialPending" class="sr-only" role="status" aria-label="正在加载充值记录">
          正在加载充值记录
        </p>
        <div
          v-if="loadMoreError"
          class="flex items-center justify-center gap-3 border-t border-ib-border-soft px-4 py-3"
          role="alert"
        >
          <span class="text-sm text-ib-danger">加载更多失败</span>
          <Button
            label="重试加载"
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
            label="加载更多"
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
import Button from 'primevue/button'
import Column from 'primevue/column'
import Skeleton from 'primevue/skeleton'
import DataTableFrame from '../../../shared/components/DataTableFrame.vue'
import { formatCurrencyMinor } from '../paymentMoney'
import type { UserRechargeHistoryItem } from '../userPaymentsResource'

type SkeletonRow = { occurred_at: string; skeleton: true }
type TableRow = UserRechargeHistoryItem | SkeletonRow

const props = defineProps<{
  items: UserRechargeHistoryItem[]
  initialPending: boolean
  initialError: unknown
  loadMoreError: unknown
  hasNextPage: boolean
  fetchingNextPage: boolean
}>()
const emit = defineEmits<{ retry: []; 'load-more': [] }>()
const ideaFormatter = new Intl.NumberFormat('en-US')
const skeletonRows: SkeletonRow[] = Array.from({ length: 4 }, (_, index) => ({
  occurred_at: `recharge-skeleton-${index}`,
  skeleton: true,
}))
const tableRows = computed<TableRow[]>(() => (props.initialPending ? skeletonRows : props.items))
function isSkeleton(row: TableRow): row is SkeletonRow {
  return 'skeleton' in row
}
function rechargeStatusLabel(item: UserRechargeHistoryItem): string {
  if (item.status === 'refunded') return '已退款'
  if (item.refunded_amount_minor) {
    return `已到账 · 已退 ${formatCurrencyMinor(item.refunded_amount_minor, item.paid_currency)}`
  }
  return '已到账'
}
function formatDateTime(value: string): string {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '时间未知'
  const pad = (part: number) => String(part).padStart(2, '0')
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}`
}
</script>
