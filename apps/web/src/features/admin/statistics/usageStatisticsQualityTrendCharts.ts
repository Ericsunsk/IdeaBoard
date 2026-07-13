import { computed, type Ref } from 'vue'
import {
  defineChartData,
  getParsedY,
  type ChartData,
  type ChartOptions,
  withAlpha,
} from '../../../shared/lib/chartPrimitives'
import type { AdminStatistics, GetAdminStatisticsRangeValue } from './statisticsResource'
import {
  adminChartStrokeWidth,
  adminChartTheme,
  defineAdminChartPercentScale,
  defineAdminChartPointClipStyle,
  defineAdminChartOptions,
} from '../shared/lib/adminChart'
import {
  buildAdminTimeRangeBuckets,
  shouldShowAdminTimeRangeTick,
} from '../shared/lib/adminTimeRange'
import {
  baseChartOptions,
  baseChartPlugins,
  baseTooltipOptions,
  normalizePercent,
  toNonNegativeNumber,
} from './usageStatisticsChartBase'

export function useUsageStatisticsQualityTrendCharts(options: {
  adminStatistics: Ref<AdminStatistics | null>
  qualityStatistics?: Ref<AdminStatistics | null>
  timeRange: Ref<GetAdminStatisticsRangeValue>
  qualityTimeRange?: Ref<GetAdminStatisticsRangeValue>
}) {
  const qualityTrendRange = computed(
    () => options.qualityTimeRange?.value ?? options.timeRange.value,
  )
  const qualityTrendSource = computed(() => {
    if (options.qualityStatistics?.value) return options.qualityStatistics.value
    if (qualityTrendRange.value === options.timeRange.value) return options.adminStatistics.value
    return null
  })
  const qualityTrendRows = computed(() => {
    const trend = qualityTrendSource.value?.quality_trend || []
    const trendMap = new Map(trend.map((item) => [item.date, item]))

    return buildAdminTimeRangeBuckets(qualityTrendRange.value).map((bucket) => {
      const item = trendMap.get(bucket.date)
      const total = toNonNegativeNumber(item?.total)
      const success = toNonNegativeNumber(item?.success)
      const fail = toNonNegativeNumber(item?.fail)

      return {
        label: item?.label || bucket.label,
        total,
        success,
        fail,
        successRate: normalizePercent(item?.success_rate),
        failureRate: total > 0 ? normalizePercent((fail / total) * 100) : 0,
      }
    })
  })

  const qualityTrendChartData = computed<ChartData>(() =>
    defineChartData({
      labels: qualityTrendRows.value.map((item) => item.label),
      datasets: [
        {
          type: 'line',
          label: '成功率',
          data: qualityTrendRows.value.map((item) => item.successRate),
          borderColor: adminChartTheme.success,
          backgroundColor: withAlpha(adminChartTheme.success, 0.12),
          borderWidth: adminChartStrokeWidth,
          ...defineAdminChartPointClipStyle(),
          fill: true,
          tension: 0.36,
          pointRadius: 2.5,
          pointHoverRadius: 5,
          yAxisID: 'y',
        },
        {
          type: 'line',
          label: '失败率',
          data: qualityTrendRows.value.map((item) => item.failureRate),
          borderColor: adminChartTheme.danger,
          backgroundColor: withAlpha(adminChartTheme.danger, 0.08),
          borderWidth: adminChartStrokeWidth,
          ...defineAdminChartPointClipStyle(),
          fill: true,
          tension: 0.36,
          pointRadius: 2.5,
          pointHoverRadius: 5,
          yAxisID: 'y',
        },
      ],
    }),
  )

  const qualityTrendChartOptions = computed<ChartOptions>(() =>
    defineAdminChartOptions({
      ...baseChartOptions(),
      interaction: { mode: 'index', intersect: false },
      plugins: {
        ...baseChartPlugins(),
        tooltip: {
          ...baseTooltipOptions(),
          callbacks: {
            label: (context) => {
              const label = context.dataset.label || ''
              const value = getParsedY(context)
              return `${label}: ${value.toFixed(1)}%`
            },
            afterBody: (items) => {
              const index = items[0]?.dataIndex ?? -1
              const row = qualityTrendRows.value[index]
              if (!row) return []
              return [
                `总调用: ${row.total.toLocaleString()}`,
                `成功: ${row.success.toLocaleString()}`,
                `失败: ${row.fail.toLocaleString()}`,
              ]
            },
          },
        },
      },
      scales: {
        x: {
          ticks: {
            color: adminChartTheme.axisText,
            maxRotation: 0,
            autoSkip: false,
            callback: (_value, index) =>
              shouldShowAdminTimeRangeTick({
                index,
                count: qualityTrendRows.value.length,
                range: qualityTrendRange.value,
                label: qualityTrendRows.value[index]?.label,
              })
                ? qualityTrendRows.value[index]?.label || ''
                : '',
          },
          grid: { display: false },
          border: { color: adminChartTheme.axisLine },
        },
        y: {
          ...defineAdminChartPercentScale(),
        },
      },
    }),
  )

  return {
    qualityTrendChartData,
    qualityTrendChartOptions,
  }
}
