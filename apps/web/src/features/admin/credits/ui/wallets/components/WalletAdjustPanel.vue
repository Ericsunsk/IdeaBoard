<template>
  <section class="flex flex-col gap-4 rounded-panel border border-border bg-card p-5 shadow-2xs">
    <div class="flex items-center justify-between border-b border-border pb-3">
      <div class="flex items-center gap-2">
        <!-- Tab 圆点对齐 Pill 语义色：grant/refund 走 success（与 getCreditCommandVariant
             一致），deduction 走 destructive；refund 不再用 warning。 -->
        <span
          class="size-2 rounded-pill"
          :class="activeTab === 'deduction' ? 'bg-destructive' : 'bg-success'"
        />
        <h3 class="m-0 text-admin-title-sm font-bold text-foreground">
          {{
            activeTab === 'grant'
              ? t('admin.wallet.grantTitle')
              : activeTab === 'deduction'
                ? t('admin.wallet.deductTitle')
                : t('admin.wallet.refundTitle')
          }}
        </h3>
      </div>
      <span class="text-admin-caption text-muted-foreground">
        {{
          activeTab === 'grant'
            ? t('admin.wallet.grantHint')
            : activeTab === 'deduction'
              ? t('admin.wallet.deductHint')
              : t('admin.wallet.refundHint')
        }}
      </span>
    </div>

    <div
      v-if="activeTab === 'refund' && refundSource"
      class="flex items-center gap-2 rounded-panel border border-primary-line bg-primary-faint px-3.5 py-2.5 text-xs text-primary font-mono"
    >
      <span>{{
        t('admin.wallet.originalChargeNotice', {
          id: refundSource.transaction_id,
          amount: formatIdeaAmount(refundSource.charge?.remaining_refundable_credits ?? 0),
        })
      }}</span>
    </div>

    <form class="flex flex-col gap-4" @submit.prevent="emit('submit')">
      <div class="flex flex-col gap-4">
        <div>
          <AdminFormField
            v-slot="{ describedbyId, invalid }"
            :label="t('admin.wallet.amountLabel')"
            for="admin-user-wallet-command-amount"
            :error="showErrors ? validation.amountError : ''"
            required
          >
            <AdminNumberInput
              :model-value="amountCredits"
              input-id="admin-user-wallet-command-amount"
              :min="1"
              :use-grouping="true"
              :placeholder="t('admin.wallet.amountPlaceholder')"
              :invalid="invalid"
              :disabled="commandPending"
              :aria-describedby="describedbyId"
              @update:model-value="onAmountChange"
            />
          </AdminFormField>
        </div>
      </div>

      <div class="flex items-center justify-end gap-2.5 pt-1">
        <ActionButton
          :label="t('admin.common.cancel')"
          type="button"
          variant="outline"
          :disabled="commandPending"
          @click="emit('cancel')"
        />
        <ActionButton
          :label="
            activeTab === 'refund'
              ? t('admin.wallet.reviewRefund')
              : t('admin.wallet.reviewOperation')
          "
          type="submit"
          :variant="activeTab === 'grant' ? 'default' : 'destructive'"
          :loading="commandPending"
          :disabled="commandPending"
        />
      </div>
    </form>
  </section>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import ActionButton from '../../../../../../core/components/ActionButton.vue'
import { AdminFormField } from '../../../../shared'
import { AdminNumberInput } from '../../../../shared'
import { formatIdeaAmount } from '../../../../../../core/i18n/userIdeaCurrency'
import type { AdminCreditLedgerTransaction } from '../../../session/wallets/resource'
import type { CreditCommandValidation } from '../../../core/wallets/creditCommandContract'

defineProps<{
  activeTab: 'grant' | 'deduction' | 'refund'
  refundSource: AdminCreditLedgerTransaction | null
  amountCredits: number | null
  showErrors: boolean
  validation: CreditCommandValidation
  commandPending: boolean
}>()

const emit = defineEmits<{
  'update:amountCredits': [value: number | null]
  'update:showErrors': [value: boolean]
  cancel: []
  submit: []
}>()

const { t } = useI18n()

function onAmountChange(value: number | null) {
  emit('update:amountCredits', value)
  emit('update:showErrors', false)
}
</script>
