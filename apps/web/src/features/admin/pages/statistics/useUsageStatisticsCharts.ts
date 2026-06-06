import { computed, type Ref } from 'vue'
import {
  adminChartLegend,
  adminChartStrokeWidth,
  adminChartTheme,
  defineAdminChartBubbleDatasetStyle,
  defineAdminChartBubblePointStyle,
  defineAdminChartCountScale,
  defineAdminChartData,
  defineAdminChartDurationScale,
  defineAdminChartOptions,
  defineAdminChartPercentScale,
  defineAdminChartRadialCountScale,
  getAdminChartBubbleHoverRadius,
  getAdminChartBubbleRadius,
  getParsedY,
  type AdminChartData,
  type AdminChartOptions,
  type AdminTooltipItem,
  withAlpha,
} from '../../shared/lib/adminChart'
import {
  buildAdminTimeRangeBuckets,
  shouldShowAdminTimeRangeTick,
} from '../../shared/lib/adminTimeRange'
import type { AdminStatisticsRange } from '../../shared/queries/adminActions'

type StatisticsLike = {
  summary?: {
    total_calls?: number
    today_calls?: number
    today_success?: number
    today_failed?: number
    yesterday_calls?: number
    yesterday_success?: number
    yesterday_failed?: number
    this_month_calls?: number
  }
  trend?: Array<{
    date: string
    label: string
    total: number
    segments: Array<{ label: string; value: number }>
  }>
  model_health?: Array<{
    name: string
    total: number
    completed: number
    failed: number
    failure_rate: number
    avg_seconds: number
  }>
  quality_trend?: Array<{
    date: string
    label: string
    total: number
    success: number
    fail: number
    success_rate: number
  }>
  slow_models?: Array<{
    name: string
    avg_seconds: number
    p95_seconds: number
    samples: number
  }>
  bottleneck_sources?: Array<{
    source: string
    label: string
    total: number
    failed: number
    avg_latency_ms: number
  }>
  failure_categories?: Array<{
    category: string
    label: string
    total: number
    avg_latency_ms: number
  }>
}

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

const defaultModelColors = [
  adminChartTheme.brand,
  adminChartTheme.slate,
  adminChartTheme.cyan,
  adminChartTheme.olive,
  adminChartTheme.mauve,
  adminChartTheme.mist,
]

