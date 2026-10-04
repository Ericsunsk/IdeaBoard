<template>
  <section
    class="flex flex-col gap-3.5 rounded-2xl border border-border bg-card p-4.5 shadow-2xs"
    aria-labelledby="admin-user-wallet-ledger-title"
  >
    <div class="overflow-hidden rounded-xl border border-border">
      <AdminTable
        :value="transactions"
        data-key="transaction_id"
        :loading="ledgerLoading && transactions.length === 0"
        scrollable
        :table-aria-label="t('admin.wallet.ledgerAria')"
        :scroll-aria-label="t('admin.wallet.ledgerAria')"
        table-style="min-width: 780px"
      >
        <template #empty>
          <AdminEmptyState compact :title="t('admin.wallet.ledgerEmpty')" />
        </template>

        <AdminColumn :header="t('admin.wallet.type')" style="width: 96px">
          <template #body="{ data }">
            <AdminBadge
              :value="getCreditCommandLabel(data.command_type)"
              :severity="getCreditCommandSeverity(data.command_type)"
              :class="adminCompactTagClass"
            />
          </template>
        </AdminColumn>

        <AdminColumn :header="t('admin.wallet.change')" style="width: 120px">
          <template #body="{ data }">
            <span
              class="font-bold tabular-nums font-mono text-admin-body"
              :class="
                data.command_type === 'grant' || data.command_type === 'refund'
                  ? 'text-success'
                  : 'text-foreground'
              "
            >
              {{ getCreditCommandSign(data.command_type)
              }}{{ formatIdeaAmount(data.amount_credits) }}
            </span>
          </template>
        </AdminColumn>

        <AdminColumn :header="t('admin.wallet.balanceAfter')" style="width: 150px">
          <template #body="{ data }">
            <span class="font-mono text-admin-caption text-muted-foreground">
              {{
                t('admin.wallet.availableAmount', {
                  amount: formatIdeaAmount(data.available_credits_after),
                })
              }}
            </span>
          </template>
        </AdminColumn>

        <AdminColumn :header="t('admin.wallet.reasonAndReference')" style="min-width: 220px">
          <template #body="{ data }">
            <div class="max-w-72 truncate text-admin-body-sm text-foreground" :title="data.reason">
              {{ data.reason || '-' }}
            </div>
            <div
              v-if="data.charge"
              class="mt-0.5 text-admin-caption text-muted-foreground font-mono"
            >
              {{
                t('admin.wallet.refundableAmount', {
                  amount: formatIdeaAmount(data.charge.remaining_refundable_credits),
                })
              }}
            </div>
          </template>
        </AdminColumn>

        <AdminColumn :header="t('admin.wallet.time')" style="width: 160px">
          <template #body="{ data }">
            <span class="font-mono text-admin-caption text-muted-foreground whitespace-nowrap">
              {{ formatAdminDateTimeWithSeconds(data.created_at) }}
            </span>
          </template>
        </AdminColumn>

        <AdminColumn :header="t('admin.common.actions')" style="width: 80px">
          <template #body="{ data }">
            <AdminButton
              v-if="isRefundableCharge(data)"
              :label="t('admin.wallet.refund')"
              size="small"
              variant="ghost"
              class="text-destructive hover:text-destructive/80"
              :disabled="commandPending"
              :aria-label="t('admin.wallet.refundAria', { id: data.transaction_id })"
              @click="emit('openRefund', data)"
            />
            <span v-else class="text-admin-caption text-muted-foreground">-</span>
          </template>
        </AdminColumn>
      </AdminTable>
    </div>

    <div v-if="hasNextPage" class="mt-2 flex justify-center">
      <AdminButton
        :label="t('admin.wallet.loadMore')"
        :class="adminActionButtonClass"
        variant="outline"
        :loading="ledgerLoading && transactions.length > 0"
        :disabled="ledgerLoading || commandPending"
        aria-controls="admin-user-wallet-ledger-title"
        @click="emit('loadMore')"
      />
    </div>
  </section>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import AdminBadge from '../../../shared/components/AdminBadge.vue'
import AdminButton from '../../../shared/components/AdminButton.vue'
import AdminColumn from '../../../shared/components/AdminColumn.vue'
import AdminEmptyState from '../../../shared/components/AdminEmptyState.vue'
import AdminTable from '../../../shared/components/AdminTable.vue'
import { adminActionButtonClass, adminCompactTagClass } from '../../../shared/lib/uiClasses'
import {
  formatAdminDateTimeWithSeconds,
  getCreditCommandLabel,
  getCreditCommandSeverity,
  getCreditCommandSign,
} from '../../shared/format'
import { formatIdeaAmount } from '../../../../../shared/i18n/userIdeaCurrency'
import type { AdminCreditLedgerTransaction } from '../resource'
import { isRefundableCharge } from '../creditCommandContract'

defineProps<{
  transactions: AdminCreditLedgerTransaction[]
  ledgerLoading: boolean
  commandPending: boolean
  hasNextPage: boolean
}>()

const emit = defineEmits<{
  openRefund: [transaction: AdminCreditLedgerTransaction]
  loadMore: []
}>()

const { t } = useI18n()
</script>
