<template>
  <div class="grid gap-6" aria-label="用量统计">
    <header>
      <p class="m-0 text-[12px] font-semibold text-ib-text-subtle">Lifetime 数据 · Asia/Shanghai</p>
      <h2 class="mt-1 mb-0 text-[26px] leading-tight font-bold text-ib-text">用量统计</h2>
    </header>

    <section
      v-if="isPending"
      class="grid min-h-35 grid-cols-5 overflow-hidden rounded-[var(--radius-ib-panel)] border border-ib-border-soft bg-ib-surface animate-pulse max-[640px]:grid-cols-2"
      aria-label="正在加载用量统计"
    >
      <span
        v-for="item in 5"
        :key="item"
        class="border-r border-ib-border-soft last:border-r-0 max-[640px]:last:col-span-2"
      />
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

    <section
      v-else
      class="overflow-hidden rounded-[var(--radius-ib-panel)] border border-ib-border-soft bg-ib-surface shadow-[var(--shadow-ib-control)]"
      aria-label="用量摘要"
    >
      <div class="grid grid-cols-5 max-[640px]:grid-cols-2">
        <article
          v-for="metric in metrics"
          :key="metric.label"
          class="min-w-0 p-5 max-[640px]:p-4"
          :class="metric.cellClass"
        >
          <p
            class="m-0 overflow-hidden text-ellipsis whitespace-nowrap font-mono text-[clamp(1.5rem,4vw,2.625rem)] leading-none font-bold text-ib-text"
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

const metrics = computed(() => {
  const lifetime = usage.value?.lifetime
  return [
    {
      label: '总消耗',
      value: formatInteger(lifetime?.total_credits ?? 0),
      cellClass: 'border-r border-ib-border-soft max-[640px]:border-b',
    },
    {
      label: '历史单日峰值',
      value: formatInteger(lifetime?.peak_daily_credits ?? 0),
      cellClass: 'border-r border-ib-border-soft max-[640px]:border-b max-[640px]:border-r-0',
    },
    {
      label: '日均消耗',
      value: formatAverage(lifetime?.average_daily_credits ?? 0),
      cellClass: 'border-r border-ib-border-soft max-[640px]:border-b max-[640px]:border-r',
    },
    {
      label: '调用次数',
      value: formatInteger(lifetime?.task_count ?? 0),
      cellClass: 'border-r border-ib-border-soft max-[640px]:border-b max-[640px]:border-r-0',
    },
    {
      label: '累计活跃天数',
      value: formatInteger(lifetime?.active_days ?? 0),
      cellClass: 'max-[640px]:col-span-2',
    },
  ]
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
