<template>
  <AdminDialogShell
    :visible="visible"
    :aria-label="`${user.displayName} 的Idea账户`"
    description="查看用户 Idea 账户资产并执行入账操作。"
    size="wide"
    content-layout="scrollable"
    :busy="commandPending"
    @update:visible="emit('update:visible', $event)"
  >
    <template #header>
      <div class="flex items-center gap-3">
        <div
          class="flex size-10 shrink-0 items-center justify-center rounded-xl border border-admin-border-soft bg-admin-surface-muted text-admin-title-sm font-bold text-admin-accent select-none shadow-2xs"
        >
          {{ (user.displayName || 'U').substring(0, 1).toUpperCase() }}
        </div>
        <div class="min-w-0">
          <div class="text-admin-title-sm font-bold text-admin-title-color truncate">
            {{ user.displayName }} 的账户资产
          </div>
          <div class="text-admin-caption font-mono text-admin-subtitle-color truncate">
            UID: {{ user.publicId }}
          </div>
        </div>
      </div>
    </template>

    <div class="flex min-w-0 flex-col gap-4 py-1">
      <!-- 1. 顶部资产概览卡片 (Stats Hero Card) -->
      <section
        class="flex flex-col gap-3.5 rounded-2xl border border-admin-border-soft bg-white p-4.5 shadow-2xs"
        aria-label="账户资产概览"
      >
        <div class="flex flex-wrap items-center justify-between gap-4">
          <!-- 核心余额与状态 -->
          <div class="flex flex-wrap items-center gap-6 sm:gap-8">
            <div class="flex flex-col gap-0.5">
              <span class="text-admin-caption font-semibold text-admin-text-3"> 账户余额 </span>
              <div class="flex items-baseline gap-2">
                <span
                  class="text-2xl font-black tracking-tight text-admin-title-color font-mono tabular-nums"
                >
                  {{ formatIdeaAmount(wallet?.available_credits ?? user.availableCredits) }}
                </span>
                <span
                  v-if="wallet && wallet.reserved_credits > 0"
                  class="rounded-md bg-admin-warning-soft px-1.5 py-0.5 font-mono text-admin-caption font-semibold text-admin-warning"
                >
                  保留 {{ formatIdeaAmount(wallet.reserved_credits) }}
                </span>
              </div>
            </div>

            <div class="hidden h-9 w-px bg-admin-border-soft sm:block" />

            <div class="flex flex-col gap-0.5">
              <span class="text-admin-caption font-semibold text-admin-text-3"> 账户状态 </span>
              <div class="flex items-center gap-1.5 pt-0.5">
                <AdminStatusPill :label="accountStatus" :variant="accountStatusVariant" />
              </div>
            </div>
          </div>

          <!-- 快捷操作按钮组 -->
          <div class="flex items-center gap-2">
            <AdminButton
              label="赠送"
              :class="adminActionButtonClass"
              size="small"
              :disabled="commandPending || !wallet"
              @click="openCommand('grant')"
            >
              <template #icon>
                <Plus class="size-4 mr-1.5" />
              </template>
            </AdminButton>
            <AdminButton
              label="扣减"
              :class="adminActionButtonClass"
              size="small"
              severity="danger"
              outlined
              :disabled="commandPending || !wallet || wallet.available_credits < 1"
              @click="openCommand('deduction')"
            >
              <template #icon>
                <Minus class="size-4 mr-1.5" />
              </template>
            </AdminButton>
          </div>
        </div>

        <!-- 限制与冻结状态提示条 -->
        <div
          v-if="activeRestrictions.length"
          class="flex items-start gap-2 rounded-xl border border-admin-warning/30 bg-admin-warning-soft p-3 text-xs"
        >
          <WarningTriangle class="size-4 shrink-0 text-admin-warning mt-0.5" />
          <div class="flex flex-col gap-1">
            <span class="font-bold text-admin-warning">有效限制生效中</span>
            <ul class="space-y-0.5 pl-4 text-admin-text-2 list-disc">
              <li v-for="restriction in activeRestrictions" :key="restriction.restriction_id">
                <span class="font-semibold text-admin-title-color"
                  >{{ getRestrictionLabel(restriction.type) }}：</span
                >
                {{ restriction.reason }}
              </li>
            </ul>
          </div>
        </div>
        <AdminAlert v-else-if="accountStatus === '冻结'" severity="warn">
          {{
            user.status === 0
              ? '用户当前已停用或正在注销，账户由身份生命周期冻结。'
              : '账户已被身份生命周期冻结，请核对用户状态与注销流程。'
          }}
        </AdminAlert>
      </section>

      <AdminAlert v-if="walletError || ledgerError" severity="error">
        账户或流水加载失败。请刷新后重试。
      </AdminAlert>

      <!-- 2. 分段功能导航 Tab (复用全局无障碍 segmented control 规范) -->
      <div class="flex items-center justify-between">
        <div
          aria-label="Idea账户操作面板"
          class="inline-flex items-center rounded-lg border border-admin-border-soft bg-admin-surface-muted/50 p-1 text-xs"
        >
          <button
            v-for="opt in walletTabOptions"
            :key="opt.value"
            type="button"
            :aria-pressed="activeTab === opt.value"
            class="rounded-md px-3 py-1 font-medium transition-colors cursor-pointer"
            :class="
              activeTab === opt.value
                ? 'bg-white text-admin-text-1 font-semibold shadow-xs'
                : 'text-admin-text-3 hover:text-admin-text-1'
            "
            @click="handleTabChange(opt.value)"
          >
            {{ opt.label }}
          </button>
        </div>

        <AdminButton
          v-if="activeTab === 'ledger'"
          label="刷新"
          :class="adminActionButtonClass"
          size="small"
          variant="outline"
          :loading="walletLoading || ledgerLoading"
          :disabled="commandPending"
          @click="refreshWallet"
        >
          <template #icon>
            <Refresh class="size-4 mr-1.5 text-admin-text-2" />
          </template>
        </AdminButton>
      </div>

      <!-- 3. Tab 内容展示区 -->
      <!-- A. 额度变动表单 Tab (Grant / Deduction / Refund) -->
      <section
        v-if="activeTab === 'grant' || activeTab === 'deduction' || activeTab === 'refund'"
        class="flex flex-col gap-4 rounded-2xl border border-admin-border-soft bg-white p-5 shadow-2xs"
      >
        <div class="flex items-center justify-between border-b border-admin-border-soft pb-3">
          <div class="flex items-center gap-2">
            <span
              class="size-2 rounded-full"
              :class="
                activeTab === 'grant'
                  ? 'bg-admin-success'
                  : activeTab === 'deduction'
                    ? 'bg-admin-danger'
                    : 'bg-admin-warning'
              "
            />
            <h3 class="m-0 text-admin-title-sm font-bold text-admin-title-color">
              {{
                activeTab === 'grant'
                  ? '赠送 Idea 额度'
                  : activeTab === 'deduction'
                    ? '扣减 Idea 额度'
                    : '退款原交易'
              }}
            </h3>
          </div>
          <span class="text-admin-caption text-admin-text-3">
            {{
              activeTab === 'grant'
                ? '赠送的额度将即时计入用户可用余额'
                : activeTab === 'deduction'
                  ? '扣减额度不得超过用户当前可用余额'
                  : '退款将返还对应 Charge 的消费额度'
            }}
          </span>
        </div>

        <div
          v-if="activeTab === 'refund' && refundSource"
          class="flex items-center gap-2 rounded-xl border border-admin-accent/20 bg-admin-accent-soft/40 px-3.5 py-2.5 text-xs text-admin-accent-strong font-mono"
        >
          <span
            >原 Charge：{{ refundSource.transaction_id }}，剩余可退
            {{ formatIdeaAmount(refundSource.charge?.remaining_refundable_credits ?? 0) }}。</span
          >
        </div>

        <form class="flex flex-col gap-4" @submit.prevent="requestCommandConfirmation">
          <div class="grid grid-cols-1 gap-4 sm:grid-cols-12 sm:items-start">
            <div class="sm:col-span-4">
              <AdminFormField
                v-slot="{ describedbyId, invalid }"
                label="Idea数量"
                for="admin-user-wallet-command-amount"
                :error="showErrors ? validation.amountError : ''"
                required
              >
                <AdminNumberInput
                  v-model="amountCredits"
                  input-id="admin-user-wallet-command-amount"
                  :min="1"
                  :use-grouping="true"
                  placeholder="例如：100"
                  :invalid="invalid"
                  :disabled="commandPending"
                  :aria-describedby="describedbyId"
                  @update:model-value="showErrors = false"
                />
              </AdminFormField>
            </div>

            <div class="sm:col-span-8">
              <AdminActionReasonField
                v-model="reason"
                input-id="admin-user-wallet-command-reason"
                :label="activeTab === 'refund' ? '退款原因' : '操作原因'"
                :error="showErrors ? validation.reasonError : ''"
                :disabled="commandPending"
                :rows="1"
                @update:model-value="showErrors = false"
              />
            </div>
          </div>

          <div class="flex items-center justify-end gap-2.5 pt-1">
            <AdminButton
              label="取消"
              type="button"
              :class="adminActionButtonClass"
              variant="outline"
              :disabled="commandPending"
              @click="switchToLedgerTab"
            />
            <AdminButton
              :label="activeTab === 'refund' ? '核对退款' : '核对操作'"
              :class="adminActionButtonClass"
              type="submit"
              :variant="activeTab === 'grant' ? 'default' : 'destructive'"
              :loading="commandPending"
              :disabled="commandPending"
            />
          </div>
        </form>
      </section>

      <!-- B. Ledger 流水明细 Tab (Ledger Transactions Table) -->
      <section
        v-else
        class="flex flex-col gap-3.5 rounded-2xl border border-admin-border-soft bg-white p-4.5 shadow-2xs"
        aria-labelledby="admin-user-wallet-ledger-title"
      >
        <div class="overflow-hidden rounded-xl border border-admin-border-soft">
          <AdminTable
            :value="transactions"
            data-key="transaction_id"
            :loading="ledgerLoading && transactions.length === 0"
            scrollable
            table-aria-label="Idea账户 Ledger 流水"
            scroll-aria-label="Idea账户 Ledger 流水"
            table-style="min-width: 780px"
          >
            <template #empty>
              <AdminEmptyState compact title="该账户暂无流水" />
            </template>

            <AdminColumn header="类型" style="width: 96px">
              <template #body="{ data }">
                <AdminBadge
                  :value="getCreditCommandLabel(data.command_type)"
                  :severity="getCreditCommandSeverity(data.command_type)"
                  :class="adminCompactTagClass"
                />
              </template>
            </AdminColumn>

            <AdminColumn header="变动额度" style="width: 120px">
              <template #body="{ data }">
                <span
                  class="font-bold tabular-nums font-mono text-admin-body"
                  :class="
                    data.command_type === 'grant' || data.command_type === 'refund'
                      ? 'text-admin-success'
                      : 'text-admin-title-color'
                  "
                >
                  {{ getCreditCommandSign(data.command_type)
                  }}{{ formatIdeaAmount(data.amount_credits) }}
                </span>
              </template>
            </AdminColumn>

            <AdminColumn header="变动后可用" style="width: 150px">
              <template #body="{ data }">
                <span class="font-mono text-admin-caption text-admin-text-2">
                  可用 {{ formatIdeaAmount(data.available_credits_after) }}
                </span>
              </template>
            </AdminColumn>

            <AdminColumn header="原因 / 关联" style="min-width: 220px">
              <template #body="{ data }">
                <div
                  class="max-w-72 truncate text-admin-body-sm text-admin-text-1"
                  :title="data.reason"
                >
                  {{ data.reason || '-' }}
                </div>
                <div
                  v-if="data.charge"
                  class="mt-0.5 text-admin-caption text-admin-text-3 font-mono"
                >
                  可退 {{ formatIdeaAmount(data.charge.remaining_refundable_credits) }}
                </div>
              </template>
            </AdminColumn>

            <AdminColumn header="时间" style="width: 160px">
              <template #body="{ data }">
                <span class="font-mono text-admin-caption text-admin-text-3 whitespace-nowrap">
                  {{ formatAdminDateTimeWithSeconds(data.created_at) }}
                </span>
              </template>
            </AdminColumn>

            <AdminColumn header="操作" style="width: 80px">
              <template #body="{ data }">
                <AdminButton
                  v-if="isRefundableCharge(data)"
                  label="退款"
                  size="small"
                  variant="ghost"
                  class="text-admin-danger hover:text-admin-danger-hover"
                  :disabled="commandPending"
                  :aria-label="`退还 ${data.transaction_id} 的Idea`"
                  @click="openRefund(data)"
                />
                <span v-else class="text-admin-caption text-admin-text-3">-</span>
              </template>
            </AdminColumn>
          </AdminTable>
        </div>

        <div v-if="nextBeforeSequence != null" class="mt-2 flex justify-center">
          <AdminButton
            label="加载更多 Ledger"
            :class="adminActionButtonClass"
            variant="outline"
            :loading="ledgerLoading && transactions.length > 0"
            :disabled="ledgerLoading || commandPending"
            aria-controls="admin-user-wallet-ledger-title"
            @click="loadMore"
          />
        </div>
      </section>
    </div>
  </AdminDialogShell>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { Minus, Plus, Refresh, WarningTriangle } from '@iconoir/vue'
