<template>
  <AccountPane :title="t('account.tabs.usage')">
    <template v-if="isInitialPending">
      <section :class="summaryPanelClass" :aria-label="t('account.usage.loading')">
        <div :class="summaryGridClass">
          <article
            v-for="metric in summaryCellDefinitions"
            :key="metric.label"
            :class="[summaryCellClass, metric.cellClass]"
            aria-hidden="true"
          >
            <div class="h-9 w-2/3 rounded-ib-control bg-ib-surface-muted" />
            <div class="mt-3 h-5 w-1/2 rounded-ib-control bg-ib-surface-muted" />
          </article>
        </div>
      </section>
    </template>

    <section
      v-else-if="!usage && error"
      class="rounded-ib-panel border border-ib-danger/25 bg-ib-surface p-5"
      role="alert"
    >
      <p class="m-0 font-semibold text-ib-danger">{{ t('account.errors.usageLoadFailed') }}</p>
      <p class="mt-1 mb-0 text-sm text-ib-text-muted">{{ errorMessage(error) }}</p>
      <Button
        type="button"
        variant="ghost"
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
              class="m-0 break-all whitespace-normal font-mono text-ib-summary-value leading-tight font-bold text-ib-text sm:text-[30px] lg:text-[34px]"
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
            class="mt-3 mb-0 text-ib-meta font-semibold text-ib-text-subtle"
            aria-live="polite"
          >
            {{ t('account.usage.loadingRangeTrend', { range: selectedRangeText }) }}
          </p>
          <div
            v-if="error"
            class="mt-3 flex min-w-0 flex-col items-start justify-between gap-3 rounded-ib-control border border-ib-danger/25 bg-ib-surface-muted px-3 py-2 sm:flex-row sm:items-center"
            role="alert"
          >
            <p class="m-0 min-w-0 text-sm text-ib-danger">{{ errorMessage(error) }}</p>
            <Button
              type="button"
              variant="ghost"
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
          class="h-full rounded-ib-control bg-ib-surface-muted"
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
  </AccountPane>
</template>

<script setup lang="ts">
import { computed, markRaw, ref, useId } from 'vue'
import { useI18n } from 'vue-i18n'
import { Button } from '@/components/ui/button'
import RangedLineChartCard from '../../../../shared/components/RangedLineChartCard.vue'
import { formatIdeaAmount } from '../../../../shared/i18n/userIdeaCurrency'
import { formatLocaleNumber } from '../../../../shared/i18n/formatters'
import { userFacingErrorCopy } from '../../../../shared/errors/errorCatalog'
import { ACCOUNT_GUIDANCE_COPY_KEYS } from '../../infrastructure'
import { useUsage } from '../../application/usage/useUsage'
import { DEFAULT_USAGE_RANGE, type UsageRange } from '../../infrastructure'
import { buildUsageTrendChart, createUsageChartPlugins, resolveUsageChartTheme } from '../../domain'
import AccountPane from '../common/AccountPane.vue'

const props = defineProps<{ active: boolean }>()
const { t, locale } = useI18n()
const selectedRange = ref<UsageRange>(DEFAULT_USAGE_RANGE)
const { usage, displayRange, isInitialPending, isSwitching, error, retry } = useUsage(
  selectedRange,
  () => props.active,
)
const summaryPanelClass =
  'overflow-hidden rounded-ib-panel border border-ib-border-soft bg-ib-surface shadow-ib-control'
const summaryGridClass = 'grid grid-cols-2 sm:grid-cols-5'
const summaryCellClass = 'min-h-[100px] min-w-0 p-4 sm:min-h-[108px] sm:p-5'
const trendChartTheme = resolveUsageChartTheme()
const trendChartPlugins = markRaw(createUsageChartPlugins(trendChartTheme))
const trendDetailsCaptionId = useId()

const rangeOptions = computed(
  () =>
    [
      { key: '7d', label: t('account.usage.rangeWeek'), days: 7 },
      { key: '30d', label: t('account.usage.rangeMonth'), days: 30 },
      { key: '1y', label: t('account.usage.rangeYear'), days: 365 },
    ] satisfies Array<{ key: UsageRange; label: string; days: number }>,
)

const summaryCellDefinitions = computed(
  () =>
    [
      {
        label: t('account.usage.metricTotalUsed'),
        cellClass: 'border-r border-b border-ib-border-soft sm:border-b-0',
      },
      {
        label: t('account.usage.metricPeakDaily'),
        cellClass: 'border-b border-ib-border-soft sm:border-r sm:border-b-0',
      },
      {
        label: t('account.usage.metricAvgDaily'),
        cellClass: 'border-r border-b border-ib-border-soft sm:border-b-0',
      },
      {
        label: t('account.usage.metricTasks'),
        cellClass: 'border-b border-ib-border-soft sm:border-r sm:border-b-0',
      },
      {
        label: t('account.usage.metricActiveDays'),
        cellClass: 'col-span-2 sm:col-span-1',
      },
    ] as const,
)

const metrics = computed(() => {
  // Track locale for reactivity
  void locale.value
  const lifetime = usage.value?.lifetime
  const values = [
    formatLocaleNumber(lifetime?.total_credits ?? 0),
    formatLocaleNumber(lifetime?.peak_daily_credits ?? 0),
    formatLocaleNumber(lifetime?.average_daily_credits ?? 0),
    formatLocaleNumber(lifetime?.task_count ?? 0),
    formatLocaleNumber(lifetime?.active_days ?? 0),
  ]
  return summaryCellDefinitions.value.map((metric, index) => ({ ...metric, value: values[index] }))
})

const trendRows = computed(() => usage.value?.trend || [])
const trendChart = computed(() => buildUsageTrendChart(trendRows.value, trendChartTheme))
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

function rangeText(value: UsageRange) {
  return t('account.usage.lastDays', { days: rangeDays(value) })
}

function rangeDays(value: UsageRange) {
  return rangeOptions.value.find((candidate) => candidate.key === value)?.days ?? 30
}

function errorMessage(value: unknown) {
  return userFacingErrorCopy(value, t('account.usage.genericError'), ACCOUNT_GUIDANCE_COPY_KEYS)
}
</script>
