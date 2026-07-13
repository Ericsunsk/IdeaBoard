<template>
  <div class="grid min-w-0 gap-6" aria-label="用量统计">
    <header>
      <h2 class="m-0 text-[26px] leading-tight font-bold text-ib-text">用量统计</h2>
    </header>

    <template v-if="isInitialPending">
      <section :class="summaryPanelClass" aria-label="正在加载用量统计">
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
    </template>

    <section
      v-else-if="!usage && error"
      class="rounded-[var(--radius-ib-panel)] border border-ib-danger/25 bg-ib-surface p-5"
      role="alert"
    >
      <p class="m-0 font-semibold text-ib-danger">用量统计加载失败</p>
      <p class="mt-1 mb-0 text-sm text-ib-text-muted">{{ errorMessage(error) }}</p>
      <Button
        type="button"
        text
        class="!mt-3 !px-0 !font-semibold !text-ib-creative-accent-pressed"
        @click="retry()"
      >
        重新加载
      </Button>
    </section>

    <template v-else-if="usage">
      <section :class="summaryPanelClass" aria-label="用量摘要">
        <div :class="summaryGridClass">
          <article
            v-for="metric in metrics"
            :key="metric.label"
            :class="[summaryCellClass, metric.cellClass]"
          >
            <p
              class="m-0 break-all whitespace-normal font-mono text-[34px] leading-tight font-bold text-ib-text max-[900px]:text-[30px] max-[640px]:text-[28px]"
            >
              {{ metric.value }}
            </p>
            <p class="mt-3 mb-0 text-sm font-semibold text-ib-text-muted">{{ metric.label }}</p>
          </article>
        </div>
      </section>
    </template>

    <section
      v-if="isInitialPending || usage"
      :class="[trendPanelClass, { 'min-h-[334px]': isInitialPending }]"
      :aria-label="isInitialPending ? '正在加载点子消耗趋势' : '点子消耗趋势'"
    >
      <header
        class="flex min-w-0 flex-wrap items-center justify-between gap-3 max-[640px]:items-stretch max-[640px]:flex-col"
      >
        <h3 class="m-0 text-[16px] leading-6 font-bold text-ib-text">点子消耗</h3>
        <div
          role="radiogroup"
          aria-label="趋势时间范围"
          class="grid w-[180px] shrink-0 grid-cols-3 rounded-[var(--radius-ib-control)] border border-ib-border-soft bg-ib-surface-muted p-1 max-[640px]:w-full"
        >
          <button
            v-for="(option, index) in rangeOptions"
            :key="option.value"
            :ref="(element) => setRangeButtonRef(option.value, element)"
            type="button"
            role="radio"
            :aria-checked="selectedRange === option.value"
            :tabindex="selectedRange === option.value ? 0 : -1"
            :class="rangeButtonClass(option.value)"
            @click="selectRange(option.value)"
            @keydown="onRangeKeydown($event, index)"
          >
            {{ option.label }}
          </button>
        </div>
      </header>
      <template v-if="isInitialPending">
        <div class="mt-5 h-[260px] rounded-[var(--radius-ib-control)] bg-ib-surface-muted" />
      </template>
      <template v-else>
        <p
          v-if="isSwitching"
          class="mt-3 mb-0 text-[12px] font-semibold text-ib-text-subtle"
          aria-live="polite"
        >
          正在加载{{ selectedRangeText }}趋势…
        </p>
        <div
          v-if="error"
          class="mt-3 flex min-w-0 items-center justify-between gap-3 rounded-[var(--radius-ib-control)] border border-ib-danger/25 bg-ib-surface-muted px-3 py-2 max-[640px]:items-start max-[640px]:flex-col"
          role="alert"
        >
          <p class="m-0 min-w-0 text-sm text-ib-danger">{{ errorMessage(error) }}</p>
          <Button
            type="button"
            text
            class="!shrink-0 !p-0 !text-sm !font-semibold !text-ib-creative-accent-pressed"
            @click="retry()"
          >
            重试当前范围
          </Button>
        </div>
        <div class="mt-4 min-w-0 overflow-hidden">
          <Chart
            type="line"
            class="h-[260px] w-full min-w-0"
            :data="trendChart.data"
            :options="trendChart.options"
            :canvas-props="trendCanvasProps"
          />
        </div>
        <table class="sr-only">
          <caption :id="trendDetailsCaptionId">
            {{
              displayRangeText
            }}点子消耗明细
          </caption>
          <thead>
            <tr>
              <th scope="col">日期</th>
              <th scope="col">点子消耗</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="point in trendRows" :key="point.date">
              <th scope="row">{{ point.date }}</th>
              <td>{{ formatInteger(point.credits) }} 点子</td>
            </tr>
          </tbody>
        </table>
      </template>
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, ref, useId } from 'vue'
import Button from 'primevue/button'
import Chart from 'primevue/chart'
import { useUserUsage } from '../useUserUsage'
import { DEFAULT_USER_USAGE_RANGE, type UserUsageRange } from '../userUsageResource'
import { buildUserUsageTrendChart, resolveUserUsageChartTheme } from '../userUsageTrendChart'

