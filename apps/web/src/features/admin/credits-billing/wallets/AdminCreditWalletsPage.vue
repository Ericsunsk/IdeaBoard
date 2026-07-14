<template>
  <div class="flex min-w-0 flex-col gap-4">
    <section
      class="rounded-[var(--radius-ib-panel)] border border-ib-border-soft bg-ib-surface p-5"
    >
      <div class="flex flex-col gap-4 lg:flex-row lg:items-end lg:justify-between">
        <div class="min-w-0">
          <h2 class="m-0 text-admin-modal-title font-[780] text-admin-title-color">用户Idea账户</h2>
          <p class="mt-1 mb-0 text-admin-body text-admin-text-3">
            通过用户 ID 精确打开 Wallet，或搜索用户后选择。余额只能通过 Ledger command 变更。
          </p>
        </div>
        <div class="flex w-full min-w-0 flex-col gap-2 sm:flex-row lg:w-auto">
          <InputNumber
            v-model="directUserId"
            input-id="admin-credit-wallet-user-id"
            placeholder="用户 ID"
            :min="1"
            :use-grouping="false"
            class="w-full sm:w-40"
            aria-label="用户 ID"
            @keyup.enter="openDirectUser"
          />
          <Button label="打开 Wallet" :disabled="!directUserId" @click="openDirectUser" />
        </div>
      </div>

      <div class="mt-4 border-t border-admin-border-soft pt-4">
        <div class="flex flex-col gap-2 sm:flex-row">
          <InputText
            v-model="userSearchKeyword"
            class="min-w-0 flex-1"
            placeholder="搜索用户名、UID 或 Auth User ID"
            aria-label="搜索用户"
            @keyup.enter="searchUsers"
          />
          <Button
            label="搜索用户"
            severity="secondary"
            outlined
            :loading="usersLoading"
            @click="searchUsers"
          />
        </div>
        <div v-if="appliedUserSearch" class="mt-3 flex flex-wrap gap-2" aria-live="polite">
          <button
            v-for="user in userSearchResults"
            :key="user.id"
            type="button"
            class="rounded-[var(--radius-ib-control)] border border-admin-border-soft bg-ib-surface px-3 py-2 text-left text-admin-body text-admin-text-2 transition-colors hover:border-ib-border hover:bg-ib-surface-muted focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-admin-accent"
            :class="user.id === selectedUserId ? 'border-admin-accent bg-ib-surface-muted' : ''"
            @click="selectUser(user)"
          >
            <span class="font-[700] text-admin-text-1">{{ user.username }}</span>
            <span class="ml-2 text-admin-caption">#{{ user.id }} · {{ user.uid }}</span>
          </button>
          <span
            v-if="!usersLoading && userSearchResults.length === 0"
            class="text-admin-body text-admin-text-3"
          >
            暂无匹配用户
          </span>
        </div>
      </div>
    </section>

    <template v-if="selectedUserId">
      <section class="grid grid-cols-1 gap-3.5 md:grid-cols-3">
        <AdminSummaryCard label="可用Idea" :value="formatCredits(wallet?.available_credits ?? 0)" />
        <AdminSummaryCard label="冻结Idea" :value="formatCredits(wallet?.reserved_credits ?? 0)" />
        <AdminSummaryCard
          label="Wallet 状态"
          :value="
            wallet?.status === 'active' ? '正常' : wallet?.status === 'frozen' ? '已冻结' : '-'
          "
          :footer="selectedUserLabel"
        />
      </section>

      <AdminTablePanel>
        <template #toolbar>
          <AdminResourceToolbar>
            <template #start>
              <div class="min-w-0">
                <div class="text-admin-title-sm font-[740] text-admin-title-color">Ledger 流水</div>
                <div class="mt-0.5 text-admin-caption text-admin-text-3">
                  {{ selectedUserLabel }} · User #{{ selectedUserId }}
                </div>
              </div>
            </template>
            <template #actions>
              <Button
                label="赠送"
                size="small"
                :disabled="walletLoading || !wallet"
                @click="openCommand('grant')"
              />
              <Button
                label="扣减"
                size="small"
                severity="danger"
                outlined
                :disabled="walletLoading || !wallet || wallet.available_credits < 1"
                @click="openCommand('deduction')"
              />
              <Button
                aria-label="刷新 Wallet 与 Ledger"
                text
                :class="adminRoundIconButtonClass"
                :loading="walletLoading || ledgerLoading"
                @click="refreshWallet"
              >
                <template #icon><Refresh class="size-4" /></template>
              </Button>
            </template>
            <template #pager>
              <AdminPagerControls
                :current-page="ledgerPage"
                :total-pages="ledgerKnownPageCount"
                :prev-disabled="!ledgerCanRetreat"
                :next-disabled="!ledgerCanAdvance"
                @prev="previousLedgerPage"
                @next="nextLedgerPage"
              />
            </template>
          </AdminResourceToolbar>
        </template>

        <div v-if="walletError || ledgerError" class="p-6">
          <Message severity="error" :closable="false">
            Wallet 或流水加载失败。请检查用户 ID 后刷新重试。
          </Message>
        </div>
        <DataTableFrame
          v-else
          :value="transactions"
          data-key="transaction_id"
          :loading="ledgerLoading"
          scrollable
          table-style="min-width: 1080px"
        >
          <template #empty>
            <div class="py-12 text-center text-admin-body text-admin-text-3">
              该 Wallet 暂无流水
            </div>
          </template>
          <Column header="类型" style="width: 92px">
            <template #body="{ data }">
              <Tag
                :value="getCreditCommandLabel(data.command_type)"
                :severity="getCreditCommandSeverity(data.command_type)"
                :class="adminCompactTagClass"
              />
            </template>
          </Column>
          <Column header="Idea" style="width: 140px">
            <template #body="{ data }">
              <span class="font-[760] tabular-nums">
                {{ getCreditCommandSign(data.command_type)
                }}{{ formatCredits(data.amount_credits) }}
              </span>
            </template>
          </Column>
          <Column header="余额快照" style="width: 190px">
            <template #body="{ data }">
              <div class="text-admin-body">
                可用 {{ formatCredits(data.available_credits_after) }}
              </div>
              <div class="text-admin-caption text-admin-text-3">
                冻结 {{ formatCredits(data.reserved_credits_after) }}
              </div>
            </template>
          </Column>
          <Column header="业务关联" style="min-width: 240px">
            <template #body="{ data }">
              <div class="flex flex-wrap gap-1.5">
                <span
                  class="rounded-[var(--radius-ib-pill)] bg-ib-surface-muted px-2 py-1 text-admin-caption font-[680]"
                >
                  {{ data.business_reference.type }} · {{ data.business_reference.id }}
                </span>
                <span
                  v-if="data.related_transaction_id"
                  class="max-w-56 truncate rounded-[var(--radius-ib-control)] border border-admin-border-soft px-2 py-1 font-mono text-admin-caption"
                  :title="data.related_transaction_id"
                >
                  Tx · {{ data.related_transaction_id }}
                </span>
              </div>
              <div v-if="data.hold" class="mt-1 text-admin-caption text-admin-text-3">
                Hold {{ data.hold.status }}
                <span v-if="data.hold.terminal_transaction_id">
                  · {{ data.hold.terminal_transaction_id }}</span
                >
              </div>
              <div v-if="data.charge" class="mt-1 text-admin-caption text-admin-text-3">
                已退 {{ formatCredits(data.charge.refunded_credits) }} · 可退
                {{ formatCredits(data.charge.remaining_refundable_credits) }}
              </div>
            </template>
          </Column>
          <Column header="原因 / 操作者" style="min-width: 220px">
            <template #body="{ data }">
              <div class="max-w-72 truncate" :title="data.reason">{{ data.reason }}</div>
              <div class="mt-1 text-admin-caption text-admin-text-3">
                {{ data.actor.type }} · {{ data.actor.id }}
              </div>
            </template>
          </Column>
          <Column header="时间" style="width: 180px">
            <template #body="{ data }">{{ formatAdminDateTime(data.created_at) }}</template>
          </Column>
          <Column header="操作" style="width: 92px">
            <template #body="{ data }">
              <Button
                v-if="isRefundableCharge(data)"
                label="退款"
                size="small"
                severity="danger"
                text
                @click="openRefund(data)"
              />
              <span v-else class="text-admin-caption text-admin-text-3">-</span>
            </template>
          </Column>
        </DataTableFrame>
      </AdminTablePanel>
    </template>

    <div
      v-else
      class="flex min-h-64 flex-col items-center justify-center rounded-[var(--radius-ib-panel)] border border-dashed border-admin-border-soft bg-ib-surface text-center"
    >
      <div class="text-admin-title-sm font-[720] text-admin-title-color">先选择一个用户 Wallet</div>
      <div class="mt-1 text-admin-body text-admin-text-3">
        输入用户 ID，或通过用户名 / UID 搜索。
      </div>
    </div>

    <CreditCommandDialog
      v-if="selectedUserId && activeCommand"
      v-model:visible="commandDialogVisible"
      :kind="activeCommand"
      :user-id="selectedUserId"
      :target-label="selectedUserLabel"
      :charge-transaction-id="refundSource?.transaction_id"
      :remaining-refundable-credits="refundSource?.charge?.remaining_refundable_credits"
      :loading="commandPending"
      @submit="requestCommandConfirmation"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { Refresh } from '@iconoir/vue'
