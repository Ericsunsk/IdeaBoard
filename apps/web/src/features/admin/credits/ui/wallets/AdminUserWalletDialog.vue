<template>
  <AdminDialogShell
    :visible="isVisible"
    :aria-label="dialogAriaLabel"
    :description="
      isAccountMode
        ? t('admin.wallet.accountDialogDescription')
        : t('admin.wallet.dialogDescription')
    "
    size="wide"
    content-layout="scrollable"
    :busy="isCommandPending"
    @update:visible="emit('update:isVisible', $event)"
  >
    <template #header>
      <div class="flex items-center gap-3">
        <div
          class="flex size-10 shrink-0 items-center justify-center rounded-control border border-border bg-muted text-admin-title-sm font-bold text-primary select-none shadow-raised"
        >
          {{ (target.displayName || 'U').substring(0, 1).toUpperCase() }}
        </div>
        <div class="min-w-0">
          <div class="text-admin-title-sm font-bold text-foreground truncate">
            {{ targetTitle }}
          </div>
          <div class="text-admin-caption font-mono text-muted-foreground truncate">
            {{ targetSubtitle }}
          </div>
          <div
            v-if="isAccountMode && wallet?.owner_type"
            class="text-admin-caption text-muted-foreground truncate"
          >
            {{
              wallet.owner_type === 'organization'
                ? t('admin.wallet.ownerTypeOrganization')
                : t('admin.wallet.ownerTypeWorkspace')
            }}
          </div>
        </div>
      </div>
    </template>

    <div class="flex min-w-0 flex-col gap-4 py-1">
      <!-- 0. 账户归属与财务成员（仅账户模式：审计身份关联） -->
      <div
        v-if="isAccountMode"
        class="flex flex-col gap-1.5 rounded-panel border border-border bg-muted/50 px-3.5 py-2.5 text-admin-meta"
      >
        <div class="flex flex-wrap items-center gap-x-4 gap-y-1">
          <span class="text-muted-foreground">
            {{ t('admin.billingAccount.linkageColumn') }}：
            <span class="font-mono font-semibold text-foreground">{{
              accountDetail?.workspace_id ?? accountDetail?.organization_id ?? ''
            }}</span>
          </span>
          <span class="text-muted-foreground">
            {{ t('admin.billingAccount.membersTitle') }}：
            <span class="font-semibold text-foreground">{{
              memberSummary || t('admin.billingAccount.emptyMembers')
            }}</span>
          </span>
        </div>
        <div
          v-if="(accountDetail?.linked_workspaces ?? []).length > 0"
          class="text-muted-foreground"
        >
          {{ t('admin.billingAccount.linkedTitle') }}：
          <span class="font-mono font-semibold text-foreground">{{ linkedSummary }}</span>
        </div>
      </div>

      <!-- 1. 顶部资产概览卡片 (Stats Hero Card) -->
      <WalletOverviewCard
        :available-credits="wallet?.available_credits ?? fallbackAvailableCredits"
        :reserved-credits="wallet?.reserved_credits"
        :account-status="accountStatus"
        :account-status-tone="accountStatusTone"
        :active-restrictions="activeRestrictions"
        :is-user-disabled-or-closing="isUserDisabledOrClosing"
        :command-pending="isCommandPending"
        :has-wallet="Boolean(wallet)"
        @open-command="openCommand"
      />

      <!-- 2. 分段功能导航 Tab -->
      <div class="flex items-center justify-between">
        <div
          :aria-label="t('admin.wallet.actionPanelAria')"
          class="inline-flex items-center rounded-control border border-border bg-muted/50 p-1 text-xs"
        >
          <button
            v-for="opt in walletTabOptions"
            :key="opt.value"
            type="button"
            :aria-pressed="activeTab === opt.value"
            class="rounded-control px-3 py-1 font-medium transition-colors duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] cursor-pointer"
            :class="
              activeTab === opt.value
                ? 'bg-card text-foreground font-semibold shadow-raised'
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
          :disabled="isCommandPending"
          @click="refreshWallet"
        >
          <template #icon>
            <Refresh class="size-4 mr-1.5 text-muted-foreground" />
          </template>
        </ActionButton>
      </div>

      <!-- 3. Tab 内容展示区 -->
      <!-- A. 额度变动表单 Tab (Grant / Deduction / Refund / Correction) -->
      <WalletAdjustPanel
        v-if="activeTab !== 'ledger' && activeTab !== 'restrictions'"
        v-model:amount-credits="amountCredits"
        v-model:show-errors="showErrors"
        :active-tab="activeTab"
        :refund-source="refundSource"
        :correction-source="correctionSource"
        :validation="validation"
        :command-pending="isCommandPending"
        @cancel="switchToLedgerTab"
        @submit="requestCommandConfirmation"
      />

      <!-- B. Ledger 流水明细 Tab -->
      <WalletLedgerTable
        v-else-if="activeTab === 'ledger'"
        :transactions="transactions"
        :ledger-loading="ledgerLoading"
        :command-pending="isCommandPending"
        :has-next-page="nextCursor != null"
        @open-refund="openRefund"
        @open-correction="openCorrection"
        @load-more="loadMore"
      />

      <!-- C. 限制管理 Tab -->
      <WalletRestrictionPanel
        v-else
        :restrictions="activeRestrictions"
        :is-restriction-pending="isRestrictionPending"
        @release="requestReleaseConfirmation"
        @create="requestRestrictConfirmation"
      />
    </div>
  </AdminDialogShell>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Refresh } from '@iconoir/vue'
