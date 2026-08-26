<template>
  <AccountManagementPane :title="t('account.tabs.usage')">
    <template v-if="isInitialPending">
      <section :class="summaryPanelClass" :aria-label="t('account.usage.loading')">
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
      <p class="m-0 font-semibold text-ib-danger">{{ t('account.errors.usageLoadFailed') }}</p>
      <p class="mt-1 mb-0 text-sm text-ib-text-muted">{{ errorMessage(error) }}</p>
      <Button
        type="button"
        text
        class="!mt-3 !px-0 !font-semibold !text-ib-creative-accent-pressed"
        @click="retry()"
      >
        {{ t('account.reload') }}
      </Button>
    </section>

    <template v-else-if="usage">
      <section :class="summaryPanelClass" :aria-label="t('account.usage.summaryAria')">
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
      :title="t('account.usage.chartTitle')"
      title-tag="h3"
      :ranges="rangeOptions"
      :range-aria-label="t('account.usage.rangeAria')"
      :data="trendChart.data"
      :options="trendChart.options"
      :plugins="trendChartPlugins"
      :canvas-props="trendCanvasProps"
      :show-chart="!isInitialPending"
      size="hero"
      class="min-w-0"
      role="region"
      :aria-label="
        isInitialPending
          ? t('account.usage.trendLoadingAria')
          : t('account.usage.trendAria', { range: displayRangeText })
      "
    >
      <template #before-chart>
        <template v-if="!isInitialPending">
          <p
            v-if="isSwitching"
            class="mt-3 mb-0 text-[12px] font-semibold text-ib-text-subtle"
            aria-live="polite"
          >
            {{ t('account.usage.loadingRangeTrend', { range: selectedRangeText }) }}
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
              {{ t('account.usage.retryCurrentRange') }}
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
              t('account.usage.detailsCaption', { range: displayRangeText })
            }}
          </caption>
          <thead>
            <tr>
              <th scope="col">{{ t('account.usage.columnDate') }}</th>
              <th scope="col">{{ t('account.usage.columnCreditsUsed') }}</th>
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
import { useI18n } from 'vue-i18n'
import Button from 'primevue/button'
import RangedLineChartCard from '../../../shared/components/RangedLineChartCard.vue'
import { formatIdeaAmount } from '../../../shared/i18n/userIdeaCurrency'
import { formatLocaleNumber } from '../../../shared/i18n/formatters'
import { userFacingErrorCopy } from '../../../shared/errors/errorCatalog'
import { useUserUsage } from '../useUserUsage'
import { DEFAULT_USER_USAGE_RANGE, type UserUsageRange } from '../userUsageResource'
import {
  buildUserUsageTrendChart,
  createUserUsageChartPlugins,
  resolveUserUsageChartTheme,
} from '../userUsageTrendChart'
import AccountManagementPane from './AccountManagementPane.vue'

const props = defineProps<{ active: boolean }>()
const { t, locale } = useI18n()
const selectedRange = ref<UserUsageRange>(DEFAULT_USER_USAGE_RANGE)
const { usage, displayRange, isInitialPending, isSwitching, error, retry } = useUserUsage(
  selectedRange,
  () => props.active,
)
const summaryPanelClass =
  'overflow-hidden rounded-[var(--radius-ib-panel)] border border-ib-border-soft bg-ib-surface shadow-[var(--shadow-ib-control)]'
const summaryGridClass = 'grid grid-cols-5 max-[640px]:grid-cols-2'
const summaryCellClass = 'min-h-[108px] min-w-0 p-5 max-[640px]:min-h-[100px] max-[640px]:p-4'
const trendChartTheme = resolveUserUsageChartTheme()
const trendChartPlugins = markRaw(createUserUsageChartPlugins(trendChartTheme))
const trendDetailsCaptionId = useId()

const rangeOptions = computed(
  () =>
    [
      { key: '7d', label: t('account.usage.rangeWeek'), days: 7 },
      { key: '30d', label: t('account.usage.rangeMonth'), days: 30 },
      { key: '1y', label: t('account.usage.rangeYear'), days: 365 },
    ] satisfies Array<{ key: UserUsageRange; label: string; days: number }>,
)

const summaryCellDefinitions = computed(
  () =>
    [
      {
        label: t('account.usage.metricTotalUsed'),
        cellClass: 'border-r border-ib-border-soft max-[640px]:border-b',
      },
      {
        label: t('account.usage.metricPeakDaily'),
        cellClass: 'border-r border-ib-border-soft max-[640px]:border-b max-[640px]:border-r-0',
      },
      {
        label: t('account.usage.metricAvgDaily'),
        cellClass: 'border-r border-ib-border-soft max-[640px]:border-b max-[640px]:border-r',
      },
      {
        label: t('account.usage.metricTasks'),
        cellClass: 'border-r border-ib-border-soft max-[640px]:border-b max-[640px]:border-r-0',
      },
      {
        label: t('account.usage.metricActiveDays'),
        cellClass: 'max-[640px]:col-span-2',
      },
    ] as const,
)

const metrics = computed(() => {
  // Track locale for reactivity
  void locale.value
  const lifetime = usage.value?.lifetime
  const values = [
    formatNumber(lifetime?.total_credits ?? 0),
    formatNumber(lifetime?.peak_daily_credits ?? 0),
    formatNumber(lifetime?.average_daily_credits ?? 0),
    formatNumber(lifetime?.task_count ?? 0),
    formatNumber(lifetime?.active_days ?? 0),
  ]
  return summaryCellDefinitions.value.map((metric, index) => ({ ...metric, value: values[index] }))
})

const trendRows = computed(() => usage.value?.trend || [])
const trendChart = computed(() => buildUserUsageTrendChart(trendRows.value, trendChartTheme))
const displayRangeText = computed(() => rangeText(displayRange.value ?? selectedRange.value))
const selectedRangeText = computed(() => rangeText(selectedRange.value))
const trendCanvasAriaLabel = computed(() =>
  t('account.usage.trendAria', { range: displayRangeText.value }),
)
const trendCanvasProps = computed(() => ({
  role: 'img',
  'aria-label': trendCanvasAriaLabel.value,
  'aria-describedby': trendDetailsCaptionId,
}))

function rangeText(value: UserUsageRange) {
  return t('account.usage.lastDays', { days: rangeDays(value) })
}

function rangeDays(value: UserUsageRange) {
  return rangeOptions.value.find((candidate) => candidate.key === value)?.days ?? 30
}

function formatNumber(value: number) {
  return formatLocaleNumber(value)
}

function errorMessage(value: unknown) {
  return userFacingErrorCopy(value, t('account.usage.genericError'))
}
</script>
