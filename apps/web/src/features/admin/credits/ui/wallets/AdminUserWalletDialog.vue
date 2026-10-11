<template>
  <AdminDialogShell
    :visible="visible"
    :aria-label="t('admin.wallet.dialogAria', { name: user.displayName })"
    :description="t('admin.wallet.dialogDescription')"
    size="wide"
    content-layout="scrollable"
    :busy="commandPending"
    @update:visible="emit('update:visible', $event)"
  >
    <template #header>
      <div class="flex items-center gap-3">
        <div
          class="flex size-10 shrink-0 items-center justify-center rounded-xl border border-border bg-muted text-admin-title-sm font-bold text-primary select-none shadow-2xs"
        >
          {{ (user.displayName || 'U').substring(0, 1).toUpperCase() }}
        </div>
        <div class="min-w-0">
          <div class="text-admin-title-sm font-bold text-foreground truncate">
            {{ t('admin.wallet.userAssetsTitle', { name: user.displayName }) }}
          </div>
          <div class="text-admin-caption font-mono text-muted-foreground truncate">
            UID: {{ user.publicId }}
          </div>
        </div>
      </div>
    </template>

    <div class="flex min-w-0 flex-col gap-4 py-1">
      <!-- 1. 顶部资产概览卡片 (Stats Hero Card) -->
      <WalletOverviewCard
        :available-credits="wallet?.available_credits ?? user.availableCredits"
        :reserved-credits="wallet?.reserved_credits"
        :account-status="accountStatus"
        :account-status-variant="accountStatusVariant"
        :active-restrictions="activeRestrictions"
        :is-user-disabled-or-closing="user.status === 0"
        :command-pending="commandPending"
        :has-wallet="Boolean(wallet)"
        @open-command="openCommand"
      />

      <!-- 2. 分段功能导航 Tab -->
      <div class="flex items-center justify-between">
        <div
          :aria-label="t('admin.wallet.actionPanelAria')"
          class="inline-flex items-center rounded-lg border border-border bg-muted/50 p-1 text-xs"
        >
          <button
            v-for="opt in walletTabOptions"
            :key="opt.value"
            type="button"
            :aria-pressed="activeTab === opt.value"
            class="rounded-md px-3 py-1 font-medium transition-colors cursor-pointer"
            :class="
              activeTab === opt.value
                ? 'bg-card text-foreground font-semibold shadow-xs'
                : 'text-muted-foreground hover:text-foreground'
            "
            @click="handleTabChange(opt.value)"
          >
            {{ opt.label }}
          </button>
        </div>

        <ActionButton
          v-if="activeTab === 'ledger'"
          :label="t('admin.common.refresh')"
          variant="outline"
          :loading="walletLoading || ledgerLoading"
          :disabled="commandPending"
          @click="refreshWallet"
        >
          <template #icon>
            <Refresh class="size-4 mr-1.5 text-muted-foreground" />
          </template>
        </ActionButton>
      </div>

      <!-- 3. Tab 内容展示区 -->
      <!-- A. 额度变动表单 Tab (Grant / Deduction / Refund) -->
      <WalletAdjustPanel
        v-if="activeTab === 'grant' || activeTab === 'deduction' || activeTab === 'refund'"
        v-model:amount-credits="amountCredits"
        v-model:reason="reason"
        v-model:show-errors="showErrors"
        :active-tab="activeTab"
        :refund-source="refundSource"
        :validation="validation"
        :command-pending="commandPending"
        @cancel="switchToLedgerTab"
        @submit="requestCommandConfirmation"
      />

      <!-- B. Ledger 流水明细 Tab -->
      <WalletLedgerTable
        v-else
        :transactions="transactions"
        :ledger-loading="ledgerLoading"
        :command-pending="commandPending"
        :has-next-page="nextCursor != null"
        @open-refund="openRefund"
        @load-more="loadMore"
      />
    </div>
  </AdminDialogShell>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Refresh } from '@iconoir/vue'