import ActionButton from '@/core/components/ActionButton.vue'
import { useFailureToast } from '@/core/composables/useFailureToast'
import { createUniqueId } from '@/core/utils/ids'
import { AdminDialogShell } from '../../../shared'
import { useConfirm } from '@/core/composables/useConfirmDialog'
import { useAdminCreditsBillingOperationsFeedback } from '../../session/shared/feedback'
import { formatIdeaAmount } from '@/core/i18n/userIdeaCurrency'
import {
  useAdminWalletResource,
  type AdminWalletTarget,
  type AdminCreditLedgerTransaction,
} from '../../session/wallets/resource'
import { useAdminBillingAccountDetailResource } from '../../session/accounts/resource'
import {
  type AdminCreditCommandKind,
  type AdminCreditCommandInput,
  isCorrectableTransaction,
  isRefundableCharge,
  validateAdminCreditCommand,
} from '../../core/wallets/creditCommandContract'
import type { WalletRestrictionDraft } from './components/WalletRestrictionPanel.vue'
import WalletOverviewCard from './components/WalletOverviewCard.vue'
import WalletAdjustPanel from './components/WalletAdjustPanel.vue'
import WalletLedgerTable from './components/WalletLedgerTable.vue'
import WalletRestrictionPanel from './components/WalletRestrictionPanel.vue'

type WalletDialogTarget = AdminWalletTarget & {
  displayName: string
} & ({ kind: 'user'; status: number; availableCredits: number } | { kind: 'billingAccount' })

const props = defineProps<{
  isVisible: boolean
  target: WalletDialogTarget
}>()
const emit = defineEmits<{ 'update:isVisible': [isVisible: boolean]; 'refresh-users': [] }>()
const { t } = useI18n()
const confirm = useConfirm()
const feedback = useAdminCreditsBillingOperationsFeedback()

const isAccountMode = computed(() => props.target.kind === 'billingAccount')
const targetTitle = computed(() =>
  props.target.kind === 'billingAccount'
    ? t('admin.wallet.accountAssetsTitle', { id: props.target.id })
    : t('admin.wallet.userAssetsTitle', { name: props.target.displayName }),
)
const targetSubtitle = computed(() => `${isAccountMode.value ? 'ba' : 'UID'}: ${props.target.id}`)
const dialogAriaLabel = computed(() =>
  props.target.kind === 'billingAccount'
    ? t('admin.wallet.accountDialogAria', { id: props.target.id })
    : t('admin.wallet.dialogAria', { name: props.target.displayName }),
)
const isUserDisabledOrClosing = computed(
  () => props.target.kind === 'user' && props.target.status === 0,
)
const fallbackAvailableCredits = computed(() =>
  props.target.kind === 'user' ? props.target.availableCredits : 0,
)
const dialogTarget = computed(() => (props.isVisible ? props.target : null))
const dialogAccountId = computed(() =>
  dialogTarget.value?.kind === 'billingAccount' ? dialogTarget.value.id : null,
)

