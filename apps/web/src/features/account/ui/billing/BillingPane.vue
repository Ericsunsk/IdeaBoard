<template>
  <AccountPane :title="t('account.tabs.billingTransactions')">
    <p v-if="billingAccountId" class="mb-3 text-admin-meta text-muted-foreground" role="status">
      {{ t('account.billingRecords.sharedAccount', { account: billingAccountId }) }}
    </p>
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
      <DataTableColumn
        field="occurred_at"
        :header="t('account.billingRecords.columnTime')"
        :style="{ width: '190px' }"
      >
        <template #body="{ data }">
          <Skeleton v-if="isSkeletonRow(data)" data-testid="billing-skeleton-row" width="8rem" />
          <time v-else class="whitespace-nowrap font-mono text-admin-body text-muted-foreground">
            {{ formatLocalDateTime(data.occurred_at) }}
          </time>
        </template>
      </DataTableColumn>

      <DataTableColumn
        field="title"
        :header="t('account.billingRecords.columnDescription')"
        :style="{ minWidth: '220px' }"
      >
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
      </DataTableColumn>

      <DataTableColumn
        field="credit_delta"
        :header="t('account.billingRecords.columnCreditChange')"
        :style="{ width: '140px' }"
      >
        <template #body="{ data }">
          <Skeleton v-if="isSkeletonRow(data)" width="4rem" />
          <span v-else class="font-mono font-bold" :class="amountTone(data)">
            {{ formatSignedAmount(data.credit_delta) }}
          </span>
        </template>
      </DataTableColumn>

      <DataTableColumn
        v-if="consumptionScope === 'account'"
        field="available_credits_after"
        :header="t('account.billingRecords.columnBalanceAfter')"
        :style="{ width: '140px' }"
      >
        <template #body="{ data }">
          <Skeleton v-if="isSkeletonRow(data)" width="5rem" />
          <span v-else class="font-mono font-semibold text-muted-foreground">
            {{
              data.available_credits_after == null
                ? '—'
                : formatLocaleNumber(data.available_credits_after)
            }}
          </span>
        </template>
      </DataTableColumn>

      <template #empty>
        <div
          v-if="isForbidden"
          class="flex min-h-48 flex-col items-center justify-center px-6 py-10 text-center"
          role="status"
        >
          <span class="font-semibold text-foreground">{{
            t('account.billingRecords.forbiddenTitle')
          }}</span>
          <span class="mt-1 text-admin-content text-muted-foreground">{{
            t('account.billingRecords.forbiddenDescription')
          }}</span>
          <Button variant="ghost" size="sm" class="mt-3" @click="retryInitial()">{{
            t('account.reload')
          }}</Button>
        </div>
        <LoadErrorPanel
          v-else-if="initialError"
          class="min-h-48 justify-center"
          align="center"
          :message="t('account.billingRecords.loadFailed')"
          :detail="errorMessage(initialError)"
          :retry-label="t('account.reload')"
          retry-variant="ghost"
          @retry="retryInitial()"
        />
        <div
          v-else
          class="flex min-h-48 flex-col items-center justify-center px-6 py-10 text-center"
          role="status"
        >
          <span class="font-semibold text-foreground">{{
            t('account.billingRecords.emptyTitle')
          }}</span>
          <span class="mt-1 text-admin-content text-muted-foreground">{{
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

        <LoadErrorPanel
          v-if="loadMoreError"
          align="center"
          :message="errorMessage(loadMoreError)"
          :retry-label="t('account.billingRecords.retryLoad')"
          :is-retrying="isFetchingNextPage"
          retry-variant="ghost"
          @retry="loadMore()"
        />
        <div v-else-if="hasNextPage" class="flex justify-center border-t border-border px-4 py-3">
          <Button
            variant="outline"
            size="sm"
            :is-loading="isFetchingNextPage"
            @click="loadMore()"
            >{{ t('account.billingRecords.loadMore') }}</Button
          >
        </div>
      </template>
    </DataTableFrame>
  </AccountPane>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { Button } from '@/components/ui/button'
import DataTableColumn from '../../../../core/components/DataTableColumn.vue'
import { Skeleton } from '@/components/ui/skeleton'
import DataTableFrame from '../../../../core/components/DataTableFrame.vue'
import LoadErrorPanel from '../../../../core/components/LoadErrorPanel.vue'
import { formatLocaleNumber } from '../../../../core/i18n/formatters'
import { userErrorCode, userFacingErrorCopy } from '../../../../core/errors/errorCatalog'
import { ACCOUNT_GUIDANCE_COPY_KEYS } from '../../session/workspaceBilling'
import { formatAccountLocalDateTime } from '../../session/formatters'
import type { BillingRecord } from '../../session/billing/resource'
import { useBillingRecords } from '../../session/billing/useBillingRecords'
import AccountPane from '../shared/AccountPane.vue'
import { accountDataTableFrameClass } from '../shared/ui'

type BillingSkeletonRow = {
  record_id: string
  isSkeleton: true
}
type BillingTableRow = BillingRecord | BillingSkeletonRow

const props = defineProps<{ active: boolean }>()
const { t } = useI18n()
const {
  billingAccountId,
  consumptionScope,
  records,
  isInitialPending,
  initialError,
  loadMoreError,
  hasNextPage,
  isFetchingNextPage,
  retryInitial,
  loadMore,
} = useBillingRecords(() => props.active)
const isForbidden = computed(
  () => userErrorCode(initialError.value) === 'billing_account_forbidden',
)
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

function amountTone(record: BillingRecord): string {
  if (record.credit_delta > 0) return 'text-success'
  if (record.kind === 'system_adjustment' && record.credit_delta < 0) {
    return 'text-destructive'
  }
  return 'text-foreground'
}

// Backend titles are Chinese protocol data; User copy resolves from the stable kind enum instead.
const BILLING_KIND_COPY_KEY_MAP: Partial<Record<BillingRecord['kind'], string>> = {
  usage: 'account.billingRecords.kinds.usage',
  reward: 'account.billingRecords.kinds.reward',
  top_up: 'account.billingRecords.kinds.topUp',
  system_grant: 'account.billingRecords.kinds.systemGrant',
  system_adjustment: 'account.billingRecords.kinds.systemAdjustment',
}

function recordDescription(record: BillingRecord): string {
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
  return userFacingErrorCopy(error, t('account.errors.networkRetry'), ACCOUNT_GUIDANCE_COPY_KEYS)
}
</script>
