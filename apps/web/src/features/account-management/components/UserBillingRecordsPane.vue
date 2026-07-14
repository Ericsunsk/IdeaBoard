<template>
  <AccountManagementPane title="账单记录">
    <DataTableFrame
      :value="tableRows"
      data-key="record_id"
      scrollable
      :table-style="{ minWidth: '720px' }"
      table-aria-label="账单记录表格"
      scroll-aria-label="账单记录表格"
      frame-class="rounded-[var(--radius-ib-panel)] border border-ib-border-soft bg-ib-surface shadow-[var(--shadow-ib-control)]"
      empty-title="暂无账单记录"
      empty-description="Idea 收支记录会显示在这里"
    >
      <Column field="occurred_at" header="交易时间" :style="{ width: '190px' }">
        <template #body="{ data }">
          <Skeleton
            v-if="isSkeletonRow(data)"
            data-testid="billing-skeleton-row"
            width="8rem"
          />
          <time v-else class="whitespace-nowrap font-mono text-[13px] text-ib-text-muted">
            {{ formatLocalDateTime(data.occurred_at) }}
          </time>
        </template>
      </Column>

      <Column field="title" header="记录内容" :style="{ minWidth: '220px' }">
        <template #body="{ data }">
          <Skeleton v-if="isSkeletonRow(data)" width="7rem" />
          <span v-else class="block max-w-[320px] truncate font-semibold" :title="data.title">
            {{ data.title }}
          </span>
        </template>
      </Column>

      <Column field="credit_delta" header="Idea 变动" :style="{ width: '140px' }">
        <template #body="{ data }">
          <Skeleton v-if="isSkeletonRow(data)" width="4rem" />
          <span v-else class="font-mono font-bold" :class="amountTone(data)">
            {{ formatSignedAmount(data.credit_delta) }}
          </span>
        </template>
      </Column>

      <Column field="available_credits_after" header="可用余额" :style="{ width: '140px' }">
        <template #body="{ data }">
          <Skeleton v-if="isSkeletonRow(data)" width="5rem" />
          <span v-else class="font-mono font-semibold text-ib-text-muted">
            {{ formatAmount(data.available_credits_after) }}
          </span>
        </template>
      </Column>

      <template #empty>
        <div
          v-if="initialError"
          class="flex min-h-48 flex-col items-center justify-center px-6 py-10 text-center"
          role="alert"
        >
          <p class="m-0 font-semibold text-ib-danger">账单加载失败</p>
          <p class="mt-1 mb-0 text-sm text-ib-text-muted">{{ errorMessage(initialError) }}</p>
          <Button
            label="重新加载"
            severity="secondary"
            size="small"
            text
            class="mt-3"
            @click="retryInitial()"
          />
        </div>
        <div
          v-else
          class="flex min-h-48 flex-col items-center justify-center px-6 py-10 text-center"
          role="status"
        >
          <span class="font-semibold text-ib-text">暂无账单记录</span>
          <span class="mt-1 text-sm text-ib-text-subtle">Idea 收支记录会显示在这里</span>
        </div>
      </template>

      <template #after>
        <p v-if="isInitialPending" class="sr-only" role="status" aria-label="正在加载账单记录">
          正在加载账单记录
        </p>

        <div
          v-if="loadMoreError"
          class="flex items-center justify-center gap-3 border-t border-ib-border-soft px-4 py-3"
          role="alert"
        >
          <span class="text-sm text-ib-danger">{{ errorMessage(loadMoreError) }}</span>
          <Button
            label="重试加载"
            severity="secondary"
            size="small"
            text
            @click="loadMore()"
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
            :loading="isFetchingNextPage"
            @click="loadMore()"
          />
        </div>
      </template>
    </DataTableFrame>
  </AccountManagementPane>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import Button from 'primevue/button'
import Column from 'primevue/column'
import Skeleton from 'primevue/skeleton'
import DataTableFrame from '../../../shared/components/DataTableFrame.vue'
import type { UserBillingRecord } from '../userBillingRecordsResource'
import { useUserBillingRecords } from '../useUserBillingRecords'
import AccountManagementPane from './AccountManagementPane.vue'

type BillingSkeletonRow = {
  record_id: string
  isSkeleton: true
}
type BillingTableRow = UserBillingRecord | BillingSkeletonRow

const props = defineProps<{ active: boolean }>()
const {
  records,
  isInitialPending,
  initialError,
  loadMoreError,
  hasNextPage,
  isFetchingNextPage,
  retryInitial,
  loadMore,
} = useUserBillingRecords(() => props.active)

const numberFormatter = new Intl.NumberFormat('zh-CN')
const skeletonRows: BillingSkeletonRow[] = Array.from({ length: 5 }, (_, index) => ({
  record_id: `billing-skeleton-${index}`,
  isSkeleton: true,
}))
const tableRows = computed<BillingTableRow[]>(() =>
  isInitialPending.value ? skeletonRows : records.value,
)
function isSkeletonRow(row: BillingTableRow): row is BillingSkeletonRow {
  return 'isSkeleton' in row
}

function formatAmount(value: number): string {
  return numberFormatter.format(value)
}

function formatSignedAmount(value: number): string {
  if (value > 0) return `+${formatAmount(value)}`
  return formatAmount(value)
}

function amountTone(record: UserBillingRecord): string {
  if (record.credit_delta > 0) return 'text-ib-success'
  if (record.kind === 'system_adjustment' && record.credit_delta < 0) {
    return 'text-ib-danger'
  }
  return 'text-ib-text'
}

function formatLocalDateTime(value: string): string {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '时间未知'

  return [
    `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}`,
    `${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}`,
  ].join(' ')
}

function pad(value: number): string {
  return String(value).padStart(2, '0')
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : '网络错误，请重试'
}
</script>
