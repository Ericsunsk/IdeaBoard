<template>
  <div class="grid gap-6" aria-label="点子账户">
    <header class="flex flex-wrap items-end justify-between gap-3">
      <div>
        <p class="m-0 text-[12px] font-semibold tracking-[0.08em] text-ib-text-subtle uppercase">
          Credits Wallet
        </p>
        <h2 class="mt-1 mb-0 text-[26px] leading-tight font-bold text-ib-text">我的点子</h2>
      </div>
      <Button
        type="button"
        text
        :loading="refreshing"
        class="!h-9 !rounded-[var(--radius-ib-control)] !border !border-ib-border-soft !bg-ib-surface !px-3 !text-sm !font-semibold !text-ib-text-muted !shadow-none hover:!border-ib-border-hover hover:!text-ib-text focus-visible:!outline-none focus-visible:!shadow-[var(--shadow-ib-focus)]"
        aria-label="刷新点子余额和流水"
        @click="handleRefresh"
      >
        刷新
      </Button>
    </header>

    <section
      v-if="walletPending"
      class="min-h-44 animate-pulse rounded-[var(--radius-ib-panel)] border border-ib-border-soft bg-ib-surface p-5"
      aria-label="正在加载点子余额"
    />
    <section
      v-else-if="walletError"
      class="rounded-[var(--radius-ib-panel)] border border-ib-danger/25 bg-ib-surface p-5"
      role="alert"
    >
      <p class="m-0 font-semibold text-ib-danger">点子余额加载失败</p>
      <p class="mt-1 mb-0 text-sm text-ib-text-muted">{{ errorMessage(walletError) }}</p>
      <Button
        type="button"
        text
        class="!mt-3 !px-0 !font-semibold !text-ib-creative-accent-pressed"
        @click="handleRefresh"
      >
        重新加载
      </Button>
    </section>
    <section
      v-else-if="wallet"
      class="overflow-hidden rounded-[var(--radius-ib-panel)] border border-ib-border-soft bg-ib-surface shadow-[var(--shadow-ib-control)]"
      aria-label="点子余额概览"
    >
      <div class="grid gap-5 p-5 sm:grid-cols-[minmax(0,1fr)_180px] sm:items-end">
        <div>
          <div class="flex items-center gap-2">
            <span class="text-sm font-semibold text-ib-text-muted">可用点子</span>
            <Tag
              :value="wallet.status === 'active' ? '正常' : '已冻结'"
              :severity="wallet.status === 'active' ? 'success' : 'danger'"
            />
          </div>
          <p
            class="mt-2 mb-0 whitespace-nowrap font-mono text-[clamp(1rem,5vw,2.625rem)] leading-none font-bold tracking-[-0.04em] text-ib-text"
          >
            {{ formatCredits(wallet.available_credits) }}
          </p>
        </div>
        <div class="rounded-[var(--radius-ib-control)] bg-ib-surface-muted px-4 py-3">
          <p class="m-0 text-xs font-semibold text-ib-text-subtle">任务冻结中</p>
          <p class="mt-1 mb-0 font-mono text-xl font-bold text-ib-text-muted">
            {{ formatCredits(wallet.reserved_credits) }}
          </p>
        </div>
      </div>
      <div class="h-1.5 bg-ib-surface-muted" aria-hidden="true">
        <div
          class="h-full bg-ib-creative-accent transition-[width] duration-[var(--duration-ib-standard)]"
          :style="{ width: `${reservedRatio}%` }"
        />
      </div>
    </section>

    <section class="grid gap-3" aria-label="点子流水">
      <div>
        <h3 class="m-0 text-lg font-bold text-ib-text">点子流水</h3>
        <p class="mt-1 mb-0 text-sm text-ib-text-muted">
          每一次赠送、冻结、消费与退款都会保留记录。
        </p>
      </div>

      <div
        v-if="ledgerPending"
        class="min-h-32 animate-pulse rounded-[var(--radius-ib-panel)] border border-ib-border-soft bg-ib-surface"
        aria-label="正在加载点子流水"
      />
      <div
        v-else-if="ledgerError"
        class="rounded-[var(--radius-ib-panel)] border border-ib-danger/25 bg-ib-surface p-4 text-sm text-ib-danger"
        role="alert"
      >
        {{ errorMessage(ledgerError) }}
      </div>
      <div
        v-else-if="transactions.length === 0"
        class="rounded-[var(--radius-ib-panel)] border border-dashed border-ib-border bg-ib-surface p-7 text-center"
      >
        <p class="m-0 font-semibold text-ib-text">还没有点子流水</p>
        <p class="mt-1 mb-0 text-sm text-ib-text-muted">
          完成 AI 图像任务后，相关记录会显示在这里。
        </p>
      </div>
      <ol v-else class="m-0 grid list-none gap-2 p-0">
        <li
          v-for="transaction in transactions"
          :key="transaction.transaction_id"
          class="grid gap-3 rounded-[var(--radius-ib-control)] border border-ib-border-soft bg-ib-surface px-4 py-3 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-center"
        >
          <div class="min-w-0">
            <div class="flex flex-wrap items-center gap-2">
              <span class="font-semibold text-ib-text">{{
                commandLabel(transaction.command_type)
              }}</span>
              <span class="text-xs text-ib-text-subtle">{{
                formatDate(transaction.created_at)
              }}</span>
            </div>
            <p class="mt-1 mb-0 truncate text-sm text-ib-text-muted" :title="transaction.reason">
              {{ transaction.reason || '系统点子操作' }}
            </p>
            <p class="mt-1 mb-0 text-xs text-ib-text-subtle">
              余额 {{ formatCredits(transaction.available_credits_after) }} · 冻结
              {{ formatCredits(transaction.reserved_credits_after) }}
            </p>
          </div>
          <p class="m-0 font-mono text-lg font-bold" :class="amountTone(transaction.postings)">
            {{ amountLabel(transaction.amount_credits, transaction.postings) }}
          </p>
        </li>
      </ol>

      <Button
        v-if="hasNextPage"
        type="button"
        text
        :loading="isFetchingNextPage"
        class="!h-10 !rounded-[var(--radius-ib-control)] !border !border-ib-border-soft !bg-ib-surface !font-semibold !text-ib-text-muted !shadow-none hover:!border-ib-border-hover hover:!text-ib-text"
        @click="fetchNextPage()"
      >
        加载更多
      </Button>
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import Button from 'primevue/button'
import Tag from 'primevue/tag'
import type {
  UserCreditLedgerCommandType,
  UserCreditPosting,
} from '../userCreditsResource'
import { useUserCredits } from '../useUserCredits'

