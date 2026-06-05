import { computed, type Ref } from 'vue'
import {
  adminChartTheme,
  defineAdminChartData,
  defineAdminChartOptions,
  getParsedY,
  type AdminChartData,
  type AdminChartOptions,
  type AdminTooltipItem,
  withAlpha,
} from '../../shared/lib/adminChart'

type StatisticsLike = {
  summary?: {
    total_calls?: number
    today_calls?: number
    today_success?: number
    today_failed?: number
    yesterday_calls?: number
    yesterday_success?: number
    yesterday_failed?: number
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
  user_risk?: Array<{
    name: string
    total: number
    completed: number
    failed: number
    failure_rate: number
  }>
}
type MonitorLike = {
  global_queue_len: number
  wait_queue_len: number
  running_tasks: number
  account_loads: Record<number, { current_concurrent: number; max_concurrent: number }>
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
  value: number
  name: string
  avgSeconds: number
  samples: number
}

type AccountLoadPoint = {
  value: number
  id: string
  current: number
  max: number
}

type UserRiskPoint = {
  value: number
  name: string
  failed: number
  completed: number
  total: number
  failureRate: number
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
  timeRange: Ref<string>
  monitorData: Ref<MonitorLike | null>
}) {
  function getModelColorByName(name: string) {
    const key = normalizeModelName(name)
    if (!key) return null
    return colorForModel(key, defaultModelColors)
  }

  const trendData = computed(() => {
    const days = options.timeRange.value === '7d' ? 7 : options.timeRange.value === '30d' ? 30 : 365
    const now = new Date()
    const trend = options.adminStatistics.value?.trend || []
    const trendMap = new Map(trend.map((day) => [day.date, day]))
    const fallback = Array.from({ length: days }, (_, idx) => {
      const d = new Date(now)
      d.setDate(now.getDate() - (days - 1 - idx))
      const date = `${d.getFullYear()}-${(d.getMonth() + 1).toString().padStart(2, '0')}-${d.getDate().toString().padStart(2, '0')}`
      const label = `${(d.getMonth() + 1).toString().padStart(2, '0')}/${d.getDate().toString().padStart(2, '0')}`
      return trendMap.get(date) || { date, label, total: 0, segments: [] }
    })
    return fallback.map((day) => {
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
      return { date: day.label || day.date, total: day.total, segments }
    })
  })

  const totalCallsInRange = computed(() => trendData.value.reduce((sum, d) => sum + d.total, 0))

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
    const count = trendData.value.length
    if (count <= 7) return true
    if (options.timeRange.value === '30d')
      return index === 0 || index === count - 1 || index % 6 === 0
    if (options.timeRange.value === '1y') {
      if (index === 0 || index === count - 1) return true
      const day = trendData.value[index]
      if (!day) return false
      return isMonthStartLabel(day.date)
    }
    return true
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
        borderWidth: 2,
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
            label: (context) =>
              `${context.dataset.label || '调用'}: ${getParsedY(context).toLocaleString()}`,
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
          stacked: true,
          beginAtZero: true,
          ticks: {
            color: adminChartTheme.axisText,
            precision: 0,
            callback: (value) => Math.round(Number(value)).toString(),
          },
          grid: { color: adminChartTheme.splitLine },
          border: { display: false },
        },
      },
    }),
  )

  const todayCalls = computed(() => options.adminStatistics.value?.summary?.today_calls || 0)
  const todaySuccess = computed(() => options.adminStatistics.value?.summary?.today_success || 0)
  const todayFailed = computed(() => options.adminStatistics.value?.summary?.today_failed || 0)
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

  const modelRiskRows = computed(() =>
    (options.adminStatistics.value?.model_health || [])
      .filter((item) => item.total > 0)
      .slice(0, 12)
      .map((item) => ({
        name: item.name === 'Unknown' ? '未识别模型' : item.name,
        total: item.total,
        failed: item.failed,
        failureRate: Math.max(0, Math.min(100, item.failure_rate || 0)),
        avgSeconds: Math.max(0, item.avg_seconds || 0),
      })),
  )

  const modelRiskChartData = computed<AdminChartData>(() => {
    const maxTotal = Math.max(1, ...modelRiskRows.value.map((item) => item.total))
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
          pointRadius: modelRiskRows.value.map((item) => 7 + Math.sqrt(item.total / maxTotal) * 18),
          pointHoverRadius: modelRiskRows.value.map(
            (item) => 9 + Math.sqrt(item.total / maxTotal) * 20,
          ),
          pointBackgroundColor: modelRiskRows.value.map((item) => riskColor(item.failureRate)),
          pointBorderColor: adminChartTheme.cardBg,
          pointBorderWidth: 1,
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
          beginAtZero: true,
          title: { display: true, text: '平均耗时', color: adminChartTheme.axisText },
          ticks: {
            color: adminChartTheme.axisText,
            callback: (value) => `${Number(value).toFixed(1)}s`,
          },
          grid: { color: adminChartTheme.splitLine },
          border: { display: false },
        },
        y: {
          beginAtZero: true,
          max: 100,
          title: { display: true, text: '失败率', color: adminChartTheme.axisText },
          ticks: {
            color: adminChartTheme.axisText,
            callback: (value) => `${Number(value)}%`,
          },
          grid: { color: adminChartTheme.splitLine },
          border: { display: false },
        },
      },
    }),
  )

  const qualityTrendRows = computed(() =>
    (options.adminStatistics.value?.quality_trend || []).map((item) => ({
      label: item.label || item.date,
      total: Math.max(0, Number(item.total) || 0),
      success: Math.max(0, Number(item.success) || 0),
      fail: Math.max(0, Number(item.fail) || 0),
      successRate: normalizePercent(item.success_rate),
    })),
  )

  const qualityTrendChartData = computed<AdminChartData>(() =>
    defineAdminChartData({
      labels: qualityTrendRows.value.map((item) => item.label),
      datasets: [
        {
          type: 'bar',
          label: '失败',
          data: qualityTrendRows.value.map((item) => item.fail),
          backgroundColor: withAlpha(adminChartTheme.danger, 0.82),
          borderRadius: 4,
          maxBarThickness: 18,
          yAxisID: 'y',
        },
        {
          type: 'line',
          label: '成功率',
          data: qualityTrendRows.value.map((item) => item.successRate),
          borderColor: adminChartTheme.brand,
          backgroundColor: adminChartTheme.brand,
          borderWidth: 2,
          tension: 0.36,
          pointRadius: 3,
          pointHoverRadius: 5,
          yAxisID: 'yRate',
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
              return label === '成功率' ? `${label}: ${value.toFixed(1)}%` : `${label}: ${value}`
            },
            afterBody: (items) => {
              const index = items[0]?.dataIndex ?? -1
              const row = qualityTrendRows.value[index]
              if (!row) return []
              return [
                `总调用: ${row.total.toLocaleString()}`,
                `成功: ${row.success.toLocaleString()}`,
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
              shouldShowAxisLabel(index, qualityTrendRows.value.length, options.timeRange.value)
                ? qualityTrendRows.value[index]?.label || ''
                : '',
          },
          grid: { display: false },
          border: { color: adminChartTheme.axisLine },
        },
        y: {
          beginAtZero: true,
          ticks: { color: adminChartTheme.axisText, precision: 0 },
          grid: { color: adminChartTheme.splitLine },
          border: { display: false },
        },
        yRate: {
          beginAtZero: true,
          max: 100,
          position: 'right',
          ticks: {
            color: adminChartTheme.axisText,
            callback: (value) => `${Number(value)}%`,
          },
          grid: { display: false },
          border: { display: false },
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

  const slowModelsChartData = computed<AdminChartData>(() =>
    defineAdminChartData({
      labels: slowModelRows.value.map((row) => truncateLabel(row.name, 18)),
      datasets: [
        {
          type: 'bar',
          label: 'P95',
          data: slowModelRows.value.map<SlowModelPoint>((row) => ({
            value: row.p95Seconds,
            name: row.name,
            avgSeconds: row.avgSeconds,
            samples: row.samples,
          })),
          parsing: { xAxisKey: 'value' },
          backgroundColor: slowModelRows.value.map((row) => row.color),
          borderRadius: 5,
          barThickness: 8,
        },
      ],
    }),
  )

  const slowModelsChartOptions = computed<AdminChartOptions>(() =>
    horizontalBarOptions({
      tooltipTitle: (context) => getSlowModelPoint(context)?.name || '',
      tooltipLines: (context) => {
        const point = getSlowModelPoint(context)
        if (!point) return []
        return [
          `P95: ${point.value.toFixed(2)}s`,
          `平均: ${point.avgSeconds.toFixed(2)}s`,
          `样本: ${point.samples.toLocaleString()}`,
        ]
      },
      xTick: (value) => `${Number(value).toFixed(1)}s`,
    }),
  )

  const monitorQueueTotal = computed(
    () =>
      (options.monitorData.value?.global_queue_len || 0) +
      (options.monitorData.value?.wait_queue_len || 0),
  )
  const monitorRunningTotal = computed(() => options.monitorData.value?.running_tasks || 0)
  const queuePressurePercent = computed(() => {
    const pending = monitorQueueTotal.value
    const running = monitorRunningTotal.value
    const total = pending + running
    if (total === 0) return 0
    return Math.round((pending / total) * 100)
  })
  const queuePressure = computed(() => `${queuePressurePercent.value}%`)
  const queuePressureFoot = computed(
    () => `排队 ${monitorQueueTotal.value} / 执行 ${monitorRunningTotal.value}`,
  )

  const accountLoadRows = computed(() => {
    const loads = options.monitorData.value?.account_loads || {}
    return Object.entries(loads)
      .map(([id, val]) => {
        const current = val.current_concurrent || 0
        const max = Math.max(1, val.max_concurrent || 0)
        const percent = Math.min(100, Math.round((current / max) * 100))
        return { id, current, max, percent, color: riskColor(percent, { warning: 70, danger: 90 }) }
      })
      .sort((a, b) => b.percent - a.percent)
      .slice(0, 5)
  })

  const peakAccountLoad = computed(() => accountLoadRows.value[0] || null)
  const peakAccountLoadPercent = computed(() => peakAccountLoad.value?.percent || 0)

  const accountLoadChartData = computed<AdminChartData>(() =>
    defineAdminChartData({
      labels: accountLoadRows.value.map((item) => `#${item.id}`),
      datasets: [
        {
          type: 'bar',
          label: '并发负载',
          data: accountLoadRows.value.map<AccountLoadPoint>((item) => ({
            value: item.percent,
            id: item.id,
            current: item.current,
            max: item.max,
          })),
          parsing: { xAxisKey: 'value' },
          backgroundColor: accountLoadRows.value.map((item) => item.color),
          borderRadius: 6,
          barThickness: 10,
        },
      ],
    }),
  )

  const accountLoadChartOptions = computed<AdminChartOptions>(() =>
    horizontalBarOptions({
      tooltipTitle: (context) => {
        const point = getAccountLoadPoint(context)
        return point ? `#${point.id}` : ''
      },
      tooltipLines: (context) => {
        const point = getAccountLoadPoint(context)
        if (!point) return []
        return [`并发: ${point.current}/${point.max}`, `负载: ${point.value}%`]
      },
      xTick: (value) => `${Number(value)}%`,
      max: 100,
    }),
  )

  const userRiskRows = computed(() =>
    (options.adminStatistics.value?.user_risk || [])
      .map((item) => ({
        name: item.name || '未知用户',
        total: Math.max(0, Number(item.total) || 0),
        completed: Math.max(0, Number(item.completed) || 0),
        failed: Math.max(0, Number(item.failed) || 0),
        failureRate: normalizePercent(item.failure_rate),
      }))
      .sort((a, b) => b.failed - a.failed)
      .slice(0, 6),
  )

  const userRiskChartData = computed<AdminChartData>(() =>
    defineAdminChartData({
      labels: userRiskRows.value.map((item) => truncateLabel(item.name, 16)),
      datasets: [
        {
          type: 'bar',
          label: '失败',
          data: userRiskRows.value.map<UserRiskPoint>((item) => ({
            value: item.failed,
            name: item.name,
            failed: item.failed,
            completed: item.completed,
            total: item.total,
            failureRate: item.failureRate,
          })),
          parsing: { xAxisKey: 'value' },
          backgroundColor: userRiskRows.value.map((item) =>
            riskColor(item.failureRate, { warning: 8, danger: 20 }),
          ),
          borderRadius: 5,
          barThickness: 8,
        },
      ],
    }),
  )

  const userRiskChartOptions = computed<AdminChartOptions>(() =>
    horizontalBarOptions({
      tooltipTitle: (context) => getUserRiskPoint(context)?.name || '',
      tooltipLines: (context) => {
        const point = getUserRiskPoint(context)
        if (!point) return []
        return [
          `失败: ${point.failed.toLocaleString()}`,
          `成功: ${point.completed.toLocaleString()}`,
          `总调用: ${point.total.toLocaleString()}`,
          `失败率: ${point.failureRate.toFixed(1)}%`,
        ]
      },
    }),
  )

  return {
    trendData,
    totalCallsInRange,
    trendChartData,
    trendChartOptions,
    todayCalls,
    todayFailed,
    todayCallsFoot,
    todayFailedFoot,
    todaySuccessRate,
    todaySuccessRateFoot,
    queuePressure,
    queuePressureFoot,
    modelRiskChartData,
    modelRiskChartOptions,
    qualityTrendChartData,
    qualityTrendChartOptions,
    slowModelsChartData,
    slowModelsChartOptions,
    accountLoadChartData,
    accountLoadChartOptions,
    peakAccountLoadPercent,
    userRiskChartData,
    userRiskChartOptions,
  }
}

function baseChartOptions(): AdminChartOptions {
  return {
    responsive: true,
    maintainAspectRatio: false,
    resizeDelay: 80,
    animation: { duration: 260 },
    plugins: {
      legend: {
        position: 'top',
        align: 'start',
        labels: {
          boxWidth: 10,
          boxHeight: 6,
          usePointStyle: true,
          color: adminChartTheme.legendText,
        },
      },
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

function horizontalBarOptions(config: {
  tooltipTitle: (context: AdminTooltipItem) => string
  tooltipLines: (context: AdminTooltipItem) => string[]
  xTick?: (value: string | number) => string
  max?: number
}): AdminChartOptions {
  return defineAdminChartOptions({
    ...baseChartOptions(),
    indexAxis: 'y',
    plugins: {
      legend: { display: false },
      tooltip: {
        ...baseTooltipOptions(),
        callbacks: {
          title: (items) => (items[0] ? config.tooltipTitle(items[0]) : ''),
          label: (context) => config.tooltipLines(context),
        },
      },
    },
    scales: {
      x: {
        beginAtZero: true,
        max: config.max,
        ticks: {
          color: adminChartTheme.axisText,
          callback: (value) => (config.xTick ? config.xTick(value) : String(value)),
        },
        grid: { color: adminChartTheme.splitLine },
        border: { display: false },
      },
      y: {
        ticks: { color: adminChartTheme.axisText },
        grid: { display: false },
        border: { display: false },
      },
    },
  })
}

function getModelRiskPoint(context: AdminTooltipItem | undefined) {
  return context?.raw as ModelRiskPoint | undefined
}

function getSlowModelPoint(context: AdminTooltipItem | undefined) {
  return context?.raw as SlowModelPoint | undefined
}

function getAccountLoadPoint(context: AdminTooltipItem | undefined) {
  return context?.raw as AccountLoadPoint | undefined
}

function getUserRiskPoint(context: AdminTooltipItem | undefined) {
  return context?.raw as UserRiskPoint | undefined
}

function normalizeModelName(name: string | null | undefined): string {
  return (name || '').trim().toLowerCase()
}

function colorForModel(name: string, palette: string[]) {
  let hash = 0
  for (let i = 0; i < name.length; i++) hash = (hash << 5) - hash + name.charCodeAt(i)
  return palette[Math.abs(hash) % palette.length]
}

function isMonthStartLabel(label: string): boolean {
  const value = (label || '').trim()
  if (!value) return false
  const mmdd = /^(\d{2})\/(\d{2})$/.exec(value)
  if (mmdd) return mmdd[2] === '01'
  const ymd = /^(\d{4})-(\d{2})-(\d{2})$/.exec(value)
  if (ymd) return ymd[3] === '01'
  return false
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

function normalizePercent(value: number): number {
  const parsed = Number(value) || 0
  const percent = parsed <= 1 ? parsed * 100 : parsed
  return Math.max(0, Math.min(100, percent))
}

function shouldShowAxisLabel(index: number, count: number, range: string) {
  if (count <= 7) return true
  if (range === '30d') return index === 0 || index === count - 1 || index % 6 === 0
  if (range === '1y') return index === 0 || index === count - 1 || index % 30 === 0
  return true
}

function riskColor(value: number, thresholds = { warning: 5, danger: 20 }) {
  if (value >= thresholds.danger) return adminChartTheme.danger
  if (value >= thresholds.warning) return adminChartTheme.warning
  return adminChartTheme.success
}

function truncateLabel(value: string, maxLength: number) {
  if (value.length <= maxLength) return value
  return `${value.slice(0, maxLength - 1)}…`
}