export function useUsageStatisticsCharts(options: {
  adminStatistics: Ref<StatisticsLike | null>
  qualityStatistics?: Ref<StatisticsLike | null>
  timeRange: Ref<AdminStatisticsRange>
  qualityTimeRange?: Ref<AdminStatisticsRange>
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
        ...baseChartOptions().plugins,
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

  const todayCalls = computed(() => options.adminStatistics.value?.summary?.today_calls || 0)
  const todaySuccess = computed(() => options.adminStatistics.value?.summary?.today_success || 0)
  const todayFailed = computed(() => options.adminStatistics.value?.summary?.today_failed || 0)
  const monthCalls = computed(() => options.adminStatistics.value?.summary?.this_month_calls || 0)
  const yesterdayCalls = computed(
    () => options.adminStatistics.value?.summary?.yesterday_calls || 0,
  )
  const yesterdaySuccess = computed(
    () => options.adminStatistics.value?.summary?.yesterday_success || 0,
  )
  const yesterdayFailed = computed(
    () => options.adminStatistics.value?.summary?.yesterday_failed || 0,
  )

  const todayCallsFoot = computed(() =>
    buildDeltaFoot(todayCalls.value, yesterdayCalls.value, '昨日'),
  )
  const todayFailedFoot = computed(() =>
    buildDeltaFoot(todayFailed.value, yesterdayFailed.value, '昨日'),
  )
  const todayAttemptedCalls = computed(() => todaySuccess.value + todayFailed.value)
  const todaySuccessRate = computed(() =>
    todayAttemptedCalls.value === 0
      ? '0.0%'
      : `${((todaySuccess.value / todayAttemptedCalls.value) * 100).toFixed(1)}%`,
  )
  const yesterdayAttemptedCalls = computed(() => yesterdaySuccess.value + yesterdayFailed.value)
  const todaySuccessRateFoot = computed(() => {
    if (yesterdayAttemptedCalls.value === 0) return '昨日无可比数据'
    const yesterdayRate = yesterdaySuccess.value / yesterdayAttemptedCalls.value
    const currentRate =
      todayAttemptedCalls.value === 0 ? 0 : todaySuccess.value / todayAttemptedCalls.value
    return `${formatSignedPercentPoint((currentRate - yesterdayRate) * 100)} 较昨日`
  })
  const monthCallsFoot = computed(() => {
    const totalCalls = options.adminStatistics.value?.summary?.total_calls || 0
    if (totalCalls <= 0) return '累计无调用'
    return `占总调用 ${formatPercentValue(monthCalls.value, totalCalls)}`
  })

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

  const qualityTrendRange = computed(() => options.qualityTimeRange?.value ?? options.timeRange.value)
  const qualityTrendRows = computed(() => {
    const trend = (options.qualityStatistics?.value ?? options.adminStatistics.value)?.quality_trend || []
    const trendMap = new Map(trend.map((item) => [item.date, item]))

    return buildAdminTimeRangeBuckets(qualityTrendRange.value).map((bucket) => {
      const item = trendMap.get(bucket.date)
      const total = Math.max(0, Number(item?.total) || 0)
      const success = Math.max(0, Number(item?.success) || 0)
      const fail = Math.max(0, Number(item?.fail) || 0)

      return {
        label: item?.label || bucket.label,
        total,
        success,
        fail,
        successRate: normalizePercent(item?.success_rate || 0),
        failureRate: total > 0 ? normalizePercent((fail / total) * 100) : 0,
      }
    })
  })

  const qualityTrendChartData = computed<AdminChartData>(() =>
    defineAdminChartData({
      labels: qualityTrendRows.value.map((item) => item.label),
      datasets: [
        {
          type: 'line',
          label: '成功率',
          data: qualityTrendRows.value.map((item) => item.successRate),
          borderColor: adminChartTheme.success,
          backgroundColor: withAlpha(adminChartTheme.success, 0.12),
          borderWidth: adminChartStrokeWidth,
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
          fill: true,
          tension: 0.36,
          pointRadius: 2.5,
          pointHoverRadius: 5,
          yAxisID: 'y',
        },
      ],
    }),
  )

  const qualityTrendChartOptions = computed<AdminChartOptions>(() =>
    defineAdminChartOptions({
      ...baseChartOptions(),
      interaction: { mode: 'index', intersect: false },
      plugins: {
        ...baseChartOptions().plugins,
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

  const slowModelRows = computed(() =>
    (options.adminStatistics.value?.slow_models || [])
      .map((item, idx) => {
        const modelName = item.name || '未知模型'
        const p95Seconds = Math.max(0, Number(item.p95_seconds) || 0)
        const avgSeconds = Math.max(0, Number(item.avg_seconds) || 0)
        const samples = Math.max(0, Math.round(Number(item.samples) || 0))
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
        ...baseChartOptions().plugins,
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

  const bottleneckSourceRows = computed(() =>
    (options.adminStatistics.value?.bottleneck_sources || [])
      .map((item, idx) => ({
        source: item.source || 'unknown',
        label: item.label || bottleneckSourceFallbackLabel(item.source),
        total: Math.max(0, Number(item.total) || 0),
        failed: Math.max(0, Number(item.failed) || 0),
        avgLatencyMS: Math.max(0, Number(item.avg_latency_ms) || 0),
        color: bottleneckSourceColor(item.source, idx),
      }))
      .filter((item) => item.total > 0)
      .sort((a, b) => b.total - a.total)
      .slice(0, 6),
  )

  const bottleneckSourceChartData = computed<AdminChartData>(() =>
    defineAdminChartData({
      labels: bottleneckSourceRows.value.map((item) => item.label),
      datasets: [
        {
          type: 'doughnut',
          label: '任务量',
          data: bottleneckSourceRows.value.map((item) => item.total),
          backgroundColor: bottleneckSourceRows.value.map((item) => item.color),
          borderColor: '#ffffff',
          borderWidth: adminChartStrokeWidth,
          hoverOffset: 4,
        },
      ],
    }),
  )

  const bottleneckSourceChartOptions = computed<AdminChartOptions>(() =>
    defineAdminChartOptions({
      ...baseChartOptions(),
      cutout: '58%',
      plugins: {
        ...baseChartOptions().plugins,
        legend: adminChartLegend('bottom'),
        tooltip: {
          ...baseTooltipOptions(),
          callbacks: {
            label: (context) => {
              const point = bottleneckSourceRows.value[context.dataIndex]
              if (!point) return ''
              return [
                `任务: ${point.total.toLocaleString()}`,
                `失败: ${point.failed.toLocaleString()}`,
                `平均耗时: ${(point.avgLatencyMS / 1000).toFixed(2)}s`,
              ]
            },
          },
        },
      },
    } as AdminChartOptions),
  )

  const failureCategoryRows = computed(() => {
    const rows = (options.adminStatistics.value?.failure_categories || [])
      .map((item, idx) => ({
        category: item.category || 'unknown',
        label: item.label || failureCategoryFallbackLabel(item.category),
        total: Math.max(0, Number(item.total) || 0),
        avgLatencyMS: Math.max(0, Number(item.avg_latency_ms) || 0),
        color: failureCategoryColor(item.category, idx),
      }))
      .filter((item) => item.total > 0)
      .sort((a, b) => b.total - a.total)
      .slice(0, 6)
    const totalFailures = rows.reduce((sum, item) => sum + item.total, 0)
    return rows.map((item) => ({
      ...item,
      sharePercent: totalFailures > 0 ? (item.total / totalFailures) * 100 : 0,
    }))
  })

  const failureCategoryChartData = computed<AdminChartData>(() =>
    defineAdminChartData({
      labels: failureCategoryRows.value.map((item) => item.label),
      datasets: [
        {
          type: 'polarArea',
          label: '失败量',
          data: failureCategoryRows.value.map((item) => item.total),
          backgroundColor: failureCategoryRows.value.map((item) => withAlpha(item.color, 0.62)),
          borderColor: failureCategoryRows.value.map((item) => item.color),
          borderWidth: adminChartStrokeWidth,
        },
      ],
    }),
  )

  const failureCategoryChartOptions = computed<AdminChartOptions>(() =>
    defineAdminChartOptions({
      ...baseChartOptions(),
      plugins: {
        ...baseChartOptions().plugins,
        legend: adminChartLegend('bottom'),
        tooltip: {
          ...baseTooltipOptions(),
          callbacks: {
            label: (context) => {
              const point = failureCategoryRows.value[context.dataIndex]
              if (!point) return ''
              return [
                `失败: ${point.total.toLocaleString()}`,
                `占比: ${point.sharePercent.toFixed(1)}%`,
                `平均耗时: ${(point.avgLatencyMS / 1000).toFixed(2)}s`,
              ]
            },
          },
        },
      },
      scales: {
        r: {
          ...defineAdminChartRadialCountScale({
            values: failureCategoryRows.value.map((row) => row.total),
            min: 5,
            headroom: 1.1,
          }),
        },
      },
    }),
  )

  return {
    trendData,
    trendChartData,
    trendChartOptions,
    todayCalls,
    todayFailed,
    todayCallsFoot,
    todayFailedFoot,
    todaySuccessRate,
    todaySuccessRateFoot,
    monthCalls,
    monthCallsFoot,
    modelRiskChartData,
    modelRiskChartOptions,
    qualityTrendChartData,
    qualityTrendChartOptions,
    slowModelsChartData,
    slowModelsChartOptions,
    bottleneckSourceChartData,
    bottleneckSourceChartOptions,
    failureCategoryChartData,
    failureCategoryChartOptions,
  }
}

function baseChartOptions(): AdminChartOptions {
  return {
    responsive: true,
    maintainAspectRatio: false,
    resizeDelay: 80,
    animation: { duration: 260 },
    plugins: {
      legend: adminChartLegend(),
      tooltip: baseTooltipOptions(),
    },
  }
}

function baseTooltipOptions() {
  return {
    backgroundColor: adminChartTheme.tooltipBg,
    borderColor: adminChartTheme.tooltipBorder,
    borderWidth: 1,
    titleColor: adminChartTheme.tooltipText,
    bodyColor: adminChartTheme.tooltipText,
    bodyFont: { size: 12 },
    titleFont: { size: 12, weight: 700 },
    padding: 10,
    displayColors: true,
  }
}

function getModelRiskPoint(context: AdminTooltipItem | undefined) {
  return context?.raw as ModelRiskPoint | undefined
}

function getSlowModelPoint(context: AdminTooltipItem | undefined) {
  return context?.raw as SlowModelPoint | undefined
}

function normalizeModelName(name: string | null | undefined): string {
  return (name || '').trim().toLowerCase()
}

function colorForModel(name: string, palette: string[]) {
  let hash = 0
  for (let i = 0; i < name.length; i++) hash = (hash << 5) - hash + name.charCodeAt(i)
  return palette[Math.abs(hash) % palette.length]
}

function buildDeltaFoot(current: number, previous: number, label: string) {
  if (previous === 0) return current === 0 ? '无数据' : `${current}（${label}无可比数据）`
  const diff = current - previous
  const ratio = Math.abs((diff / previous) * 100).toFixed(1)
  if (diff === 0) return `0.0% 持平 ${label}`
  return `${diff > 0 ? '+' : '-'}${ratio}% ${diff > 0 ? '较' : '低于'}${label}`
}

function formatSignedPercentPoint(value: number) {
  if (value === 0) return '0.0pp'
  return `${value > 0 ? '+' : '-'}${Math.abs(value).toFixed(1)}pp`
}

function formatPercentValue(value: number, total: number) {
  if (total <= 0) return '0.0%'
  return `${((value / total) * 100).toFixed(1)}%`
}

function normalizePercent(value: number): number {
  const parsed = Number(value) || 0
  const percent = parsed <= 1 ? parsed * 100 : parsed
  return Math.max(0, Math.min(100, percent))
}

function bottleneckSourceFallbackLabel(source: string | undefined) {
  const labels: Record<string, string> = {
    queue: '队列等待',
    worker: 'Worker 调度',
    provider: '上游模型',
    storage: '存储链路',
    finalize: '结果落库',
    input: '输入校验',
    unknown: '未知来源',
  }
  return labels[source || 'unknown'] || labels.unknown
}

function bottleneckSourceColor(source: string | undefined, index: number) {
  const colors: Record<string, string> = {
    queue: adminChartTheme.cyan,
    worker: adminChartTheme.slate,
    provider: adminChartTheme.brand,
    storage: adminChartTheme.danger,
    finalize: adminChartTheme.olive,
    input: adminChartTheme.warning,
    unknown: adminChartTheme.mist,
  }
  return colors[source || 'unknown'] || defaultModelColors[index % defaultModelColors.length]
}

function failureCategoryFallbackLabel(category: string | undefined) {
  const labels: Record<string, string> = {
    input_invalid: '输入无效',
    auth: '认证失败',
    quota: '额度限制',
    timeout: '执行超时',
    provider_error: '上游错误',
    storage_error: '存储错误',
    finalize_error: '结果落库',
    cancelled: '任务取消',
    unknown: '未知错误',
  }
  return labels[category || 'unknown'] || labels.unknown
}

function failureCategoryColor(category: string | undefined, index: number) {
  const colors: Record<string, string> = {
    input_invalid: adminChartTheme.warning,
    auth: adminChartTheme.mauve,
    quota: adminChartTheme.olive,
    timeout: adminChartTheme.danger,
    provider_error: adminChartTheme.brand,
    storage_error: adminChartTheme.cyan,
    finalize_error: adminChartTheme.slate,
    cancelled: adminChartTheme.mist,
    unknown: adminChartTheme.emptyText,
  }
  return colors[category || 'unknown'] || defaultModelColors[index % defaultModelColors.length]
}