import AdminColumn from '../../shared/components/AdminColumn.vue'
import AdminAlert from '../../shared/components/AdminAlert.vue'
import AdminBadge from '../../shared/components/AdminBadge.vue'
import AdminButton from '../../shared/components/AdminButton.vue'
import AdminNumberInput from '../../shared/components/AdminNumberInput.vue'
import AdminTable from '../../shared/components/AdminTable.vue'
import AdminDialogShell from '../../shared/components/AdminDialogShell.vue'
import AdminEmptyState from '../../shared/components/AdminEmptyState.vue'
import AdminFormField from '../../shared/components/AdminFormField.vue'
import AdminStatusPill from '../../shared/components/AdminStatusPill.vue'
import { useAdminConfirm } from '../../shared/composables/useAdminConfirm'
import { adminActionButtonClass, adminCompactTagClass } from '../../shared/lib/uiClasses'
import AdminActionReasonField from '../shared/AdminActionReasonField.vue'
import { useAdminCreditsBillingOperationsFeedback } from '../shared/feedback'
import {
  formatAdminDateTimeWithSeconds,
  getCreditCommandLabel,
  getCreditCommandSeverity,
  getCreditCommandSign,
} from '../shared/format'
import { formatIdeaAmount } from '../../../../shared/i18n/userIdeaCurrency'
import { useAdminCreditWalletResource, type AdminCreditLedgerTransaction } from './resource'
import { type AdminCreditCommandKind, validateAdminCreditCommand } from './creditCommandContract'

