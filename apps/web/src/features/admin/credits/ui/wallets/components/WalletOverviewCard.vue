<template>
  <section
    class="flex flex-col gap-3.5 rounded-panel border border-border bg-card p-4.5 shadow-2xs"
    :aria-label="t('admin.wallet.overviewAria')"
  >
    <div class="flex flex-wrap items-center justify-between gap-4">
      <!-- 核心余额与状态 -->
      <div class="flex flex-wrap items-center gap-6 sm:gap-8">
        <div class="flex flex-col gap-0.5">
          <span class="text-admin-caption font-semibold text-muted-foreground">
            {{ t('admin.wallet.accountBalance') }}
          </span>
          <div class="flex items-baseline gap-2">
            <span class="text-2xl font-black tracking-tight text-foreground font-mono tabular-nums">
              {{ formatIdeaAmount(availableCredits) }}
            </span>
            <span
              v-if="reservedCredits && reservedCredits > 0"
              class="rounded-control bg-warning-soft px-1.5 py-0.5 font-mono text-admin-caption font-semibold text-warning"
            >
              {{ t('admin.wallet.reservedAmount', { amount: formatIdeaAmount(reservedCredits) }) }}
            </span>
          </div>
        </div>

        <div class="hidden h-9 w-px bg-border sm:block" />

        <div class="flex flex-col gap-0.5">
          <span class="text-admin-caption font-semibold text-muted-foreground">
            {{ t('admin.wallet.accountStatus') }}
          </span>
          <div class="flex items-center gap-1.5 pt-0.5">
            <AdminStatusPill :label="accountStatus" :variant="accountStatusVariant" />
          </div>
        </div>
      </div>

      <!-- 快捷操作按钮组 -->
      <div class="flex items-center gap-2">
        <ActionButton
          :label="t('admin.wallet.grant')"
          :disabled="commandPending || !hasWallet"
          @click="emit('openCommand', 'grant')"
        >
          <template #icon>
            <Plus class="size-4 mr-1.5" />
          </template>
        </ActionButton>
        <ActionButton
          :label="t('admin.wallet.deduct')"
          severity="danger"
          outlined
          :disabled="commandPending || !hasWallet || availableCredits < 1"
          @click="emit('openCommand', 'deduction')"
        >
          <template #icon>
            <Minus class="size-4 mr-1.5" />
          </template>
        </ActionButton>
      </div>
    </div>

    <!-- 限制与冻结状态提示条 -->
    <div
      v-if="activeRestrictions.length"
      class="flex items-start gap-2 rounded-panel border border-warning/30 bg-warning-soft p-3 text-xs"
    >
      <WarningTriangle class="size-4 shrink-0 text-warning mt-0.5" />
      <div class="flex flex-col gap-1">
        <span class="font-bold text-warning">{{ t('admin.wallet.activeRestrictionsNotice') }}</span>
        <ul class="space-y-0.5 pl-4 text-muted-foreground list-disc">
          <li v-for="restriction in activeRestrictions" :key="restriction.restriction_id">
            <span class="font-semibold text-foreground"
              >{{ getRestrictionLabel(restriction.type) }}：</span
            >
            {{ restriction.reason }}
          </li>
        </ul>
      </div>
    </div>
    <AdminAlert v-else-if="accountStatus === t('admin.wallet.statusFrozen')" severity="warn">
      {{
        isUserDisabledOrClosing
          ? t('admin.wallet.statusFrozenDisabled')
          : t('admin.wallet.statusFrozenStandard')
      }}
    </AdminAlert>
  </section>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { Minus, Plus, WarningTriangle } from '@iconoir/vue'
import { AdminAlert } from '../../../../shared'
import ActionButton from '../../../../../../core/components/ActionButton.vue'
import { AdminStatusPill } from '../../../../shared'
import { formatIdeaAmount } from '../../../../../../core/i18n/userIdeaCurrency'

type WalletRestriction = {
  restriction_id: string
  type: string
  reason: string
}

defineProps<{
  availableCredits: number
  reservedCredits?: number | undefined
  accountStatus: string
  accountStatusVariant: 'success' | 'warning' | 'danger'
  activeRestrictions: readonly WalletRestriction[]
  isUserDisabledOrClosing: boolean
  commandPending: boolean
  hasWallet: boolean
}>()

const emit = defineEmits<{
  openCommand: [kind: 'grant' | 'deduction']
}>()

const { t } = useI18n()

function getRestrictionLabel(type: string) {
  return type === 'ledger_integrity' ? t('admin.wallet.restrictionLedgerIntegrity') : type
}
</script>
