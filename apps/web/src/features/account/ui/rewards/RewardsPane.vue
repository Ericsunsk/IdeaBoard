<template>
  <AccountPane :title="t('account.tabs.rewards')">
    <section :class="accountCardPanelClass" aria-labelledby="reward-code-heading">
      <form class="grid gap-3" @submit.prevent="submitCode">
        <label
          id="reward-code-heading"
          for="account-reward-code"
          class="text-admin-body font-semibold text-muted-foreground"
        >
          {{ t('account.rewards.codeLabel') }}
        </label>
        <div class="grid min-w-0 grid-cols-1 gap-3 sm:grid-cols-[minmax(0,1fr)_112px]">
          <Input
            id="account-reward-code"
            v-model="code"
            type="text"
            autocomplete="off"
            autocapitalize="characters"
            :spellcheck="false"
            :disabled="pending"
            :class="[ibFormInputClass, redeemFailed ? ibFormInputErrorClass : '']"
            :aria-invalid="redeemFailed || undefined"
            :aria-describedby="redeemFailed ? 'account-reward-code-error' : undefined"
            :placeholder="t('account.rewards.codePlaceholder')"
            @update:model-value="redeemFailed = false"
          />
          <Button
            type="submit"
            :is-disabled="!canSubmit"
            :is-loading="pending"
            :class="[accountPrimaryActionButtonClass, 'h-10']"
            >{{ t('account.rewards.redeem') }}</Button
          >
        </div>
        <p
          v-if="redeemFailed"
          id="account-reward-code-error"
          class="m-0 border-0 bg-transparent p-0 text-xs font-semibold text-destructive"
          role="alert"
        >
          {{ redeemErrorText }}
        </p>
      </form>
    </section>

    <section class="grid min-w-0 gap-3" aria-labelledby="reward-history-heading">
      <h3 id="reward-history-heading" class="m-0 text-lg font-bold text-foreground">
        {{ t('account.rewards.historyTitle') }}
      </h3>
      <DataTableFrame
        :value="tableRows"
        data-key="id"
        scrollable
        :table-style="{ minWidth: '680px' }"
        :table-aria-label="t('account.rewards.tableAria')"
        :scroll-aria-label="t('account.rewards.tableAria')"
        :frame-class="accountDataTableFrameClass"
        :empty-title="t('account.rewards.emptyTitle')"
        :empty-description="t('account.rewards.emptyDescription')"
      >
        <DataTableColumn
          field="redeemed_at"
          :header="t('account.rewards.columnRedeemedAt')"
          :style="{ width: '190px' }"
        >
          <template #body="{ data }">
            <Skeleton v-if="isSkeletonRow(data)" width="8rem" />
            <time v-else class="whitespace-nowrap font-mono text-admin-body text-muted-foreground">
              {{ formatShanghaiDateTime(data.redeemed_at) }}
            </time>
          </template>
        </DataTableColumn>

        <DataTableColumn
          field="reward_code_display_name"
          :header="t('account.rewards.columnReward')"
          :style="{ minWidth: '220px' }"
        >
          <template #body="{ data }">
            <Skeleton v-if="isSkeletonRow(data)" width="7rem" />
            <span
              v-else
              class="block max-w-[320px] truncate font-semibold"
              :title="data.reward_code_display_name"
            >
              {{ data.reward_code_display_name }}
            </span>
          </template>
        </DataTableColumn>

        <DataTableColumn
          field="credited_amount"
          :header="t('account.rewards.columnCredited')"
          :style="{ width: '130px' }"
        >
          <template #body="{ data }">
            <Skeleton v-if="isSkeletonRow(data)" width="4rem" />
            <span v-else class="whitespace-nowrap font-mono font-bold text-success">
              {{ formatCreditedAmount(data.credited_amount) }}
            </span>
          </template>
        </DataTableColumn>

        <DataTableColumn
          field="masked_code_hint"
          :header="t('account.rewards.columnCode')"
          :style="{ width: '150px' }"
        >
          <template #body="{ data }">
            <Skeleton v-if="isSkeletonRow(data)" width="5rem" />
            <span v-else class="whitespace-nowrap font-mono text-admin-body text-muted-foreground">
              {{ data.masked_code_hint }}
            </span>
          </template>
        </DataTableColumn>

        <template #empty>
          <LoadErrorPanel
            v-if="initialError"
            class="min-h-40 justify-center"
            align="center"
            :message="t('account.rewards.loadFailed')"
            :detail="errorMessage(initialError)"
            :retry-label="t('account.reload')"
            retry-variant="ghost"
            @retry="retryInitial()"
          />
          <div
            v-else
            class="flex min-h-40 flex-col items-center justify-center px-6 py-10 text-center"
            role="status"
          >
            <span class="font-semibold text-foreground">{{ t('account.rewards.emptyTitle') }}</span>
            <span class="mt-1 text-sm text-muted-foreground">{{
              t('account.rewards.emptyDescription')
            }}</span>
          </div>
        </template>

        <template #after>
          <p
            v-if="isInitialPending"
            class="sr-only"
            role="status"
            :aria-label="t('account.rewards.loading')"
          >
            {{ t('account.rewards.loading') }}
          </p>

          <LoadErrorPanel
            v-if="loadMoreError"
            align="center"
            :message="errorMessage(loadMoreError)"
            :retry-label="t('account.rewards.retryLoad')"
            :is-retrying="isFetchingNextPage"
            retry-variant="ghost"
            @retry="loadMore()"
          />
          <div v-else-if="hasNextPage" class="flex justify-center border-t border-border px-4 py-3">
            <Button
              variant="outline"
              size="sm"
              :is-loading="isFetchingNextPage"
              @click="loadMore()"
              >{{ t('account.rewards.loadMore') }}</Button
            >
          </div>
        </template>
      </DataTableFrame>
    </section>
  </AccountPane>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { Button } from '@/components/ui/button'