type AdminUserWalletTarget = {
  publicId: string
  displayName: string
  status: number
  availableCredits: number
}

const props = defineProps<{ visible: boolean; user: AdminUserWalletTarget }>()
const emit = defineEmits<{ 'update:visible': [visible: boolean]; 'refresh-users': [] }>()
const confirm = useAdminConfirm()
const feedback = useAdminCreditsBillingOperationsFeedback()
const beforeSequence = ref<number | undefined>()
const transactions = ref<AdminCreditLedgerTransaction[]>([])
const nextBeforeSequence = ref<number | undefined>()
const loadedCursors = new Set<string>()
const activeCommand = ref<AdminCreditCommandKind | null>(null)
const refundSource = ref<AdminCreditLedgerTransaction | null>(null)
const amountCredits = ref<number | null>(null)
const reason = ref('')
const showErrors = ref(false)
const resource = useAdminCreditWalletResource(
  () => (props.visible ? props.user.publicId : null),
  beforeSequence,
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
  if (wallet.value?.status === 'frozen') return '冻结'
  if (activeRestrictions.value.length > 0) return '受限'
  return '正常'
})
const accountStatusVariant = computed<'success' | 'warning' | 'danger'>(() => {
  if (accountStatus.value === '正常') return 'success'
  if (accountStatus.value === '受限') return 'warning'
  return 'danger'
})
type WalletDialogTab = 'ledger' | 'grant' | 'deduction' | 'refund'
const activeTab = ref<WalletDialogTab>('ledger')

