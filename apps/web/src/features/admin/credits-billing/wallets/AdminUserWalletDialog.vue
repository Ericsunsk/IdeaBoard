<template>
  <AdminDialogShell
    :visible="visible"
    :aria-label="`${user.username} 的Idea账户`"
    size="wide"
    content-layout="scrollable"
    :busy="commandPending"
    @update:visible="emit('update:visible', $event)"
  >
    <template #header>
      <div>
        <div class="text-admin-modal-title font-[780] text-admin-title-color">{{ user.username }} 的Idea账户</div>
        <div class="mt-0.5 text-admin-caption text-admin-text-3">User #{{ user.id }} · 余额仅可通过 Ledger command 变更</div>
      </div>
    </template>

    <div class="flex min-w-0 flex-col gap-5">
      <section class="grid grid-cols-1 gap-3 sm:grid-cols-2" aria-label="账户概览">
        <AdminSummaryCard label="账户余额" :value="formatCredits(wallet?.available_credits ?? user.availableCredits)" />
        <AdminSummaryCard
          label="账户状态"
          :value="accountStatus"
        />
      </section>

      <Message v-if="walletError || ledgerError" severity="error" :closable="false">
        账户或流水加载失败。请刷新后重试。
      </Message>

      <section v-if="activeRestrictions.length" class="rounded-[var(--radius-ib-control)] border border-admin-border-soft bg-ib-surface-muted p-3">
        <div class="text-admin-meta font-[740] text-admin-text-1">有效限制</div>
        <ul class="mt-2 space-y-2 pl-4 text-admin-body text-admin-text-2">
          <li v-for="restriction in activeRestrictions" :key="restriction.restriction_id">
            <span class="font-[680]">{{ getRestrictionLabel(restriction.type) }}：</span>{{ restriction.reason }}
          </li>
        </ul>
      </section>
      <Message v-else-if="accountStatus === '冻结'" severity="warn" :closable="false">
        {{
          user.status === 0
            ? '用户当前已停用或正在注销，账户由身份生命周期冻结。'
            : '账户已被身份生命周期冻结，请核对用户状态与注销流程。'
        }}
      </Message>

      <section class="rounded-[var(--radius-ib-control)] border border-admin-border-soft p-4" aria-labelledby="admin-user-wallet-command-title">
        <div class="flex flex-wrap items-center justify-between gap-3">
          <div>
            <div id="admin-user-wallet-command-title" class="text-admin-title-sm font-[740] text-admin-title-color">账户操作</div>
            <div class="mt-0.5 text-admin-caption text-admin-text-3">提交后需要二次确认；原因会写入不可变 Ledger。</div>
          </div>
          <div class="flex gap-2">
            <Button label="赠送" size="small" :disabled="commandPending || !wallet" @click="openCommand('grant')" />
            <Button label="扣减" size="small" severity="danger" outlined :disabled="commandPending || !wallet || wallet.available_credits < 1" @click="openCommand('deduction')" />
          </div>
        </div>

        <form v-if="activeCommand" class="mt-4 grid grid-cols-1 gap-3 sm:grid-cols-[minmax(0,180px)_1fr_auto] sm:items-end" @submit.prevent="requestCommandConfirmation">
          <AdminFormField v-slot="{ describedbyId, invalid }" label="Idea数量" for="admin-user-wallet-command-amount" :error="showErrors ? validation.amountError : ''" required>
            <InputNumber v-model="amountCredits" input-id="admin-user-wallet-command-amount" fluid :min="1" :use-grouping="true" :invalid="invalid" :disabled="commandPending" :aria-describedby="describedbyId" @update:model-value="showErrors = false" />
          </AdminFormField>
          <AdminActionReasonField v-model="reason" input-id="admin-user-wallet-command-reason" :label="activeCommand === 'refund' ? '退款原因' : '操作原因'" :error="showErrors ? validation.reasonError : ''" :disabled="commandPending" @update:model-value="showErrors = false" />
          <div class="flex gap-2 sm:pb-0.5">
            <Button label="取消" type="button" severity="secondary" outlined :disabled="commandPending" @click="clearCommand" />
            <Button :label="activeCommand === 'refund' ? '核对退款' : '核对操作'" type="submit" :severity="activeCommand === 'grant' ? undefined : 'danger'" :loading="commandPending" :disabled="commandPending" />
          </div>
        </form>
        <Message v-if="activeCommand === 'refund' && refundSource" severity="info" size="small" :closable="false" class="mt-3">
          原 Charge：{{ refundSource.transaction_id }}，剩余可退 {{ formatCredits(refundSource.charge?.remaining_refundable_credits ?? 0) }}。
        </Message>
      </section>

      <section aria-labelledby="admin-user-wallet-ledger-title">
        <div class="mb-3 flex flex-wrap items-center justify-between gap-3">
          <div>
            <div id="admin-user-wallet-ledger-title" class="text-admin-title-sm font-[740] text-admin-title-color">最近 Ledger 流水</div>
            <div class="mt-0.5 text-admin-caption text-admin-text-3">每次加载 25 条；退款只能从可退款 Charge 发起。</div>
          </div>
          <Button label="刷新" size="small" severity="secondary" outlined :loading="walletLoading || ledgerLoading" :disabled="commandPending" @click="refreshWallet" />
        </div>
        <DataTableFrame :value="transactions" data-key="transaction_id" :loading="ledgerLoading && transactions.length === 0" scrollable table-aria-label="Idea账户 Ledger 流水" scroll-aria-label="Idea账户 Ledger 流水" table-style="min-width: 880px">
          <template #empty><div class="py-8 text-center text-admin-body text-admin-text-3">该账户暂无流水</div></template>
          <Column header="类型" style="width: 104px"><template #body="{ data }"><Tag :value="getCreditCommandLabel(data.command_type)" :severity="getCreditCommandSeverity(data.command_type)" :class="adminCompactTagClass" /></template></Column>
          <Column header="Idea" style="width: 130px"><template #body="{ data }"><span class="font-[760] tabular-nums">{{ getCreditCommandSign(data.command_type) }}{{ formatCredits(data.amount_credits) }}</span></template></Column>
          <Column header="余额快照" style="width: 180px"><template #body="{ data }"><div>可用 {{ formatCredits(data.available_credits_after) }}</div></template></Column>
          <Column header="原因 / 关联" style="min-width: 240px"><template #body="{ data }"><div class="max-w-72 truncate" :title="data.reason">{{ data.reason }}</div><div class="mt-1 text-admin-caption text-admin-text-3">{{ data.business_reference.type }} · {{ data.business_reference.id }}</div><div v-if="data.charge" class="mt-1 text-admin-caption text-admin-text-3">可退 {{ formatCredits(data.charge.remaining_refundable_credits) }}</div></template></Column>
          <Column header="时间" style="width: 180px"><template #body="{ data }">{{ formatAdminDateTime(data.created_at) }}</template></Column>
          <Column header="操作" style="width: 108px"><template #body="{ data }"><Button v-if="isRefundableCharge(data)" label="退款" size="small" severity="danger" text :disabled="commandPending" :aria-label="`退还 ${data.transaction_id} 的Idea`" @click="openRefund(data)" /><span v-else class="text-admin-caption text-admin-text-3">-</span></template></Column>
        </DataTableFrame>
        <div class="mt-3 flex justify-center">
          <Button v-if="nextBeforeSequence != null" label="加载更多 Ledger" severity="secondary" outlined :loading="ledgerLoading && transactions.length > 0" :disabled="ledgerLoading || commandPending" aria-controls="admin-user-wallet-ledger-title" @click="loadMore" />
          <span v-else-if="transactions.length" class="text-admin-caption text-admin-text-3">已加载全部可用流水</span>
        </div>
      </section>
    </div>
  </AdminDialogShell>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import Button from 'primevue/button'