import Button from 'primevue/button'
import Column from 'primevue/column'
import InputNumber from 'primevue/inputnumber'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import Tag from 'primevue/tag'
import AdminPagerControls from '../../shared/components/AdminPagerControls.vue'
import AdminResourceToolbar from '../../shared/components/AdminResourceToolbar.vue'
import AdminSummaryCard from '../../shared/components/AdminSummaryCard.vue'
import AdminTablePanel from '../../shared/components/AdminTablePanel.vue'
import { useAdminConfirm } from '../../shared/composables/useAdminConfirm'
import { useAdminCursorPagination } from '../../shared/composables/useAdminCursorPagination'
import DataTableFrame from '../../../../shared/components/DataTableFrame.vue'
import { adminCompactTagClass, adminRoundIconButtonClass } from '../../shared/lib/adminUiClasses'
import { useAdminUsersResource, type AdminUserListItem } from '../../users/adminUsersResource'
import { useCreditsBillingFeedback } from '../creditsBillingFeedback'
import {
  formatAdminDateTime,
  formatCredits,
  getCreditCommandLabel,
  getCreditCommandSeverity,
  getCreditCommandSign,
} from '../creditsBillingFormat'
import {
  useAdminCreditWalletResource,
  type AdminCreditLedgerTransaction,
} from './adminCreditWalletResource'
import CreditCommandDialog from './CreditCommandDialog.vue'
import type { AdminCreditCommandDraft, AdminCreditCommandKind } from './creditCommandContract'

