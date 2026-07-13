<template>
  <div class="grid gap-6" aria-label="用量统计">
    <header>
      <p class="m-0 text-[12px] font-semibold text-ib-text-subtle">Lifetime 数据 · Asia/Shanghai</p>
      <h2 class="mt-1 mb-0 text-[26px] leading-tight font-bold text-ib-text">用量统计</h2>
    </header>

    <section v-if="isPending" :class="summaryPanelClass" aria-label="正在加载用量统计">
      <div :class="summaryGridClass">
        <article
          v-for="metric in summaryCellDefinitions"
          :key="metric.label"
          :class="[summaryCellClass, metric.cellClass]"
          aria-hidden="true"
        >
          <div class="h-9 w-2/3 rounded-[var(--radius-ib-control)] bg-ib-surface-muted" />
          <div class="mt-3 h-5 w-1/2 rounded-[var(--radius-ib-control)] bg-ib-surface-muted" />
        </article>
      </div>
    </section>

    <section
      v-else-if="error"
      class="rounded-[var(--radius-ib-panel)] border border-ib-danger/25 bg-ib-surface p-5"
      role="alert"
    >
      <p class="m-0 font-semibold text-ib-danger">用量统计加载失败</p>
      <p class="mt-1 mb-0 text-sm text-ib-text-muted">{{ errorMessage(error) }}</p>
      <Button
        type="button"
        text
        class="!mt-3 !px-0 !font-semibold !text-ib-creative-accent-pressed"
        @click="refresh()"
      >
        重新加载
      </Button>
    </section>

    <section v-else :class="summaryPanelClass" aria-label="用量摘要">
      <div :class="summaryGridClass">
        <article
          v-for="metric in metrics"
          :key="metric.label"
          :class="[summaryCellClass, metric.cellClass]"
        >
          <p
            class="m-0 overflow-hidden text-ellipsis whitespace-nowrap font-mono text-[34px] leading-none font-bold text-ib-text max-[900px]:text-[30px] max-[640px]:text-[28px]"
          >
            {{ metric.value }}
          </p>
          <p class="mt-3 mb-0 text-sm font-semibold text-ib-text-muted">{{ metric.label }}</p>
        </article>
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import Button from 'primevue/button'
import { useUserUsage } from '../useUserUsage'

const props = defineProps<{ active: boolean }>()
const { usage, isPending, error, refresh } = useUserUsage(() => props.active)

const integerFormatter = new Intl.NumberFormat('zh-CN')
const summaryPanelClass =
  'overflow-hidden rounded-[var(--radius-ib-panel)] border border-ib-border-soft bg-ib-surface shadow-[var(--shadow-ib-control)]'
const summaryGridClass = 'grid grid-cols-5 max-[640px]:grid-cols-2'
const summaryCellClass = 'min-h-[108px] min-w-0 p-5 max-[640px]:min-h-[100px] max-[640px]:p-4'

const summaryCellDefinitions = [
  {
    label: '总消耗',
    cellClass: 'border-r border-ib-border-soft max-[640px]:border-b',
  },
  {
    label: '历史单日峰值',
    cellClass: 'border-r border-ib-border-soft max-[640px]:border-b max-[640px]:border-r-0',
  },
  {
    label: '日均消耗',
    cellClass: 'border-r border-ib-border-soft max-[640px]:border-b max-[640px]:border-r',
  },
  {
    label: '调用次数',
    cellClass: 'border-r border-ib-border-soft max-[640px]:border-b max-[640px]:border-r-0',
  },
  {
    label: '累计活跃天数',
    cellClass: 'max-[640px]:col-span-2',
  },
] as const

const metrics = computed(() => {
  const lifetime = usage.value?.lifetime
  const values = [
    formatInteger(lifetime?.total_credits ?? 0),
    formatInteger(lifetime?.peak_daily_credits ?? 0),
    formatAverage(lifetime?.average_daily_credits ?? 0),
    formatInteger(lifetime?.task_count ?? 0),
    formatInteger(lifetime?.active_days ?? 0),
  ]
  return summaryCellDefinitions.map((metric, index) => ({ ...metric, value: values[index] }))
})

function formatInteger(value: number) {
  return integerFormatter.format(value)
}

function formatAverage(value: number) {
  if (Number.isInteger(value)) return formatInteger(value)
  return new Intl.NumberFormat('zh-CN', { maximumFractionDigits: 1 }).format(value)
}

function errorMessage(value: unknown) {
  return value instanceof Error ? value.message : '请稍后重试'
}
</script>
