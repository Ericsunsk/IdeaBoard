<template>
  <Dialog
    :visible="visible"
    modal
    :draggable="false"
    :header="dialogTitle"
    :style="{ width: 'min(500px, calc(100vw - 24px))' }"
    :pt="dialogPt"
    @update:visible="handleVisibleUpdate"
  >
    <div :class="adminDialogFormBodyClass">
      <Message severity="info" :closable="false">
        目标：{{ targetLabel }}。提交后还需要在二次确认中核对点子数量与原因。
      </Message>

      <div
        v-if="kind === 'refund'"
        class="rounded-[var(--radius-ib-control)] border border-admin-border-soft bg-ib-surface-muted px-3 py-2.5 text-admin-body text-admin-text-2"
      >
        <div class="font-[680] text-admin-text-1">原 Charge</div>
        <div class="mt-1 break-all font-mono text-admin-caption">{{ chargeTransactionId }}</div>
        <div class="mt-1">剩余可退：{{ formatCredits(remainingRefundableCredits ?? 0) }}</div>
      </div>

      <AdminFormField
        v-slot="{ describedbyId, invalid }"
        label="点子数量"
        for="admin-credit-command-amount"
        :error="showErrors ? validation.amountError : ''"
        required
      >
        <InputNumber
          v-model="amountCredits"
          input-id="admin-credit-command-amount"
          fluid
          :min="1"
          :max="amountMax"
          :use-grouping="true"
          :invalid="invalid"
          :aria-describedby="describedbyId"
          @update:model-value="showErrors = false"
        />
      </AdminFormField>

      <AdminActionReasonField
        v-model="reason"
        input-id="admin-credit-command-reason"
        hint="原因会进入不可变 Ledger，便于审计与追溯。"
        :error="showErrors ? validation.reasonError : ''"
        @update:model-value="showErrors = false"
      />
    </div>

    <template #footer>
      <div :class="adminDialogCompactFooterActionsClass">
        <Button label="取消" severity="secondary" outlined :disabled="loading" @click="close" />
        <Button
          :label="submitLabel"
          :severity="submitSeverity"
          :loading="loading"
          @click="submit"
        />
      </div>
    </template>
  </Dialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import InputNumber from 'primevue/inputnumber'
import Message from 'primevue/message'
import AdminFormField from '../../shared/components/AdminFormField.vue'
import AdminActionReasonField from '../AdminActionReasonField.vue'
import { createAdminDialogPt } from '../../shared/lib/adminOverlayPt'
import {
  adminDialogCompactFooterActionsClass,
  adminDialogFormBodyClass,
} from '../../shared/lib/adminUiClasses'
import { formatCredits } from '../creditsBillingFormat'
import {
  validateAdminCreditCommand,
  type AdminCreditCommandDraft,
  type AdminCreditCommandKind,
} from './creditCommandContract'

const props = withDefaults(
  defineProps<{
    visible: boolean
    kind: AdminCreditCommandKind
    userId: number
    targetLabel: string
    chargeTransactionId?: string
    remainingRefundableCredits?: number
    loading?: boolean
  }>(),
  {
    chargeTransactionId: undefined,
    remainingRefundableCredits: undefined,
    loading: false,
  },
)

const emit = defineEmits<{
  'update:visible': [visible: boolean]
  submit: [draft: AdminCreditCommandDraft]
}>()

const dialogPt = createAdminDialogPt({ rootClass: '!rounded-[var(--radius-ib-overlay)]' })
const amountCredits = ref<number | null>(null)
const reason = ref('')
const showErrors = ref(false)

const dialogTitle = computed(() => {
  if (props.kind === 'grant') return '赠送点子'
  if (props.kind === 'deduction') return '扣减点子'
  return '退还点子'
})
const submitLabel = computed(() => (props.kind === 'refund' ? '核对退款' : '下一步：确认'))
const submitSeverity = computed(() => (props.kind === 'grant' ? undefined : 'danger'))
const amountMax = computed(() =>
  props.kind === 'refund'
    ? Math.max(1, props.remainingRefundableCredits ?? 1)
    : 9_000_000_000_000_000,
)
const validation = computed(() =>
  validateAdminCreditCommand({
    kind: props.kind,
    userId: props.userId,
    amountCredits: amountCredits.value,
    reason: reason.value,
    chargeTransactionId: props.chargeTransactionId,
    remainingRefundableCredits: props.remainingRefundableCredits,
  }),
)

watch(
  () => props.visible,
  (visible) => {
    if (!visible) return
    amountCredits.value = null
    reason.value = ''
    showErrors.value = false
  },
)

function handleVisibleUpdate(visible: boolean) {
  if (!visible && !props.loading) emit('update:visible', false)
}

function close() {
  if (!props.loading) emit('update:visible', false)
}

function submit() {
  showErrors.value = true
  if (!validation.value.valid) return
  emit('submit', {
    amountCredits: amountCredits.value,
    reason: reason.value.trim(),
  })
}
</script>