const ledgerCursor = ref<string | undefined>()
const transactions = ref<AdminCreditLedgerTransaction[]>([])
const nextCursor = ref<string | undefined>()
const loadedCursors = new Set<string>()
const activeCommand = ref<AdminCreditCommandKind | null>(null)
const refundSource = ref<AdminCreditLedgerTransaction | null>(null)
const correctionSource = ref<AdminCreditLedgerTransaction | null>(null)
const amountCredits = ref<number | null>(null)
const showErrors = ref(false)
const resource = useAdminWalletResource(dialogTarget, ledgerCursor)
const accountDetailResource = useAdminBillingAccountDetailResource(dialogAccountId)
const accountDetail = computed(() => accountDetailResource.detail.value)
const wallet = resource.wallet
const walletLoading = computed(
  () => resource.walletQuery.isLoading.value || resource.walletQuery.isFetching.value,
)
const ledgerLoading = computed(
  () => resource.ledgerQuery.isLoading.value || resource.ledgerQuery.isFetching.value,
)
const isCommandPending = resource.isCommandPending
const isRestrictionPending = resource.isRestrictionPending
const walletError = computed(() => resource.walletQuery.error.value)
const ledgerError = computed(() => resource.ledgerQuery.error.value)
const activeRestrictions = computed(() => wallet.value?.active_restrictions ?? [])
const accountStatus = computed(() => {
  if (wallet.value?.status === 'frozen') return t('admin.wallet.statusFrozen')
  if (activeRestrictions.value.length > 0) return t('admin.wallet.statusRestricted')
  return t('admin.wallet.statusActive')
})
const accountStatusTone = computed<'success' | 'warning' | 'danger'>(() => {
  if (accountStatus.value === t('admin.wallet.statusActive')) return 'success'
  if (accountStatus.value === t('admin.wallet.statusRestricted')) return 'warning'
  return 'danger'
})
type WalletDialogTab = 'ledger' | 'grant' | 'deduction' | 'refund' | 'correction' | 'restrictions'
const activeTab = ref<WalletDialogTab>('ledger')

const walletTabOptions = computed<{ label: string; value: WalletDialogTab }[]>(() => {
  const options: { label: string; value: WalletDialogTab }[] = [
    { label: t('admin.wallet.tabLedger'), value: 'ledger' },
    { label: t('admin.wallet.tabGrant'), value: 'grant' },
    { label: t('admin.wallet.tabDeduct'), value: 'deduction' },
    { label: t('admin.wallet.tabRestrictions'), value: 'restrictions' },
  ]
  if (activeTab.value === 'refund' || refundSource.value) {
    options.push({ label: t('admin.wallet.tabRefund'), value: 'refund' })
  }
  if (activeTab.value === 'correction' || correctionSource.value) {
    options.push({ label: t('admin.wallet.tabCorrection'), value: 'correction' })
  }
  return options
})

function handleTabChange(tab: WalletDialogTab) {
  if (tab === 'ledger' || tab === 'restrictions') {
    clearCommand()
    activeTab.value = tab
  } else if (tab === 'grant' || tab === 'deduction') {
    openCommand(tab)
  } else if (tab === 'refund') {
    activeTab.value = 'refund'
  } else {
    activeTab.value = 'correction'
  }
}
const commandDraft = computed<AdminCreditCommandInput>(() => ({
  kind: activeCommand.value ?? 'grant',
  ...(props.target.kind === 'billingAccount'
    ? { billingAccountId: props.target.id }
    : { userPublicId: props.target.id }),
  amountCredits: amountCredits.value,
  chargeTransactionId: refundSource.value?.transaction_id,
  remainingRefundableCredits: refundSource.value?.charge?.remaining_refundable_credits,
  originalTransactionId: correctionSource.value?.transaction_id,
  originalAmountCredits: correctionSource.value?.amount_credits,
}))
const validation = computed(() => validateAdminCreditCommand(commandDraft.value))
const ledgerTargetKey = computed(() => `${props.target.kind}:${props.target.id}`)
const memberSummary = computed(() =>
  (accountDetail.value?.members ?? [])
    .map((member) => `${member.display_name} (${member.role})`)
    .join('、'),
)
const linkedSummary = computed(() =>
  (accountDetail.value?.linked_workspaces ?? [])
    .map((workspace) => workspace.workspace_id)
    .join('、'),
)

