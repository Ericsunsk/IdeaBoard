<template>
  <section class="flex flex-col gap-4 rounded-2xl border border-border bg-card p-5 shadow-2xs">
    <div class="flex items-center justify-between border-b border-border pb-3">
      <div class="flex items-center gap-2">
        <span
          class="size-2 rounded-full"
          :class="
            activeTab === 'grant'
              ? 'bg-emerald-500'
              : activeTab === 'deduction'
                ? 'bg-destructive'
                : 'bg-amber-500'
          "
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
      class="flex items-center gap-2 rounded-xl border border-primary/20 bg-primary/10 px-3.5 py-2.5 text-xs text-primary font-mono"
    >
      <span>{{
        t('admin.wallet.originalChargeNotice', {
          id: refundSource.transaction_id,
          amount: formatIdeaAmount(refundSource.charge?.remaining_refundable_credits ?? 0),
        })
      }}</span>
    </div>

    <form class="flex flex-col gap-4" @submit.prevent="emit('submit')">
      <div class="grid grid-cols-1 gap-4 sm:grid-cols-12 sm:items-start">
        <div class="sm:col-span-4">
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

        <div class="sm:col-span-8">
          <AdminActionReasonField
            :model-value="reason"
            input-id="admin-user-wallet-command-reason"
            :label="
              activeTab === 'refund'
                ? t('admin.wallet.refundReasonLabel')
                : t('admin.wallet.reasonLabel')
            "
            :error="showErrors ? validation.reasonError : ''"
            :disabled="commandPending"
            :rows="1"
            @update:model-value="onReasonChange"
          />
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
import ActionButton from '../../../../../shared/components/ActionButton.vue'
import AdminFormField from '../../../shared/components/AdminFormField.vue'
import AdminNumberInput from '../../../shared/components/AdminNumberInput.vue'
import AdminActionReasonField from '../../shared/AdminActionReasonField.vue'
import { formatIdeaAmount } from '../../../../../shared/i18n/userIdeaCurrency'
import type { AdminCreditLedgerTransaction } from '../resource'
import type { CreditCommandValidation } from '../creditCommandContract'

defineProps<{
  activeTab: 'grant' | 'deduction' | 'refund'
  refundSource: AdminCreditLedgerTransaction | null
  amountCredits: number | null
  reason: string
  showErrors: boolean
  validation: CreditCommandValidation
  commandPending: boolean
}>()

const emit = defineEmits<{
  'update:amountCredits': [value: number | null]
  'update:reason': [value: string]
  'update:showErrors': [value: boolean]
  cancel: []
  submit: []
}>()

const { t } = useI18n()

function onAmountChange(value: number | null) {
  emit('update:amountCredits', value)
  emit('update:showErrors', false)
}

function onReasonChange(value: string) {
  emit('update:reason', value)
  emit('update:showErrors', false)
}
</script>
