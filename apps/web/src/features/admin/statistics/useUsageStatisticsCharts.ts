import { computed, type Ref } from 'vue'
import {
  defineChartData,
  type ChartData,
  type ChartOptions,
  type ChartTooltipItem,
  withAlpha,
} from '../../../shared/lib/chartPrimitives'
import type { AdminStatistics, GetAdminStatisticsRangeValue } from './statisticsResource'
import {
  adminChartStrokeWidth,
  adminChartTheme,
  defineAdminChartCountScale,
  defineAdminChartPointClipStyle,
  defineAdminChartOptions,
} from '../shared/lib/adminChart'
import {
  buildAdminTimeRangeBuckets,
  shouldShowAdminTimeRangeTick,
} from '../shared/lib/adminTimeRange'
import { useUsageStatisticsBreakdownCharts } from './usageStatisticsBreakdownCharts'
import {
  baseChartOptions,
  baseChartPlugins,
  baseTooltipOptions,
  colorForModel,
  defaultModelColors,
  normalizeModelName,
} from './usageStatisticsChartBase'
import { useUsageStatisticsModelHealthCharts } from './usageStatisticsModelHealthCharts'
import { useUsageStatisticsQualityTrendCharts } from './usageStatisticsQualityTrendCharts'
import { useUsageStatisticsSummaryMetrics } from './usageStatisticsSummaryMetrics'

export function useUsageStatisticsCharts(options: {
  adminStatistics: Ref<AdminStatistics | null>
  qualityStatistics?: Ref<AdminStatistics | null>
  timeRange: Ref<GetAdminStatisticsRangeValue>
  qualityTimeRange?: Ref<GetAdminStatisticsRangeValue>
}) {
  function getModelColorByName(name: string) {
    const key = normalizeModelName(name)
    if (!key) return null
    return colorForModel(key, defaultModelColors)
  }

  const trendData = computed(() => {
    const trend = options.adminStatistics.value?.trend || []
    const trendMap = new Map(trend.map((day) => [day.date, day]))
    return buildAdminTimeRangeBuckets(options.timeRange.value).map((bucket) => {
      const day = trendMap.get(bucket.date) || {
        date: bucket.date,
        label: bucket.label,
        total: 0,
        segments: [],
      }
      const segments = day.segments.map((seg, idx) => {
        const color = getModelColorByName(seg.label)
        return {
          label: seg.label,
          value: seg.value,
          color: color || defaultModelColors[idx % defaultModelColors.length],
        }
      })
      if (segments.length === 0)
        segments.push({ label: '无数据', value: 0, color: adminChartTheme.emptyText })
      return { date: day.label || bucket.label, total: day.total, segments }
    })
  })

  const trendSeriesModelOrder = computed(() => {
    const byModel = new Map<string, { color: string; total: number }>()
    trendData.value.forEach((day) => {
      day.segments.forEach((segment, idx) => {
        const existing = byModel.get(segment.label)
        if (existing) {
          existing.total += segment.value
          return
        }
        byModel.set(segment.label, {
          color:
            segment.color ||
            defaultModelColors[idx % defaultModelColors.length] ||
            adminChartTheme.mist,
          total: segment.value,
        })
      })
    })
    return Array.from(byModel.entries())
      .filter(([label, meta]) => label !== '无数据' || meta.total > 0)
      .map(([label, meta]) => ({ label, color: meta.color }))
  })

  function shouldShowTrendLabel(index: number) {
    return shouldShowAdminTimeRangeTick({
      index,
      count: trendData.value.length,
      range: options.timeRange.value,
      label: trendData.value[index]?.date,
    })
  }

  function getUsageTrendTooltipPoint(context: ChartTooltipItem | undefined) {
    if (!context) return null
    const label = context.dataset.label || ''
    const day = trendData.value[context.dataIndex]
    if (!day) return null
    const segment = day.segments.find((seg) => seg.label === label)
    const value = segment?.value || 0
    const total = Math.max(0, day.total || 0)
    return {
      value,
      total,
    }
  }

  const trendChartData = computed<ChartData>(() =>
    defineChartData({
      labels: trendData.value.map((day) => day.date),
      datasets: trendSeriesModelOrder.value.map((model) => ({
        type: 'line',
        label: model.label,
        data: trendData.value.map((day) => {
          const segment = day.segments.find((seg) => seg.label === model.label)
          return segment?.value || 0
        }),
        borderColor: model.color,
        backgroundColor: withAlpha(model.color, 0.16),
        fill: true,
        tension: 0.38,
        borderWidth: adminChartStrokeWidth,
        ...defineAdminChartPointClipStyle(),
        pointRadius: trendData.value.length <= 1 ? 3 : 0,
        pointHoverRadius: 4,
        pointBackgroundColor: model.color,
        stack: 'usage',
      })),
    }),
  )

  const trendChartOptions = computed<ChartOptions>(() =>
    defineAdminChartOptions({
      ...baseChartOptions(),
      interaction: { mode: 'index', intersect: false },
      plugins: {
        ...baseChartPlugins(),
        tooltip: {
          ...baseTooltipOptions(),
          callbacks: {
            label: (context) => {
              const point = getUsageTrendTooltipPoint(context)
              if (!point) return `${context.dataset.label || '调用'}: 0`
              return `${context.dataset.label || '调用'}: ${point.value.toLocaleString()}`
            },
            afterBody: (items) => {
              const point = getUsageTrendTooltipPoint(items[0])
              if (!point) return []
              return [`总调用: ${point.total.toLocaleString()} 次`]
            },
          },
        },
      },
      scales: {
        x: {
          stacked: true,
          ticks: {
            color: adminChartTheme.axisText,
            maxRotation: 0,
            autoSkip: false,
            callback: (_value, index) =>
              shouldShowTrendLabel(index) ? trendData.value[index]?.date || '' : '',
          },
          grid: { color: withAlpha(adminChartTheme.splitLine, 0.75) },
          border: { color: adminChartTheme.axisLine },
        },
        y: {
          ...defineAdminChartCountScale({
            values: trendData.value.map((day) => day.total),
            min: 10,
            headroom: 1.15,
            stacked: true,
          }),
        },
      },
    }),
  )

  const summaryMetrics = useUsageStatisticsSummaryMetrics(options.adminStatistics)
  const { modelRiskChartData, modelRiskChartOptions, slowModelsChartData, slowModelsChartOptions } =
    useUsageStatisticsModelHealthCharts(options.adminStatistics)

  const qualityTrendCharts = useUsageStatisticsQualityTrendCharts(options)

  const breakdownCharts = useUsageStatisticsBreakdownCharts(options.adminStatistics)

  return {
    trendChartData,
    trendChartOptions,
    ...summaryMetrics,
    modelRiskChartData,
    modelRiskChartOptions,
    ...qualityTrendCharts,
    slowModelsChartData,
    slowModelsChartOptions,
    ...breakdownCharts,
  }
}
