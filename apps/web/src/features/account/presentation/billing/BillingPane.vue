<template>
  <AccountPane :title="t('account.tabs.billingTransactions')">
    <DataTableFrame
      :value="tableRows"
      data-key="record_id"
      scrollable
      :table-style="{ minWidth: '720px' }"
      :table-aria-label="t('account.billingRecords.tableAria')"
      :scroll-aria-label="t('account.billingRecords.tableAria')"
      :frame-class="accountDataTableFrameClass"
      :empty-title="t('account.billingRecords.emptyTitle')"
      :empty-description="t('account.billingRecords.emptyDescription')"
    >
      <UiColumn
        field="occurred_at"
        :header="t('account.billingRecords.columnTime')"
        :style="{ width: '190px' }"
      >
        <template #body="{ data }">
          <UiSkeleton v-if="isSkeletonRow(data)" data-testid="billing-skeleton-row" width="8rem" />
          <time v-else class="whitespace-nowrap font-mono text-[13px] text-ib-text-muted">
            {{ formatLocalDateTime(data.occurred_at) }}
          </time>
        </template>
      </UiColumn>

      <UiColumn
        field="title"
        :header="t('account.billingRecords.columnDescription')"
        :style="{ minWidth: '220px' }"
      >
        <template #body="{ data }">
          <UiSkeleton v-if="isSkeletonRow(data)" width="7rem" />
          <span
            v-else
            class="block max-w-[320px] truncate font-semibold"
            :title="recordDescription(data)"
          >
            {{ recordDescription(data) }}
          </span>
        </template>
      </UiColumn>

      <UiColumn
        field="credit_delta"
        :header="t('account.billingRecords.columnCreditChange')"
        :style="{ width: '140px' }"
      >
        <template #body="{ data }">
          <UiSkeleton v-if="isSkeletonRow(data)" width="4rem" />
          <span v-else class="font-mono font-bold" :class="amountTone(data)">
            {{ formatSignedAmount(data.credit_delta) }}
          </span>
        </template>
      </UiColumn>

      <UiColumn
        field="available_credits_after"
        :header="t('account.billingRecords.columnBalanceAfter')"
        :style="{ width: '140px' }"
      >
        <template #body="{ data }">
          <UiSkeleton v-if="isSkeletonRow(data)" width="5rem" />
          <span v-else class="font-mono font-semibold text-ib-text-muted">
            {{ formatLocaleNumber(data.available_credits_after) }}
          </span>
        </template>
      </UiColumn>

      <template #empty>
        <div
          v-if="initialError"
          class="flex min-h-48 flex-col items-center justify-center px-6 py-10 text-center"
          role="alert"
        >
          <p class="m-0 font-semibold text-ib-danger">
            {{ t('account.billingRecords.loadFailed') }}
          </p>
          <p class="mt-1 mb-0 text-sm text-ib-text-muted">{{ errorMessage(initialError) }}</p>
          <UiButton
            variant="ghost"
            size="sm"
            class="mt-3"
            @click="retryInitial()">{{ t('account.reload') }}</UiButton>
        </div>
        <div
          v-else
          class="flex min-h-48 flex-col items-center justify-center px-6 py-10 text-center"
          role="status"
        >
          <span class="font-semibold text-ib-text">{{
            t('account.billingRecords.emptyTitle')
          }}</span>
          <span class="mt-1 text-sm text-ib-text-muted">{{
            t('account.billingRecords.emptyDescription')
          }}</span>
        </div>
      </template>

      <template #after>
        <p
          v-if="isInitialPending"
          class="sr-only"
          role="status"
          :aria-label="t('account.billingRecords.loading')"
        >
          {{ t('account.billingRecords.loading') }}
        </p>

        <div
          v-if="loadMoreError"
          class="flex items-center justify-center gap-3 border-t border-ib-border-soft px-4 py-3"
          role="alert"
        >
          <span class="text-sm text-ib-danger">{{ errorMessage(loadMoreError) }}</span>
          <UiButton
            variant="ghost"
            size="sm"
            @click="loadMore()">{{ t('account.billingRecords.retryLoad') }}</UiButton>
        </div>
        <div
          v-else-if="hasNextPage"
          class="flex justify-center border-t border-ib-border-soft px-4 py-3"
        >
          <UiButton
            variant="outline"
            size="sm"
            :loading="isFetchingNextPage"
            @click="loadMore()">{{ t('account.billingRecords.loadMore') }}</UiButton>
        </div>
      </template>
    </DataTableFrame>
  </AccountPane>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { Button as UiButton } from '@/components/ui/button'
import UiColumn from '../../../../shared/components/DataTableColumn.vue'
import { Skeleton as UiSkeleton } from '@/components/ui/skeleton'
import DataTableFrame from '../../../../shared/components/DataTableFrame.vue'
import { formatLocaleNumber } from '../../../../shared/i18n/formatters'
import { userFacingErrorCopy } from '../../../../shared/errors/errorCatalog'
import { formatAccountLocalDateTime } from '../../domain'
import type { BillingRecordItem } from '../../infrastructure'
import { useBillingRecords } from '../../application/billing/useBillingRecords'
import AccountPane from '../common/AccountPane.vue'
import { accountDataTableFrameClass } from '../common/ui'

type BillingSkeletonRow = {
  record_id: string
  isSkeleton: true
}
type BillingTableRow = BillingRecordItem | BillingSkeletonRow

const props = defineProps<{ active: boolean }>()
const { t } = useI18n()
const {
  records,
  isInitialPending,
  initialError,
  loadMoreError,
  hasNextPage,
  isFetchingNextPage,
  retryInitial,
  loadMore,
} = useBillingRecords(() => props.active)
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

function formatSignedAmount(value: number): string {
  if (value > 0) return `+${formatLocaleNumber(value)}`
  return formatLocaleNumber(value)
}

function amountTone(record: BillingRecordItem): string {
  if (record.credit_delta > 0) return 'text-ib-success'
  if (record.kind === 'system_adjustment' && record.credit_delta < 0) {
    return 'text-ib-danger'
  }
  return 'text-ib-text'
}

// Backend titles are Chinese protocol data; User copy resolves from the stable kind enum instead.
const BILLING_KIND_COPY_KEY_MAP: Partial<Record<BillingRecordItem['kind'], string>> = {
  usage: 'account.billingRecords.kinds.usage',
  reward: 'account.billingRecords.kinds.reward',
  top_up: 'account.billingRecords.kinds.topUp',
  system_grant: 'account.billingRecords.kinds.systemGrant',
  system_adjustment: 'account.billingRecords.kinds.systemAdjustment',
}

function recordDescription(record: BillingRecordItem): string {
  const key = BILLING_KIND_COPY_KEY_MAP[record.kind]
  // ai_usage keeps the pricing-config display name (e.g. the SKU label shown before purchase).
  if (!key) return record.title
  return t(key)
}

function formatLocalDateTime(value: string): string {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return t('account.billingRecords.unknownTime')
  return formatAccountLocalDateTime(date)
}

function errorMessage(error: unknown): string {
  return userFacingErrorCopy(error, t('account.errors.networkRetry'))
}
</script>