import DataTableColumn from '../../../../core/components/DataTableColumn.vue'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import DataTableFrame from '../../../../core/components/DataTableFrame.vue'
import LoadErrorPanel from '../../../../core/components/LoadErrorPanel.vue'
import { userFacingErrorCopy } from '../../../../core/errors/errorCatalog'
import { ACCOUNT_GUIDANCE_COPY_KEYS } from '../../session/workspaceBilling'
import { ibFormInputClass, ibFormInputErrorClass } from '../../../../theme/formControls'
import { formatIdeaAmount } from '../../../../core/i18n/userIdeaCurrency'
import { useProfileFeedback } from '../../session/profile/useProfileFeedback'
import { formatAccountShanghaiDateTime } from '../../session/formatters'
import { useRewardRedemption } from '../../session/rewards/useRewardRedemption'
import { useRewardHistory } from '../../session/rewards/useRewardHistory'
import type { RewardRedemptionItem } from '../../session/rewards/resource'
import AccountPane from '../shared/AccountPane.vue'
import {
  accountCardPanelClass,
  accountDataTableFrameClass,
  accountPrimaryActionButtonClass,
} from '../shared/ui'

const { t } = useI18n()
const code = ref('')
const profileFeedback = useProfileFeedback()
const { pending, redeem } = useRewardRedemption()
const {
  redemptions,
  isInitialPending,
  initialError,
  loadMoreError,
  hasNextPage,
  isFetchingNextPage,
  retryInitial,
  loadMore,
} = useRewardHistory()
const normalizedCode = computed(() => code.value.trim())
const canSubmit = computed(() => normalizedCode.value.length > 0 && !pending.value)
// Submit failures surface inline next to the control (the Admin invalid +
// error-text contract), not only as a toast: the user needs to see which field
// failed without hunting a transient message.
const redeemFailed = ref(false)
const redeemErrorText = ref('')

type RewardSkeletonRow = {
  id: string
  isSkeleton: true
}
type RewardTableRow = RewardRedemptionItem | RewardSkeletonRow

const skeletonRows: RewardSkeletonRow[] = Array.from({ length: 4 }, (_, index) => ({
  id: `reward-skeleton-${index}`,
  isSkeleton: true,
}))
const tableRows = computed<RewardTableRow[]>(() =>
  isInitialPending.value ? skeletonRows : redemptions.value,
)

function isSkeletonRow(row: RewardTableRow): row is RewardSkeletonRow {
  return 'isSkeleton' in row
}

function formatCreditedAmount(value: number): string {
  return `+${formatIdeaAmount(value)}`
}

function formatShanghaiDateTime(value: string): string {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return t('account.rewards.unknownTime')

  return formatAccountShanghaiDateTime(date)
}

function errorMessage(error: unknown): string {
  return userFacingErrorCopy(error, t('account.errors.networkRetry'), ACCOUNT_GUIDANCE_COPY_KEYS)
}

async function submitCode() {
  if (!canSubmit.value) return
  redeemFailed.value = false
  try {
    const result = await redeem(normalizedCode.value)
    code.value = ''
    profileFeedback.rewardRedemptionSucceeded(result.credited_amount, result.replayed)
  } catch (error) {
    redeemFailed.value = true
    redeemErrorText.value = errorMessage(error)
    profileFeedback.rewardRedemptionFailed(error)
  }
}
</script>
