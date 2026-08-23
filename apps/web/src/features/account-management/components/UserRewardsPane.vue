<template>
  <AccountManagementPane title="奖励中心">
    <section :class="accountCardPanelClass" aria-labelledby="reward-code-heading">
      <form class="grid gap-3" @submit.prevent="submitCode">
        <label
          id="reward-code-heading"
          for="account-reward-code"
          class="text-[13px] font-semibold text-ib-text-muted"
        >
          输入兑换码
        </label>
        <div class="grid min-w-0 grid-cols-[minmax(0,1fr)_112px] gap-3 max-[640px]:grid-cols-1">
          <InputText
            id="account-reward-code"
            v-model="code"
            unstyled
            type="text"
            autocomplete="off"
            autocapitalize="characters"
            :spellcheck="false"
            :disabled="pending"
            :class="ibFormInputClass"
            placeholder="请输入兑换码"
          />
          <Button
            unstyled
            type="submit"
            label="兑换"
            :disabled="!canSubmit"
            :loading="pending"
            :class="accountPrimaryActionButtonClass"
            :pt="accountButtonPt"
          />
        </div>
      </form>
    </section>

    <section class="grid min-w-0 gap-3" aria-labelledby="reward-history-heading">
      <h3 id="reward-history-heading" class="m-0 text-lg font-bold text-ib-text">兑换记录</h3>
      <DataTableFrame
        :value="tableRows"
        data-key="id"
        scrollable
        :table-style="{ minWidth: '680px' }"
        table-aria-label="兑换记录表格"
        scroll-aria-label="兑换记录表格"
        :frame-class="accountDataTableFrameClass"
        empty-title="暂无兑换记录"
        empty-description="成功兑换的奖励会显示在这里"
      >
        <Column field="redeemed_at" header="兑换时间" :style="{ width: '190px' }">
          <template #body="{ data }">
            <Skeleton v-if="isSkeletonRow(data)" width="8rem" />
            <time v-else class="whitespace-nowrap font-mono text-[13px] text-ib-text-muted">
              {{ formatShanghaiDateTime(data.redeemed_at) }}
            </time>
          </template>
        </Column>

        <Column field="reward_code_display_name" header="奖励内容" :style="{ minWidth: '220px' }">
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
        </Column>

        <Column field="credited_amount" header="奖励" :style="{ width: '130px' }">
          <template #body="{ data }">
            <Skeleton v-if="isSkeletonRow(data)" width="4rem" />
            <span v-else class="whitespace-nowrap font-mono font-bold text-ib-success">
              {{ formatCreditedAmount(data.credited_amount) }}
            </span>
          </template>
        </Column>

        <Column field="masked_code_hint" header="兑换码" :style="{ width: '150px' }">
          <template #body="{ data }">
            <Skeleton v-if="isSkeletonRow(data)" width="5rem" />
            <span v-else class="whitespace-nowrap font-mono text-[13px] text-ib-text-muted">
              {{ data.masked_code_hint }}
            </span>
          </template>
        </Column>

        <template #empty>
          <div
            v-if="initialError"
            class="flex min-h-40 flex-col items-center justify-center px-6 py-10 text-center"
            role="alert"
          >
            <p class="m-0 font-semibold text-ib-danger">兑换记录加载失败</p>
            <p class="mt-1 mb-0 text-sm text-ib-text-muted">
              {{ errorMessage(initialError) }}
            </p>
            <Button
              label="重新加载"
              severity="secondary"
              size="small"
              text
              class="mt-3"
              @click="retryInitial()"
            />
          </div>
          <div
            v-else
            class="flex min-h-40 flex-col items-center justify-center px-6 py-10 text-center"
            role="status"
          >
            <span class="font-semibold text-ib-text">暂无兑换记录</span>
            <span class="mt-1 text-sm text-ib-text-subtle">成功兑换的奖励会显示在这里</span>
          </div>
        </template>

        <template #after>
          <p v-if="isInitialPending" class="sr-only" role="status" aria-label="正在加载兑换记录">
            正在加载兑换记录
          </p>

          <div
            v-if="loadMoreError"
            class="flex items-center justify-center gap-3 border-t border-ib-border-soft px-4 py-3"
            role="alert"
          >
            <span class="text-sm text-ib-danger">{{ errorMessage(loadMoreError) }}</span>
            <Button label="重试加载" severity="secondary" size="small" text @click="loadMore()" />
          </div>
          <div
            v-else-if="hasNextPage"
            class="flex justify-center border-t border-ib-border-soft px-4 py-3"
          >
            <Button
              label="加载更多"
              severity="secondary"
              size="small"
              outlined
              :loading="isFetchingNextPage"
              @click="loadMore()"
            />
          </div>
        </template>
      </DataTableFrame>
    </section>
  </AccountManagementPane>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import Button from 'primevue/button'
import Column from 'primevue/column'
import InputText from 'primevue/inputtext'
import Skeleton from 'primevue/skeleton'
import DataTableFrame from '../../../shared/components/DataTableFrame.vue'
import { ibFormInputClass } from '../../../shared/theme/formControls'
import { formatIdeaAmount } from '../../../shared/utils/ideaCurrency'
import { useProfileFeedback } from '../useProfileFeedback'
import { useUserRewardRedemption } from '../useUserRewardRedemption'
import { useUserRewardRedemptions } from '../useUserRewardRedemptions'
import type { UserRewardRedemption } from '../userRewardsResource'
import AccountManagementPane from './AccountManagementPane.vue'
import {
  accountButtonPt,
  accountCardPanelClass,
  accountDataTableFrameClass,
  accountPrimaryActionButtonClass,
} from './accountManagementUi'

const code = ref('')
const profileFeedback = useProfileFeedback()
const { pending, redeem } = useUserRewardRedemption()
const {
  redemptions,
  isInitialPending,
  initialError,
  loadMoreError,
  hasNextPage,
  isFetchingNextPage,
  retryInitial,
  loadMore,
} = useUserRewardRedemptions()
const normalizedCode = computed(() => code.value.trim())
const canSubmit = computed(() => normalizedCode.value.length > 0 && !pending.value)

type RewardSkeletonRow = {
  id: string
  isSkeleton: true
}
type RewardTableRow = UserRewardRedemption | RewardSkeletonRow

const shanghaiDateTimeFormatter = new Intl.DateTimeFormat('zh-CN', {
  timeZone: 'Asia/Shanghai',
  year: 'numeric',
  month: '2-digit',
  day: '2-digit',
  hour: '2-digit',
  minute: '2-digit',
  second: '2-digit',
  hourCycle: 'h23',
})
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
  if (Number.isNaN(date.getTime())) return '时间未知'

  const parts = Object.fromEntries(
    shanghaiDateTimeFormatter
      .formatToParts(date)
      .filter((part) => part.type !== 'literal')
      .map((part) => [part.type, part.value]),
  )
  return `${parts.year}-${parts.month}-${parts.day} ${parts.hour}:${parts.minute}:${parts.second}`
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : '网络错误，请重试'
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
