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
            :class="ibFormInputClass"
            :placeholder="t('account.rewards.codePlaceholder')"
          />
          <Button
            type="submit"
            :is-disabled="!canSubmit"
            :is-loading="pending"
            :class="[accountPrimaryActionButtonClass, 'h-10']"
            >{{ t('account.rewards.redeem') }}</Button
          >
        </div>
        <p class="m-0 text-admin-meta text-muted-foreground" role="note">
          {{ t('account.rewards.beneficiaryNote') }}
        </p>
      </form>
    </section>

    <section class="grid min-w-0 gap-3" aria-labelledby="reward-history-heading">
      <h3 id="reward-history-heading" class="m-0 text-admin-heading font-bold text-foreground">
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
          <AccountEmptyRetry
            v-if="initialError"
            :title="t('account.rewards.emptyTitle')"
            :description="t('account.rewards.emptyDescription')"
            :retry-label="t('account.reload')"
            @retry="retryInitial()"
          />
          <div v-else :class="accountEmptyStateClass" role="status">
            <span :class="accountEmptyTitleClass">{{ t('account.rewards.emptyTitle') }}</span>
            <span :class="accountEmptyDescriptionClass">{{
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
import { useFailureToast } from '../../../../core/composables/useFailureToast'
import { userFacingErrorCopy } from '../../../../core/errors/errorCatalog'
import { ACCOUNT_GUIDANCE_COPY_KEYS } from '../../session/workspaceBilling'
import { ibFormInputClass } from '../../../../theme/formControls'
import { formatIdeaAmount } from '../../../../core/i18n/userIdeaCurrency'
import { useProfileFeedback } from '../../session/profile/useProfileFeedback'
import { formatAccountShanghaiDateTime } from '../../session/formatters'
import { useRewardRedemption } from '../../session/rewards/useRewardRedemption'
import { useRewardHistory } from '../../session/rewards/useRewardHistory'
import type { RewardRedemptionItem } from '../../session/rewards/resource'
import AccountPane from '../shared/AccountPane.vue'
import AccountEmptyRetry from '../shared/AccountEmptyRetry.vue'
import {
  accountCardPanelClass,
  accountDataTableFrameClass,
  accountEmptyDescriptionClass,
  accountEmptyStateClass,
  accountEmptyTitleClass,
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

useFailureToast(initialError, (error) => errorMessage(error))

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
  try {
    const result = await redeem(normalizedCode.value)
    code.value = ''
    profileFeedback.rewardRedemptionSucceeded(result.credited_amount, result.replayed)
  } catch (error) {
    profileFeedback.rewardRedemptionFailed(error)
  }
}
</script>