import Column from 'primevue/column'
import InputNumber from 'primevue/inputnumber'
import Message from 'primevue/message'
import Tag from 'primevue/tag'
import DataTableFrame from '../../../../shared/components/DataTableFrame.vue'
import AdminDialogShell from '../../shared/components/AdminDialogShell.vue'
import AdminFormField from '../../shared/components/AdminFormField.vue'
import AdminSummaryCard from '../../shared/components/AdminSummaryCard.vue'
import { useAdminConfirm } from '../../shared/composables/useAdminConfirm'
import { adminCompactTagClass } from '../../shared/lib/adminUiClasses'
import AdminActionReasonField from '../AdminActionReasonField.vue'
import { useAdminCreditsBillingOperationsFeedback } from '../creditsBillingFeedback'
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
import {
  type AdminCreditCommandKind,
  validateAdminCreditCommand,
} from './creditCommandContract'

type AdminUserWalletTarget = {
  id: number
  username: string
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
const resource = useAdminCreditWalletResource(() => (props.visible ? props.user.id : null), beforeSequence)
const wallet = resource.wallet
const walletLoading = computed(() => resource.walletQuery.isLoading.value || resource.walletQuery.isFetching.value)
const ledgerLoading = computed(() => resource.ledgerQuery.isLoading.value || resource.ledgerQuery.isFetching.value)
const commandPending = resource.commandPending
const walletError = computed(() => resource.walletQuery.error.value)
const ledgerError = computed(() => resource.ledgerQuery.error.value)
const activeRestrictions = computed(() => wallet.value?.active_restrictions ?? [])
const accountStatus = computed(() => {
  if (wallet.value?.status === 'frozen') return '冻结'
  if (activeRestrictions.value.length > 0) return '受限'
  return '正常'
})
const validation = computed(() => validateAdminCreditCommand({ kind: activeCommand.value ?? 'grant', userId: props.user.id, amountCredits: amountCredits.value, reason: reason.value, chargeTransactionId: refundSource.value?.transaction_id, remainingRefundableCredits: refundSource.value?.charge?.remaining_refundable_credits }))

watch([() => props.visible, () => props.user.id], ([visible]) => { if (visible) resetLedger() }, { immediate: true })
watch([() => resource.ledgerQuery.dataUpdatedAt.value, () => resource.ledgerQuery.isFetching.value, () => resource.ledgerDataCriteria.value], () => {
  const criteria = resource.ledgerDataCriteria.value
  const page = resource.ledger.value
  if (!props.visible || resource.ledgerQuery.isFetching.value || !criteria || !page || criteria.userId !== props.user.id || criteria.beforeSequence !== beforeSequence.value) return
  const cursorKey = String(criteria.beforeSequence ?? 'first')
  if (loadedCursors.has(cursorKey)) return
  loadedCursors.add(cursorKey)
  const known = new Set(transactions.value.map((transaction) => transaction.transaction_id))
  transactions.value.push(...page.transactions.filter((transaction) => !known.has(transaction.transaction_id)))
  nextBeforeSequence.value = page.next_before_sequence
}, { immediate: true })
watch(walletError, (error) => { if (error) feedback.walletLoadFailed(error) })
watch(ledgerError, (error) => { if (error) feedback.ledgerLoadFailed(error) })

function resetLedger() { beforeSequence.value = undefined; transactions.value = []; nextBeforeSequence.value = undefined; loadedCursors.clear(); void Promise.all([resource.walletQuery.refetch(), resource.ledgerQuery.refetch()]) }
function loadMore() { if (nextBeforeSequence.value == null || ledgerLoading.value) return; beforeSequence.value = nextBeforeSequence.value }
function refreshWallet() { resetLedger() }
function openCommand(kind: Exclude<AdminCreditCommandKind, 'refund'>) { refundSource.value = null; activeCommand.value = kind; amountCredits.value = null; reason.value = ''; showErrors.value = false }
function openRefund(transaction: AdminCreditLedgerTransaction) { if (!isRefundableCharge(transaction)) return; refundSource.value = transaction; activeCommand.value = 'refund'; amountCredits.value = null; reason.value = ''; showErrors.value = false }
function clearCommand() { activeCommand.value = null; refundSource.value = null; amountCredits.value = null; reason.value = ''; showErrors.value = false }
function isRefundableCharge(transaction: AdminCreditLedgerTransaction) { return transaction.command_type === 'capture' && (transaction.charge?.remaining_refundable_credits ?? 0) > 0 }
function requestCommandConfirmation() { showErrors.value = true; if (!activeCommand.value || !validation.value.valid || amountCredits.value == null) return; const kind = activeCommand.value; const label = kind === 'grant' ? '赠送' : kind === 'deduction' ? '扣减' : '退款'; confirm.request({ title: `确认${label}Idea`, message: `将为 ${props.user.username} ${label} ${formatCredits(amountCredits.value)}。原因：${reason.value.trim()}`, confirmLabel: `确认${label}`, intent: kind === 'grant' ? 'warn' : 'danger', onConfirm: async () => { try { const response = await resource.executeCommand({ kind, userId: props.user.id, amountCredits: amountCredits.value, reason: reason.value, chargeTransactionId: refundSource.value?.transaction_id, remainingRefundableCredits: refundSource.value?.charge?.remaining_refundable_credits }); feedback.commandSucceeded(label, response.replayed); clearCommand(); resetLedger(); emit('refresh-users') } catch (error) { feedback.commandFailed(error, label) } } }) }
function getRestrictionLabel(type: string) { return type === 'ledger_integrity' ? '账本完整性' : type }
</script>
