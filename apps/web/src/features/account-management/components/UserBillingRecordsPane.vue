<template>
  <AccountManagementPane :title="t('account.tabs.billingTransactions')">
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
      <Column field="occurred_at" :header="t('account.billingRecords.columnTime')" :style="{ width: '190px' }">
        <template #body="{ data }">
          <Skeleton v-if="isSkeletonRow(data)" data-testid="billing-skeleton-row" width="8rem" />
          <time v-else class="whitespace-nowrap font-mono text-[13px] text-ib-text-muted">
            {{ formatLocalDateTime(data.occurred_at) }}
          </time>
        </template>
      </Column>

      <Column field="title" :header="t('account.billingRecords.columnDescription')" :style="{ minWidth: '220px' }">
        <template #body="{ data }">
          <Skeleton v-if="isSkeletonRow(data)" width="7rem" />
          <span
            v-else
            class="block max-w-[320px] truncate font-semibold"
            :title="recordDescription(data)"
          >
            {{ recordDescription(data) }}
          </span>
        </template>
      </Column>

      <Column field="credit_delta" :header="t('account.billingRecords.columnCreditChange')" :style="{ width: '140px' }">
        <template #body="{ data }">
          <Skeleton v-if="isSkeletonRow(data)" width="4rem" />
          <span v-else class="font-mono font-bold" :class="amountTone(data)">
            {{ formatSignedAmount(data.credit_delta) }}
          </span>
        </template>
      </Column>

      <Column field="available_credits_after" :header="t('account.billingRecords.columnBalanceAfter')" :style="{ width: '140px' }">
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
          <p class="m-0 font-semibold text-ib-danger">{{ t('account.billingRecords.loadFailed') }}</p>
          <p class="mt-1 mb-0 text-sm text-ib-text-muted">{{ errorMessage(initialError) }}</p>
          <Button
            :label="t('account.reload')"
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
          <span class="font-semibold text-ib-text">{{ t('account.billingRecords.emptyTitle') }}</span>
          <span class="mt-1 text-sm text-ib-text-subtle">{{ t('account.billingRecords.emptyDescription') }}</span>
        </div>
      </template>

      <template #after>
        <p v-if="isInitialPending" class="sr-only" role="status" :aria-label="t('account.billingRecords.loading')">
          {{ t('account.billingRecords.loading') }}
        </p>

        <div
          v-if="loadMoreError"
          class="flex items-center justify-center gap-3 border-t border-ib-border-soft px-4 py-3"
          role="alert"
        >
          <span class="text-sm text-ib-danger">{{ errorMessage(loadMoreError) }}</span>
          <Button :label="t('account.billingRecords.retryLoad')" severity="secondary" size="small" text @click="loadMore()" />
        </div>
        <div
          v-else-if="hasNextPage"
          class="flex justify-center border-t border-ib-border-soft px-4 py-3"
        >
          <Button
            :label="t('account.billingRecords.loadMore')"
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
import { useI18n } from 'vue-i18n'
import Button from 'primevue/button'
import Column from 'primevue/column'
import Skeleton from 'primevue/skeleton'
import DataTableFrame from '../../../shared/components/DataTableFrame.vue'
import { userFacingErrorCopy } from '../../../shared/errors/errorCatalog'
import type { UserBillingRecord } from '../userBillingRecordsResource'
import { useUserBillingRecords } from '../useUserBillingRecords'
import AccountManagementPane from './AccountManagementPane.vue'
import { accountDataTableFrameClass } from './accountManagementUi'

type BillingSkeletonRow = {
  record_id: string
  isSkeleton: true
}
type BillingTableRow = UserBillingRecord | BillingSkeletonRow

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
} = useUserBillingRecords(() => props.active)

const numberFormatter = new Intl.NumberFormat('en-US')
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

// Backend titles are Chinese protocol data; User copy resolves from the stable kind enum instead.
const BILLING_KIND_COPY_KEY_MAP: Partial<Record<UserBillingRecord['kind'], string>> = {
  usage: 'account.billingRecords.kinds.usage',
  reward: 'account.billingRecords.kinds.reward',
  top_up: 'account.billingRecords.kinds.topUp',
  system_grant: 'account.billingRecords.kinds.systemGrant',
  system_adjustment: 'account.billingRecords.kinds.systemAdjustment',
}

function recordDescription(record: UserBillingRecord): string {
  const key = BILLING_KIND_COPY_KEY_MAP[record.kind]
  // ai_usage keeps the pricing-config display name (e.g. the SKU label shown before purchase).
  if (!key) return record.title
  return t(key)
}

function formatLocalDateTime(value: string): string {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return t('account.billingRecords.unknownTime')

  return [
    `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}`,
    `${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}`,
  ].join(' ')
}

function pad(value: number): string {
  return String(value).padStart(2, '0')
}

function errorMessage(error: unknown): string {
  return userFacingErrorCopy(error, t('account.errors.networkRetry'))
}
</script>