import ActionButton from '../../../../../core/components/ActionButton.vue'
import { AdminDialogShell } from '../../../shared'
import { useAdminConfirm } from '../../../shared'
import { useAdminCreditsBillingOperationsFeedback } from '../../session/shared/feedback'
import { formatIdeaAmount } from '../../../../../core/i18n/userIdeaCurrency'
import { useAdminCreditWalletResource, type AdminCreditLedgerTransaction } from '../../session/wallets/resource'
import {
  type AdminCreditCommandKind,
  isRefundableCharge,
  validateAdminCreditCommand,
} from '../../core/wallets/creditCommandContract'
import WalletOverviewCard from './components/WalletOverviewCard.vue'
import WalletAdjustPanel from './components/WalletAdjustPanel.vue'
import WalletLedgerTable from './components/WalletLedgerTable.vue'

type AdminUserWalletTarget = {
  publicId: string
  displayName: string
  status: number
  availableCredits: number
}

const props = defineProps<{ visible: boolean; user: AdminUserWalletTarget }>()
const emit = defineEmits<{ 'update:visible': [visible: boolean]; 'refresh-users': [] }>()
const { t } = useI18n()
const confirm = useAdminConfirm()
const feedback = useAdminCreditsBillingOperationsFeedback()
const ledgerCursor = ref<string | undefined>()
const transactions = ref<AdminCreditLedgerTransaction[]>([])
const nextCursor = ref<string | undefined>()
const loadedCursors = new Set<string>()
const activeCommand = ref<AdminCreditCommandKind | null>(null)
const refundSource = ref<AdminCreditLedgerTransaction | null>(null)
const amountCredits = ref<number | null>(null)
const reason = ref('')
const showErrors = ref(false)
const resource = useAdminCreditWalletResource(
  () => (props.visible ? props.user.publicId : null),
  ledgerCursor,
)
const wallet = resource.wallet
const walletLoading = computed(
  () => resource.walletQuery.isLoading.value || resource.walletQuery.isFetching.value,
)
const ledgerLoading = computed(
  () => resource.ledgerQuery.isLoading.value || resource.ledgerQuery.isFetching.value,
)
const commandPending = resource.commandPending
const walletError = computed(() => resource.walletQuery.error.value)
const ledgerError = computed(() => resource.ledgerQuery.error.value)
const activeRestrictions = computed(() => wallet.value?.active_restrictions ?? [])
const accountStatus = computed(() => {
  if (wallet.value?.status === 'frozen') return t('admin.wallet.statusFrozen')
  if (activeRestrictions.value.length > 0) return t('admin.wallet.statusRestricted')
  return t('admin.wallet.statusActive')
})
const accountStatusVariant = computed<'success' | 'warning' | 'danger'>(() => {
  if (accountStatus.value === t('admin.wallet.statusActive')) return 'success'
  if (accountStatus.value === t('admin.wallet.statusRestricted')) return 'warning'
  return 'danger'
})
type WalletDialogTab = 'ledger' | 'grant' | 'deduction' | 'refund'
const activeTab = ref<WalletDialogTab>('ledger')

const walletTabOptions = computed<{ label: string; value: WalletDialogTab }[]>(() => {
  const options: { label: string; value: WalletDialogTab }[] = [
    { label: t('admin.wallet.tabLedger'), value: 'ledger' },
    { label: t('admin.wallet.tabGrant'), value: 'grant' },
    { label: t('admin.wallet.tabDeduct'), value: 'deduction' },
  ]
  if (activeTab.value === 'refund' || refundSource.value) {
    options.push({ label: t('admin.wallet.tabRefund'), value: 'refund' })
  }
  return options
})

function handleTabChange(tab: 'ledger' | 'grant' | 'deduction' | 'refund') {
  if (tab === 'ledger') {
    switchToLedgerTab()
  } else if (tab === 'grant' || tab === 'deduction') {
    openCommand(tab)
  } else if (tab === 'refund') {
    activeTab.value = 'refund'
  }
}
const validation = computed(() =>
  validateAdminCreditCommand({
    kind: activeCommand.value ?? 'grant',
    userPublicId: props.user.publicId,
    amountCredits: amountCredits.value,
    reason: reason.value,
    chargeTransactionId: refundSource.value?.transaction_id,
    remainingRefundableCredits: refundSource.value?.charge?.remaining_refundable_credits,
  }),
)

