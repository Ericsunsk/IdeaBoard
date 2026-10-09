import { computed, type Ref } from 'vue'
import {
  defineChartData,
  type ChartData,
  type ChartOptions,
  type ChartTooltipItem,
  withAlpha,
} from '../../../shared/lib/chartPrimitives'
import type { AdminStatistics, GetAdminStatisticsRangeValue } from './resource'
import {
  adminChartStrokeWidth,
  adminChartTheme,
  defineAdminChartCountScale,
  defineAdminChartPointClipStyle,
  defineAdminChartOptions,
} from '../shared/lib/chart'
import { buildAdminTimeRangeBuckets, shouldShowAdminTimeRangeTick } from '../shared/lib/timeRange'
import {
  baseChartOptions,
  baseChartPlugins,
  baseTooltipOptions,
  defaultModelColors,
  getModelColorByName,
} from './usageStatisticsChartBase'
import { formatCount } from './usageStatisticsTooltipText'
import { useUsageStatisticsModelHealthCharts } from './usageStatisticsModelHealthCharts'
import { useUsageStatisticsQualityTrendCharts } from './usageStatisticsQualityTrendCharts'
import { useUsageStatisticsSummaryMetrics } from './usageStatisticsSummaryMetrics'

const warnedTrendTotalMismatchKeys = new Set<string>()

export function useUsageStatisticsCharts(options: {
  adminStatistics: Ref<AdminStatistics | null>
  timeRange: Ref<GetAdminStatisticsRangeValue>
}) {
  const trendData = computed(() => {
    const trend = options.adminStatistics.value?.trend || []
    const trendMap = new Map(trend.map((day) => [day.date, day]))
    return buildAdminTimeRangeBuckets(options.timeRange.value).map((bucket) => {
      const day = trendMap.get(bucket.date) || {
        date: bucket.date,
        label: bucket.label,
        total: 0,
        captured_credits: 0,
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
      // 后端截断 segments[:3] 是正常契约：只在未截断时检查背离，且 Set 封顶防长期累积。
      if (import.meta.env.DEV && day.segments.length > 0 && day.segments.length <= 3) {
        const segmentSum = day.segments.reduce((sum, seg) => sum + (seg.value || 0), 0)
        if (segmentSum !== day.total) {
          const warnKey = `${bucket.date}:${day.total}:${segmentSum}`
          if (!warnedTrendTotalMismatchKeys.has(warnKey)) {
            if (warnedTrendTotalMismatchKeys.size > 200) warnedTrendTotalMismatchKeys.clear()
            warnedTrendTotalMismatchKeys.add(warnKey)
            console.warn(
              `[useUsageStatisticsCharts] trend day ${bucket.date}: segments sum (${segmentSum}) !== total (${day.total})`,
            )
          }
        }
      }
      return {
        date: day.label || bucket.label,
        total: day.total,
        captured: Math.max(0, day.captured_credits || 0),
        segments,
      }
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
    return Array.from(byModel.entries()).map(([label, meta]) => ({ label, color: meta.color }))
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
    if (label === '消耗积分') return { value: day.captured, total: day.captured, isCost: true }
    const segment = day.segments.find((seg) => seg.label === label)
    const value = segment?.value || 0
    const total = Math.max(0, day.total || 0)
    return {
      value,
      total,
    }
  }

  const hasCaptureCost = computed(() => trendData.value.some((day) => day.captured > 0))

  const trendChartData = computed<ChartData>(() => {
    const costColor = adminChartTheme.warning
    return defineChartData({
      labels: trendData.value.map((day) => day.date),
      datasets: [
        ...trendSeriesModelOrder.value.map((model) => ({
          type: 'line' as const,
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
        // 消耗积分（右轴）：只在有 capture 数据时出现，不进调用量堆叠。
        ...(hasCaptureCost.value
          ? [
              {
                type: 'line' as const,
                label: '消耗积分',
                data: trendData.value.map((day) => day.captured),
                borderColor: costColor,
                backgroundColor: withAlpha(costColor, 0.12),
                fill: false,
                tension: 0.38,
                borderWidth: adminChartStrokeWidth,
                ...defineAdminChartPointClipStyle(),
                pointRadius: trendData.value.length <= 1 ? 3 : 0,
                pointHoverRadius: 4,
                pointBackgroundColor: costColor,
                yAxisID: 'cost',
              },
            ]
          : []),
      ],
    })
  })

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
              if (!point) return `${context.dataset.label || '调用'}: ${formatCount(0)}`
              if ('isCost' in point) return `消耗积分: ${formatCount(point.value)} 积分`
              return `${context.dataset.label || '调用'}: ${formatCount(point.value)}`
            },
            afterBody: (items) => {
              const point = getUsageTrendTooltipPoint(items[0])
              if (!point) return []
              return [`总调用: ${formatCount(point.total)} 次`]
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
        ...(hasCaptureCost.value
          ? {
              cost: {
                ...defineAdminChartCountScale({
                  values: trendData.value.map((day) => day.captured),
                  min: 10,
                  headroom: 1.15,
                }),
                position: 'right' as const,
                grid: { drawOnChartArea: false },
              },
            }
          : {}),
      },
    }),
  )

  const summaryMetrics = useUsageStatisticsSummaryMetrics(options.adminStatistics)
  const { modelRiskChartData, modelRiskChartOptions } = useUsageStatisticsModelHealthCharts(
    options.adminStatistics,
  )

  const qualityTrendCharts = useUsageStatisticsQualityTrendCharts(options)

  return {
    trendChartData,
    trendChartOptions,
    ...summaryMetrics,
    modelRiskChartData,
    modelRiskChartOptions,
    ...qualityTrendCharts,
  }
}
