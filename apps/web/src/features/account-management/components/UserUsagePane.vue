<template>
  <AccountManagementPane title="用量统计">
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

    <RangedLineChartCard
      v-if="isInitialPending || usage"
      v-model="selectedRange"
      title="Idea消耗"
      title-tag="h3"
      :ranges="rangeOptions"
      range-aria-label="趋势时间范围"
      :data="trendChart.data"
      :options="trendChart.options"
      :plugins="trendChartPlugins"
      :canvas-props="trendCanvasProps"
      :show-chart="!isInitialPending"
      size="hero"
      class="min-w-0"
      role="region"
      :aria-label="isInitialPending ? '正在加载Idea消耗趋势' : 'Idea消耗趋势'"
    >
      <template #before-chart>
        <template v-if="!isInitialPending">
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
        </template>
      </template>

      <template #chart-fallback>
        <div
          data-testid="usage-trend-skeleton"
          class="h-full rounded-[var(--radius-ib-control)] bg-ib-surface-muted"
          aria-hidden="true"
        />
      </template>

      <template #after-chart>
        <table v-if="!isInitialPending" class="sr-only">
          <caption :id="trendDetailsCaptionId">
            {{
              displayRangeText
            }}Idea消耗明细
          </caption>
          <thead>
            <tr>
              <th scope="col">日期</th>
              <th scope="col">Idea消耗</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="point in trendRows" :key="point.date">
              <th scope="row">{{ point.date }}</th>
              <td>{{ formatIdeaAmount(point.credits) }}</td>
            </tr>
          </tbody>
        </table>
      </template>
    </RangedLineChartCard>
  </AccountManagementPane>
</template>

<script setup lang="ts">
import { computed, markRaw, ref, useId } from 'vue'
import Button from 'primevue/button'
import RangedLineChartCard from '../../../shared/components/RangedLineChartCard.vue'
import { formatIdeaAmount } from '../../../shared/utils/ideaCurrency'
import { useUserUsage } from '../useUserUsage'
import { DEFAULT_USER_USAGE_RANGE, type UserUsageRange } from '../userUsageResource'
import {
  buildUserUsageTrendChart,
  createUserUsageChartPlugins,
  resolveUserUsageChartTheme,
} from '../userUsageTrendChart'
import AccountManagementPane from './AccountManagementPane.vue'

const props = defineProps<{ active: boolean }>()
const selectedRange = ref<UserUsageRange>(DEFAULT_USER_USAGE_RANGE)
const { usage, displayRange, isInitialPending, isSwitching, error, retry } = useUserUsage(
  selectedRange,
  () => props.active,
)

const numberFormatter = new Intl.NumberFormat('zh-CN')
const summaryPanelClass =
  'overflow-hidden rounded-[var(--radius-ib-panel)] border border-ib-border-soft bg-ib-surface shadow-[var(--shadow-ib-control)]'
const summaryGridClass = 'grid grid-cols-5 max-[640px]:grid-cols-2'
const summaryCellClass = 'min-h-[108px] min-w-0 p-5 max-[640px]:min-h-[100px] max-[640px]:p-4'
const trendChartTheme = resolveUserUsageChartTheme()
const trendChartPlugins = markRaw(createUserUsageChartPlugins(trendChartTheme))
const trendDetailsCaptionId = useId()

const rangeOptions = [
  { key: '7d', label: '周', days: 7 },
  { key: '30d', label: '月', days: 30 },
  { key: '1y', label: '年', days: 365 },
] satisfies Array<{ key: UserUsageRange; label: string; days: number }>

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
    formatNumber(lifetime?.total_credits ?? 0),
    formatNumber(lifetime?.peak_daily_credits ?? 0),
    formatNumber(lifetime?.average_daily_credits ?? 0),
    formatNumber(lifetime?.task_count ?? 0),
    formatNumber(lifetime?.active_days ?? 0),
  ]
  return summaryCellDefinitions.map((metric, index) => ({ ...metric, value: values[index] }))
})

const trendRows = computed(() => usage.value?.trend || [])
const trendChart = computed(() => buildUserUsageTrendChart(trendRows.value, trendChartTheme))
const displayRangeText = computed(() => rangeText(displayRange.value ?? selectedRange.value))
const selectedRangeText = computed(() => rangeText(selectedRange.value))
const trendCanvasProps = computed(() => ({
  role: 'img',
  'aria-label': `${displayRangeText.value}Idea消耗趋势`,
  'aria-describedby': trendDetailsCaptionId,
}))

function rangeText(value: UserUsageRange) {
  return `最近 ${rangeDays(value)} 天`
}

function rangeDays(value: UserUsageRange) {
  return rangeOptions.find((candidate) => candidate.key === value)?.days ?? 30
}

function formatNumber(value: number) {
  return numberFormatter.format(value)
}

function errorMessage(value: unknown) {
  return value instanceof Error ? value.message : '请稍后重试'
}
</script>
