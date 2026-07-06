import { computed, type Ref } from 'vue'
import type {
  AdminStatistics,
  GetAdminStatisticsRangeValue,
} from './statisticsResource'
import {
  adminChartStrokeWidth,
  adminChartTheme,
  defineAdminChartBubbleDatasetStyle,
  defineAdminChartBubblePointStyle,
  defineAdminChartCountScale,
  defineAdminChartData,
  defineAdminChartDurationScale,
  defineAdminChartOptions,
  defineAdminChartPercentScale,
  defineAdminChartPointClipStyle,
  getAdminChartBubbleHoverRadius,
  getAdminChartBubbleRadius,
  type AdminChartData,
  type AdminChartOptions,
  type AdminTooltipItem,
  withAlpha,
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
  toNonNegativeInteger,
  toNonNegativeNumber,
} from './usageStatisticsChartBase'
import { useUsageStatisticsQualityTrendCharts } from './usageStatisticsQualityTrendCharts'
import { useUsageStatisticsSummaryMetrics } from './usageStatisticsSummaryMetrics'

type ModelRiskPoint = {
  x: number
  y: number
  name: string
  total: number
  failed: number
  avgSeconds: number
  failureRate: number
}

type SlowModelPoint = {
  x: number
  y: number
  r: number
  name: string
  p95Seconds: number
  avgSeconds: number
  samples: number
}

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
          color: segment.color || defaultModelColors[idx % defaultModelColors.length],
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

  function getUsageTrendTooltipPoint(context: AdminTooltipItem | undefined) {
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

  const trendChartData = computed<AdminChartData>(() =>
    defineAdminChartData({
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

  const trendChartOptions = computed<AdminChartOptions>(() =>
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

  const modelRiskRows = computed(() =>
    (options.adminStatistics.value?.model_health || [])
      .filter((item) => item.total > 0)
      .map((item, idx) => ({
        name: item.name === 'Unknown' ? '未识别模型' : item.name,
        total: item.total,
        failed: item.failed,
        failureRate: Math.max(0, Math.min(100, item.failure_rate || 0)),
        avgSeconds: Math.max(0, item.avg_seconds || 0),
        color:
          getModelColorByName(item.name) ||
          defaultModelColors[idx % defaultModelColors.length] ||
          adminChartTheme.brand,
      }))
      .slice(0, 12),
  )

  const modelRiskChartData = computed<AdminChartData>(() => {
    const maxTotal = Math.max(1, ...modelRiskRows.value.map((item) => item.total))
    const colors = modelRiskRows.value.map((item) => item.color)
    const points = modelRiskRows.value.map<ModelRiskPoint>((item) => ({
      x: item.avgSeconds,
      y: item.failureRate,
      name: item.name,
      total: item.total,
      failed: item.failed,
      avgSeconds: item.avgSeconds,
      failureRate: item.failureRate,
    }))
    return defineAdminChartData({
      labels: modelRiskRows.value.map((item) => item.name),
      datasets: [
        {
          type: 'scatter',
          label: '模型风险',
          data: points,
          pointRadius: modelRiskRows.value.map((item) =>
            getAdminChartBubbleRadius(item.total, maxTotal),
          ),
          pointHoverRadius: modelRiskRows.value.map((item) =>
            getAdminChartBubbleHoverRadius(item.total, maxTotal),
          ),
          ...defineAdminChartBubblePointStyle(colors),
        },
      ],
    })
  })

  const modelRiskChartOptions = computed<AdminChartOptions>(() =>
    defineAdminChartOptions({
      ...baseChartOptions(),
      interaction: { mode: 'nearest', intersect: true },
      plugins: {
        legend: { display: false },
        tooltip: {
          ...baseTooltipOptions(),
          callbacks: {
            title: (items) => getModelRiskPoint(items[0])?.name || '未知模型',
            label: (context) => {
              const point = getModelRiskPoint(context)
              if (!point) return ''
              return `失败率: ${point.failureRate.toFixed(1)}%`
            },
            afterLabel: (context) => {
              const point = getModelRiskPoint(context)
              if (!point) return []
              return [
                `平均耗时: ${point.avgSeconds.toFixed(2)}s`,
                `调用量: ${point.total.toLocaleString()}`,
                `失败: ${point.failed.toLocaleString()}`,
              ]
            },
          },
        },
      },
      scales: {
        x: {
          ...defineAdminChartDurationScale({
            values: modelRiskRows.value.map((row) => row.avgSeconds),
            min: 1,
            headroom: 1.15,
          }),
        },
        y: {
          ...defineAdminChartPercentScale(),
        },
      },
    }),
  )

  const qualityTrendCharts = useUsageStatisticsQualityTrendCharts(options)

  const slowModelRows = computed(() =>
    (options.adminStatistics.value?.slow_models || [])
      .map((item, idx) => {
        const modelName = item.name || '未知模型'
        const p95Seconds = toNonNegativeNumber(item.p95_seconds)
        const avgSeconds = toNonNegativeNumber(item.avg_seconds)
        const samples = toNonNegativeInteger(item.samples)
        return {
          name: modelName,
          p95Seconds,
          avgSeconds,
          samples,
          color:
            getModelColorByName(modelName) ||
            defaultModelColors[idx % defaultModelColors.length] ||
            adminChartTheme.brand,
        }
      })
      .sort((a, b) => b.p95Seconds - a.p95Seconds)
      .slice(0, 6),
  )

  const slowModelsChartData = computed<AdminChartData>(() => {
    const maxSamples = Math.max(1, ...slowModelRows.value.map((row) => row.samples))
    const colors = slowModelRows.value.map((row) => row.color)

    return defineAdminChartData({
      labels: slowModelRows.value.map((row) => row.name),
      datasets: [
        {
          type: 'bubble',
          label: '模型尾延迟',
          data: slowModelRows.value.map<SlowModelPoint>((row) => ({
            x: row.p95Seconds,
            y: row.avgSeconds,
            r: getAdminChartBubbleRadius(row.samples, maxSamples),
            name: row.name,
            p95Seconds: row.p95Seconds,
            avgSeconds: row.avgSeconds,
            samples: row.samples,
          })),
          ...defineAdminChartBubbleDatasetStyle(colors),
        },
      ],
    })
  })

  const slowModelsChartOptions = computed<AdminChartOptions>(() =>
    defineAdminChartOptions({
      ...baseChartOptions(),
      layout: { padding: { top: 10, right: 18 } },
      plugins: {
        ...baseChartPlugins(),
        legend: { display: false },
        tooltip: {
          ...baseTooltipOptions(),
          callbacks: {
            title: (items) => getSlowModelPoint(items[0])?.name || '',
            label: (context) => {
              const point = getSlowModelPoint(context)
              if (!point) return []
              return [
                `P95: ${point.p95Seconds.toFixed(2)}s`,
                `平均: ${point.avgSeconds.toFixed(2)}s`,
                `样本: ${point.samples.toLocaleString()}`,
              ]
            },
          },
        },
      },
      scales: {
        x: {
          ...defineAdminChartDurationScale({
            values: slowModelRows.value.map((row) => row.p95Seconds),
            min: 1,
            headroom: 1.15,
            borderColor: adminChartTheme.axisLine,
          }),
        },
        y: {
          ...defineAdminChartDurationScale({
            values: slowModelRows.value.map((row) => row.avgSeconds),
            min: 1,
            headroom: 1.15,
          }),
        },
      },
    }),
  )

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

function getModelRiskPoint(context: AdminTooltipItem | undefined) {
  return context?.raw as ModelRiskPoint | undefined
}

function getSlowModelPoint(context: AdminTooltipItem | undefined) {
  return context?.raw as SlowModelPoint | undefined
}