watch(
  [() => props.isVisible, ledgerTargetKey],
  ([visible]) => {
    clearCommand()
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
      !props.isVisible ||
      resource.ledgerQuery.isFetching.value ||
      !criteria ||
      !page ||
      criteria.target?.kind !== props.target.kind ||
      criteria.target.id !== props.target.id ||
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
// 钱包/流水初载失败只走 toast（概览卡余额兜底 + 流水空态即恢复入口）。
useFailureToast(walletError, feedback.walletLoadFailed, { shouldFireOnMount: false })
useFailureToast(ledgerError, feedback.ledgerLoadFailed, { shouldFireOnMount: false })

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
function openCommand(kind: Exclude<AdminCreditCommandKind, 'refund' | 'correction'>) {
  refundSource.value = null
  correctionSource.value = null
  activeCommand.value = kind
  activeTab.value = kind
  amountCredits.value = null
  showErrors.value = false
}

function openRefund(transaction: AdminCreditLedgerTransaction) {
  if (!isRefundableCharge(transaction)) return
  correctionSource.value = null
  refundSource.value = transaction
  activeCommand.value = 'refund'
  activeTab.value = 'refund'
  amountCredits.value = null
  showErrors.value = false
}
function openCorrection(transaction: AdminCreditLedgerTransaction) {
  if (!isCorrectableTransaction(transaction)) return
  refundSource.value = null
  correctionSource.value = transaction
  activeCommand.value = 'correction'
  activeTab.value = 'correction'
  amountCredits.value = null
  showErrors.value = false
}
function switchToLedgerTab() {
  clearCommand()
  activeTab.value = 'ledger'
}
function clearCommand() {
  activeCommand.value = null
  refundSource.value = null
  correctionSource.value = null
  amountCredits.value = null
  showErrors.value = false
}
function commandLabel(kind: AdminCreditCommandKind) {
  if (kind === 'grant') return t('admin.wallet.grant')
  if (kind === 'deduction') return t('admin.wallet.deduct')
  if (kind === 'refund') return t('admin.wallet.refund')
  return t('admin.wallet.correct')
}
async function requestCommandConfirmation() {
  showErrors.value = true
  if (!activeCommand.value || !validation.value.valid || amountCredits.value == null) return
  const draft = commandDraft.value
  if (draft.amountCredits == null) return
  const targetKey = ledgerTargetKey.value
  const kind = draft.kind
  const label = commandLabel(kind)
  const isConfirmed = await confirm.confirm({
    title: t('admin.wallet.confirmCommandTitle', { action: label }),
    message: t('admin.wallet.confirmCommandMessage', {
      name: props.target.displayName,
      action: label,
      amount: formatIdeaAmount(draft.amountCredits),
    }),
    confirmLabel: t('admin.wallet.confirmCommandLabel', { action: label }),
    intent: kind === 'grant' ? 'warn' : 'danger',
  })
  if (!isConfirmed || !props.isVisible || ledgerTargetKey.value !== targetKey) return
  try {
    const response = await resource.executeCommand(draft)
    feedback.commandSucceeded(label, response.wasReplayed)
    clearCommand()
    activeTab.value = 'ledger'
    resetLedger()
    emit('refresh-users')
  } catch (error) {
    feedback.commandFailed(error, label)
  }
}
async function requestReleaseConfirmation(restrictionId: string) {
  const label = t('admin.wallet.restrictionRelease')
  const isConfirmed = await confirm.confirm({
    title: t('admin.wallet.confirmReleaseTitle'),
    message: t('admin.wallet.confirmReleaseMessage', { id: restrictionId }),
    confirmLabel: t('admin.wallet.confirmReleaseLabel'),
    intent: 'warn',
  })
  if (!isConfirmed) return
  try {
    const response = await resource.releaseMutation.mutateAsync({
      restrictionId,
      body: {},
      idempotencyKey: `control-ui:release:${createUniqueId()}`,
    })
    feedback.commandSucceeded(label, response.replayed === true)
    resetLedger()
    emit('refresh-users')
  } catch (error) {
    feedback.commandFailed(error, label)
  }
}
async function requestRestrictConfirmation(draft: WalletRestrictionDraft) {
  const label = t('admin.wallet.restrictionCreate')
  const billingAccountId =
    props.target.kind === 'billingAccount'
      ? props.target.id
      : (wallet.value?.billing_account_id ?? '')
  if (!billingAccountId) {
    feedback.commandFailed(new Error('missing billing account'), label)
    return
  }
  const isConfirmed = await confirm.confirm({
    title: t('admin.wallet.confirmRestrictTitle'),
    message: t('admin.wallet.confirmRestrictMessage', {
      id: billingAccountId,
      type: draft.type,
    }),
    confirmLabel: t('admin.wallet.confirmRestrictLabel'),
    intent: 'danger',
  })
  if (!isConfirmed) return
  try {
    const response = await resource.restrictMutation.mutateAsync({
      body: {
        billing_account_id: billingAccountId,
        type: draft.type,
        source: 'control_review',
        related_object_type: draft.relatedObjectType,
        related_object_id: draft.relatedObjectId,
      },
      idempotencyKey: `control-ui:restrict:${createUniqueId()}`,
    })
    feedback.commandSucceeded(label, response.replayed === true)
    resetLedger()
    emit('refresh-users')
  } catch (error) {
    feedback.commandFailed(error, label)
  }
}
</script>
