<template>
  <section
    class="flex flex-col gap-3.5 rounded-2xl border border-admin-border-soft bg-white p-4.5 shadow-2xs"
    :aria-label="t('admin.wallet.overviewAria')"
  >
    <div class="flex flex-wrap items-center justify-between gap-4">
      <!-- 核心余额与状态 -->
      <div class="flex flex-wrap items-center gap-6 sm:gap-8">
        <div class="flex flex-col gap-0.5">
          <span class="text-admin-caption font-semibold text-admin-text-3"> {{ t('admin.wallet.accountBalance') }} </span>
          <div class="flex items-baseline gap-2">
            <span
              class="text-2xl font-black tracking-tight text-admin-title-color font-mono tabular-nums"
            >
              {{ formatIdeaAmount(availableCredits) }}
            </span>
            <span
              v-if="reservedCredits && reservedCredits > 0"
              class="rounded-md bg-admin-warning-soft px-1.5 py-0.5 font-mono text-admin-caption font-semibold text-admin-warning"
            >
              {{ t('admin.wallet.reservedAmount', { amount: formatIdeaAmount(reservedCredits) }) }}
            </span>
          </div>
        </div>

        <div class="hidden h-9 w-px bg-admin-border-soft sm:block" />

        <div class="flex flex-col gap-0.5">
          <span class="text-admin-caption font-semibold text-admin-text-3"> {{ t('admin.wallet.accountStatus') }} </span>
          <div class="flex items-center gap-1.5 pt-0.5">
            <AdminStatusPill :label="accountStatus" :variant="accountStatusVariant" />
          </div>
        </div>
      </div>

      <!-- 快捷操作按钮组 -->
      <div class="flex items-center gap-2">
        <AdminButton
          :label="t('admin.wallet.grant')"
          :class="adminActionButtonClass"
          size="small"
          :disabled="commandPending || !hasWallet"
          @click="emit('openCommand', 'grant')"
        >
          <template #icon>
            <Plus class="size-4 mr-1.5" />
          </template>
        </AdminButton>
        <AdminButton
          :label="t('admin.wallet.deduct')"
          :class="adminActionButtonClass"
          size="small"
          severity="danger"
          outlined
          :disabled="commandPending || !hasWallet || availableCredits < 1"
          @click="emit('openCommand', 'deduction')"
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
        <span class="font-bold text-admin-warning">{{ t('admin.wallet.activeRestrictionsNotice') }}</span>
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
import AdminAlert from '../../../shared/components/AdminAlert.vue'
import AdminButton from '../../../shared/components/AdminButton.vue'
import AdminStatusPill from '../../../shared/components/AdminStatusPill.vue'
import { adminActionButtonClass } from '../../../shared/lib/uiClasses'
import { formatIdeaAmount } from '../../../../../shared/i18n/userIdeaCurrency'

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