watch(
  [() => props.visible, () => props.user.publicId],
  ([visible]) => {
    if (visible) {
      activeTab.value = 'ledger'
      resetLedger()
    }
  },
  { immediate: true },
)
watch(
  [
    () => resource.ledgerQuery.dataUpdatedAt.value,
    () => resource.ledgerQuery.isFetching.value,
    () => resource.ledgerDataCriteria.value,
  ],
  () => {
    const criteria = resource.ledgerDataCriteria.value
    const page = resource.ledger.value
    if (
      !props.visible ||
      resource.ledgerQuery.isFetching.value ||
      !criteria ||
      !page ||
      criteria.userPublicId !== props.user.publicId ||
      (criteria.cursor ?? undefined) !== ledgerCursor.value
    )
      return
    const cursorKey = criteria.cursor ?? 'first'
    if (loadedCursors.has(cursorKey)) return
    loadedCursors.add(cursorKey)
    const known = new Set(transactions.value.map((transaction) => transaction.transaction_id))
    transactions.value.push(
      ...page.transactions.filter((transaction) => !known.has(transaction.transaction_id)),
    )
    nextCursor.value = page.nextCursor
  },
  { immediate: true },
)
watch(walletError, (error) => {
  if (error) feedback.walletLoadFailed(error)
})
watch(ledgerError, (error) => {
  if (error) feedback.ledgerLoadFailed(error)
})

function resetLedger() {
  ledgerCursor.value = undefined
  transactions.value = []
  nextCursor.value = undefined
  loadedCursors.clear()
  void Promise.all([resource.walletQuery.refetch(), resource.ledgerQuery.refetch()])
}
function loadMore() {
  if (nextCursor.value == null || ledgerLoading.value) return
  ledgerCursor.value = nextCursor.value
}
function refreshWallet() {
  resetLedger()
}
function openCommand(kind: Exclude<AdminCreditCommandKind, 'refund'>) {
  refundSource.value = null
  activeCommand.value = kind
  activeTab.value = kind
  amountCredits.value = null
  reason.value = ''
  showErrors.value = false
}

function openRefund(transaction: AdminCreditLedgerTransaction) {
  if (!isRefundableCharge(transaction)) return
  refundSource.value = transaction
  activeCommand.value = 'refund'
  activeTab.value = 'refund'
  amountCredits.value = null
  reason.value = ''
  showErrors.value = false
}
function switchToLedgerTab() {
  clearCommand()
  activeTab.value = 'ledger'
}
function clearCommand() {
  activeCommand.value = null
  refundSource.value = null
  amountCredits.value = null
  reason.value = ''
  showErrors.value = false
}
async function requestCommandConfirmation() {
  showErrors.value = true
  if (!activeCommand.value || !validation.value.valid || amountCredits.value == null) return
  const kind = activeCommand.value
  const label =
    kind === 'grant'
      ? t('admin.wallet.grant')
      : kind === 'deduction'
        ? t('admin.wallet.deduct')
        : t('admin.wallet.refund')
  const confirmed = await confirm.confirm({
    title: t('admin.wallet.confirmCommandTitle', { action: label }),
    message: t('admin.wallet.confirmCommandMessage', {
      name: props.user.displayName,
      action: label,
      amount: formatIdeaAmount(amountCredits.value),
      reason: reason.value.trim(),
    }),
    confirmLabel: t('admin.wallet.confirmCommandLabel', { action: label }),
    intent: kind === 'grant' ? 'warn' : 'danger',
  })
  if (!confirmed) return
  try {
    const response = await resource.executeCommand({
      kind,
      userPublicId: props.user.publicId,
      amountCredits: amountCredits.value,
      reason: reason.value,
      chargeTransactionId: refundSource.value?.transaction_id,
      remainingRefundableCredits: refundSource.value?.charge?.remaining_refundable_credits,
    })
    feedback.commandSucceeded(label, response.replayed)
    clearCommand()
    activeTab.value = 'ledger'
    resetLedger()
    emit('refresh-users')
  } catch (error) {
    feedback.commandFailed(error, label)
  }
}
</script>