const walletTabOptions = computed<{ label: string; value: WalletDialogTab }[]>(() => {
  const options: { label: string; value: WalletDialogTab }[] = [
    { label: 'Ledger 流水', value: 'ledger' },
    { label: '赠送额度', value: 'grant' },
    { label: '扣减额度', value: 'deduction' },
  ]
  if (activeTab.value === 'refund' || refundSource.value) {
    options.push({ label: '退款原交易', value: 'refund' })
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
      criteria.beforeSequence !== beforeSequence.value
    )
      return
    const cursorKey = String(criteria.beforeSequence ?? 'first')
    if (loadedCursors.has(cursorKey)) return
    loadedCursors.add(cursorKey)
    const known = new Set(transactions.value.map((transaction) => transaction.transaction_id))
    transactions.value.push(
      ...page.transactions.filter((transaction) => !known.has(transaction.transaction_id)),
    )
    nextBeforeSequence.value = page.next_before_sequence
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
  beforeSequence.value = undefined
  transactions.value = []
  nextBeforeSequence.value = undefined
  loadedCursors.clear()
  void Promise.all([resource.walletQuery.refetch(), resource.ledgerQuery.refetch()])
}
function loadMore() {
  if (nextBeforeSequence.value == null || ledgerLoading.value) return
  beforeSequence.value = nextBeforeSequence.value
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
function isRefundableCharge(transaction: AdminCreditLedgerTransaction) {
  return (
    transaction.command_type === 'capture' &&
    (transaction.charge?.remaining_refundable_credits ?? 0) > 0
  )
}
function requestCommandConfirmation() {
  showErrors.value = true
  if (!activeCommand.value || !validation.value.valid || amountCredits.value == null) return
  const kind = activeCommand.value
  const label = kind === 'grant' ? '赠送' : kind === 'deduction' ? '扣减' : '退款'
  confirm.request({
    title: `确认${label}Idea`,
    message: `将为 ${props.user.displayName} ${label} ${formatIdeaAmount(amountCredits.value)}。原因：${reason.value.trim()}`,
    confirmLabel: `确认${label}`,
    intent: kind === 'grant' ? 'warn' : 'danger',
    onConfirm: async () => {
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
    },
  })
}
function getRestrictionLabel(type: string) {
  return type === 'ledger_integrity' ? '账本完整性' : type
}
</script>