const directUserId = ref<number | null>(null)
const selectedUserId = ref<number | null>(null)
const selectedUser = ref<AdminUserListItem | null>(null)
const userSearchKeyword = ref('')
const appliedUserSearch = ref('')
const activeCommand = ref<AdminCreditCommandKind | null>(null)
const commandDialogVisible = ref(false)
const refundSource = ref<AdminCreditLedgerTransaction | null>(null)

const feedback = useCreditsBillingFeedback()
const confirm = useAdminConfirm()
const usersQuery = useAdminUsersResource(
  computed(() => ({
    page: 1,
    page_size: 8,
    keyword: appliedUserSearch.value || undefined,
  })),
)
const userSearchResults = computed(() => usersQuery.data.value?.list ?? [])
const usersLoading = computed(() => usersQuery.isLoading.value || usersQuery.isFetching.value)
const selectedUserLabel = computed(
  () => selectedUser.value?.username || `用户 #${selectedUserId.value}`,
)

const ledgerPagination = useAdminCursorPagination<number>({
  pending: () =>
    walletResource.ledgerQuery.isLoading.value || walletResource.ledgerQuery.isFetching.value,
})
const ledgerBeforeSequence = ledgerPagination.cursor
const walletResource = useAdminCreditWalletResource(selectedUserId, ledgerBeforeSequence)
const { wallet, ledger, commandPending } = walletResource
const transactions = computed(() => ledger.value?.transactions ?? [])
const walletLoading = computed(
  () => walletResource.walletQuery.isLoading.value || walletResource.walletQuery.isFetching.value,
)
const ledgerLoading = computed(
  () => walletResource.ledgerQuery.isLoading.value || walletResource.ledgerQuery.isFetching.value,
)
const walletError = computed(() => walletResource.walletQuery.error.value)
const ledgerError = computed(() => walletResource.ledgerQuery.error.value)
const ledgerPage = ledgerPagination.currentPage
const ledgerKnownPageCount = ledgerPagination.knownPageCount
const ledgerCanAdvance = ledgerPagination.canAdvance
const ledgerCanRetreat = ledgerPagination.canRetreat