const props = defineProps<{ active: boolean }>()
const selectedRange = ref<UserUsageRange>(DEFAULT_USER_USAGE_RANGE)
const { usage, displayRange, isInitialPending, isSwitching, error, retry } = useUserUsage(
  selectedRange,
  () => props.active,
)

const integerFormatter = new Intl.NumberFormat('zh-CN')
const summaryPanelClass =
  'overflow-hidden rounded-[var(--radius-ib-panel)] border border-ib-border-soft bg-ib-surface shadow-[var(--shadow-ib-control)]'
const summaryGridClass = 'grid grid-cols-5 max-[640px]:grid-cols-2'
const summaryCellClass = 'min-h-[108px] min-w-0 p-5 max-[640px]:min-h-[100px] max-[640px]:p-4'
const trendPanelClass =
  'min-w-0 overflow-hidden rounded-[var(--radius-ib-panel)] border border-ib-border-soft bg-ib-surface p-5 shadow-[var(--shadow-ib-control)] max-[640px]:p-4'
const trendChartTheme = resolveUserUsageChartTheme()
const trendDetailsCaptionId = useId()
const rangeButtonRefs = new Map<UserUsageRange, HTMLButtonElement>()

const rangeOptions = [
  { value: '7d', label: '周', days: 7 },
  { value: '30d', label: '月', days: 30 },
  { value: '1y', label: '年', days: 365 },
] as const satisfies ReadonlyArray<{ value: UserUsageRange; label: string; days: number }>

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

const trendRows = computed(() => usage.value?.trend || [])
const trendChart = computed(() => buildUserUsageTrendChart(trendRows.value, trendChartTheme))
const displayRangeText = computed(() => rangeText(displayRange.value ?? selectedRange.value))
const selectedRangeText = computed(() => rangeText(selectedRange.value))
const trendCanvasProps = computed(() => ({
  role: 'img',
  'aria-label': `${displayRangeText.value}点子消耗趋势`,
  'aria-describedby': trendDetailsCaptionId,
}))

function selectRange(value: UserUsageRange) {
  selectedRange.value = value
}

function setRangeButtonRef(value: UserUsageRange, element: unknown) {
  if (element instanceof HTMLButtonElement) {
    rangeButtonRefs.set(value, element)
  } else {
    rangeButtonRefs.delete(value)
  }
}

function onRangeKeydown(event: KeyboardEvent, currentIndex: number) {
  let targetIndex: number | undefined
  switch (event.key) {
    case 'ArrowRight':
    case 'ArrowDown':
      targetIndex = (currentIndex + 1) % rangeOptions.length
      break
    case 'ArrowLeft':
    case 'ArrowUp':
      targetIndex = (currentIndex - 1 + rangeOptions.length) % rangeOptions.length
      break
    case 'Home':
      targetIndex = 0
      break
    case 'End':
      targetIndex = rangeOptions.length - 1
      break
    default:
      return
  }
  event.preventDefault()
  const target = rangeOptions[targetIndex]
  selectRange(target.value)
  void nextTick(() => rangeButtonRefs.get(target.value)?.focus())
}

function rangeButtonClass(value: UserUsageRange) {
  const base =
    'min-w-0 rounded-[calc(var(--radius-ib-control)-2px)] px-3 py-1.5 text-sm font-semibold transition-colors focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ib-creative-accent'
  return selectedRange.value === value
    ? `${base} bg-ib-surface text-ib-text shadow-[var(--shadow-ib-control)]`
    : `${base} text-ib-text-muted hover:bg-ib-surface hover:text-ib-text`
}

function rangeText(value: UserUsageRange) {
  const option = rangeOptions.find((candidate) => candidate.value === value)
  return `最近 ${option?.days ?? 30} 天`
}

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