const props = defineProps<{ active: boolean }>()
const refreshing = ref(false)
const credits = useUserCredits(() => props.active)
const {
  wallet,
  transactions,
  walletPending,
  walletError,
  ledgerPending,
  ledgerError,
  hasNextPage,
  isFetchingNextPage,
  fetchNextPage,
  refresh,
} = credits

const reservedRatio = computed(() => {
  if (!wallet.value) return 0
  const total = wallet.value.available_credits + wallet.value.reserved_credits
  return total > 0 ? Math.min(100, Math.round((wallet.value.reserved_credits / total) * 100)) : 0
})

async function handleRefresh() {
  refreshing.value = true
  try {
    await refresh()
  } finally {
    refreshing.value = false
  }
}

function formatCredits(value: number) {
  return new Intl.NumberFormat('zh-CN').format(value)
}

function formatDate(value: string) {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '时间未知'

  return new Intl.DateTimeFormat('zh-CN', {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  }).format(date)
}

const commandLabels: Record<UserCreditLedgerCommandType, string> = {
  grant: '获得点子',
  deduction: '管理员扣减',
  hold: '任务冻结',
  capture: 'AI 图像消费',
  release: '任务解冻',
  refund: '点子退款',
  correction: '管理员调整',
}

function commandLabel(command: UserCreditLedgerCommandType) {
  return commandLabels[command]
}

function userWalletDelta(postings: UserCreditPosting[]) {
  return postings.reduce((delta, posting) => {
    if (posting.account_role !== 'user_available' && posting.account_role !== 'user_reserved') {
      return delta
    }
    return delta + posting.amount_credits
  }, 0)
}

function amountLabel(amount: number, postings: UserCreditPosting[]) {
  const delta = userWalletDelta(postings)
  if (delta === 0) return formatCredits(amount)
  return `${delta > 0 ? '+' : '-'}${formatCredits(Math.abs(delta))}`
}

function amountTone(postings: UserCreditPosting[]) {
  const delta = userWalletDelta(postings)
  if (delta === 0) return 'text-ib-text-muted'
  return delta > 0 ? 'text-ib-success' : 'text-ib-danger'
}

function errorMessage(error: unknown) {
  return error instanceof Error ? error.message : '请稍后重试'
}
</script>