watch(walletError, (error) => {
  if (error) feedback.walletLoadFailed(error)
})
watch(ledgerError, (error) => {
  if (error) feedback.ledgerLoadFailed(error)
})
watch(
  [
    () => walletResource.ledgerQuery.dataUpdatedAt.value,
    () => walletResource.ledgerQuery.isFetching.value,
    () => walletResource.ledgerQuery.isPlaceholderData.value,
    () => walletResource.ledgerDataCriteria.value,
    () => selectedUserId.value,
    () => ledgerBeforeSequence.value,
  ],
  () => {
    const criteria = walletResource.ledgerDataCriteria.value
    if (
      walletResource.ledgerQuery.isFetching.value ||
      walletResource.ledgerQuery.isPlaceholderData.value ||
      !criteria ||
      criteria.userId !== selectedUserId.value ||
      criteria.beforeSequence !== ledgerBeforeSequence.value
    ) {
      return
    }
    ledgerPagination.setNextCursor(ledger.value?.next_before_sequence)
  },
  { immediate: true },
)
watch(selectedUserId, () => {
  ledgerPagination.reset()
})

function searchUsers() {
  const nextKeyword = userSearchKeyword.value.trim()
  if (appliedUserSearch.value === nextKeyword) {
    void usersQuery.refetch()
    return
  }
  appliedUserSearch.value = nextKeyword
}

function selectUser(user: AdminUserListItem) {
  selectedUser.value = user
  selectedUserId.value = user.id
  directUserId.value = user.id
}

function openDirectUser() {
  if (!directUserId.value || directUserId.value < 1) return
  selectedUser.value =
    userSearchResults.value.find((user) => user.id === directUserId.value) ?? null
  selectedUserId.value = directUserId.value
}

function refreshWallet() {
  void Promise.all([walletResource.walletQuery.refetch(), walletResource.ledgerQuery.refetch()])
}

function nextLedgerPage() {
  ledgerPagination.advance()
}

function previousLedgerPage() {
  ledgerPagination.retreat()
}

function openCommand(kind: Exclude<AdminCreditCommandKind, 'refund'>) {
  refundSource.value = null
  activeCommand.value = kind
  commandDialogVisible.value = true
}

function isRefundableCharge(transaction: AdminCreditLedgerTransaction) {
  return (
    transaction.command_type === 'capture' &&
    (transaction.charge?.remaining_refundable_credits ?? 0) > 0
  )
}

function openRefund(transaction: AdminCreditLedgerTransaction) {
  if (!isRefundableCharge(transaction)) return
  refundSource.value = transaction
  activeCommand.value = 'refund'
  commandDialogVisible.value = true
}

function requestCommandConfirmation(draft: AdminCreditCommandDraft) {
  if (!selectedUserId.value || !activeCommand.value || draft.amountCredits == null) return
  const kind = activeCommand.value
  const label = kind === 'grant' ? '赠送' : kind === 'deduction' ? '扣减' : '退款'
  confirm.request({
    title: `确认${label}Idea`,
    message: `将为 ${selectedUserLabel.value} ${label} ${formatCredits(draft.amountCredits)}。原因：${draft.reason}`,
    confirmLabel: `确认${label}`,
    intent: kind === 'grant' ? 'warn' : 'danger',
    onConfirm: async () => {
      try {
        const response = await walletResource.executeCommand({
          ...draft,
          kind,
          userId: selectedUserId.value as number,
          chargeTransactionId: refundSource.value?.transaction_id,
          remainingRefundableCredits: refundSource.value?.charge?.remaining_refundable_credits,
        })
        feedback.commandSucceeded(label, response.replayed)
        commandDialogVisible.value = false
      } catch (error) {
        feedback.commandFailed(error, label)
      }
    },
  })
}
</script>
