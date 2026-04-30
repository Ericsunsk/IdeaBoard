import { computed, ref, type Ref } from 'vue'
import { splitCommaValues } from '../shared/assetLibraryShared'

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
    pending?: number
    running?: number
    completed?: number
    failed?: number
  }
  trend?: Array<{ date: string; label: string; total: number; segments: Array<{ label: string; value: number }> }>
  model_ranking?: Array<{ name: string; value: number; percent: number }>
  user_rank?: Array<{ name: string; value: number }>
  latency?: Array<{ name: string; avg_seconds: number }>
  health_cells?: Array<{ status: number; success: number; fail: number; total: number }>
}
type HealthCellLike = { status: number; success: number; fail: number; total: number }

type MonitorLike = {
  global_queue_len: number
  wait_queue_len: number
  running_tasks: number
  account_loads: Record<number, { current_concurrent: number; max_concurrent: number }>
}

type ChannelLike = { id: number; name: string; models: string }
type ConfiguredAccountLike = { model_set: string; platform: string; channel_id: number }

export function useUsageStatisticsCharts(options: {
  adminStatistics: Ref<StatisticsLike | null>
  configuredAccounts: Ref<ConfiguredAccountLike[]>
  configuredChannels: Ref<ChannelLike[]>
  timeRange: Ref<string>
  monitorData: Ref<MonitorLike | null>
}) {
  const defaultModelColors = [
    'var(--admin-model-indigo)',
    'var(--admin-model-purple)',
    'var(--admin-model-cyan)',
    '#14b8a6',
    '#f59e0b',
    '#ef4444',
  ]
  const HEALTH_COLS = 70
  const HEALTH_ROWS = 6

  const configuredModelProviderMap = computed(() => {
    const map = new Map<string, string>()
    const channelNameByID = new Map<number, string>()
    options.configuredChannels.value.forEach((channel) => {
      channelNameByID.set(channel.id, channel.name || '')
    })
    options.configuredAccounts.value.forEach((account) => {
      const channelName = channelNameByID.get(account.channel_id) || account.platform || '未配置渠道'
      splitCommaValues(account.model_set).forEach((model) => {
        const key = normalizeModelName(model)
        if (!key || map.has(key)) return
        map.set(key, channelName)
      })
    })
    options.configuredChannels.value.forEach((channel) => {
      splitCommaValues(channel.models).forEach((model) => {
        const key = normalizeModelName(model)
        if (!key || map.has(key)) return
        map.set(key, channel.name || '未配置渠道')
      })
    })
    return map
  })

  const configuredModelNames = computed(() => {
    const names: string[] = []
    const seen = new Set<string>()
    options.configuredAccounts.value.forEach((account) => {
      splitCommaValues(account.model_set).forEach((model) => {
        const key = normalizeModelName(model)
        if (!key || seen.has(key)) return
        seen.add(key)
        names.push(model)
      })
    })
    options.configuredChannels.value.forEach((channel) => {
      splitCommaValues(channel.models).forEach((model) => {
        const key = normalizeModelName(model)
        if (!key || seen.has(key)) return
        seen.add(key)
        names.push(model)
      })
    })
    return names
  })

  function getModelMetaByName(name: string) {
    const key = normalizeModelName(name)
    if (!key) return null
    const provider = configuredModelProviderMap.value.get(key) || '未配置渠道'
    return { label: provider, color: colorForModel(key, defaultModelColors) }
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
        const match = getModelMetaByName(seg.label)
        return { label: seg.label, value: seg.value, color: match?.color || defaultModelColors[idx % defaultModelColors.length] }
      })
      if (segments.length === 0) segments.push({ label: '无数据', value: 0, color: '#3f3f46' })
      return { date: day.label || day.date, total: day.total, segments }
    })
  })

  const totalCallsInRange = computed(() => trendData.value.reduce((sum, d) => sum + d.total, 0))
  const trendMaxY = computed(() => Math.max(1, ...trendData.value.map((day) => day.segments.reduce((sum, seg) => sum + seg.value, 0))))
  const areaSeriesMeta = computed(() => {
    const maxSegments = Math.max(1, ...trendData.value.map((day) => day.segments.length))
    return Array.from({ length: maxSegments }, (_, idx) => ({ color: defaultModelColors[idx % defaultModelColors.length] }))
  })

  const activeDayIndex = ref<number | null>(null)
  const handleChartHover = (e: MouseEvent) => {
    const rect = (e.currentTarget as HTMLElement).getBoundingClientRect()
    const x = e.clientX - rect.left
    const span = Math.max(1, trendData.value.length - 1)
    const index = Math.round((x / rect.width) * span)
    if (index >= 0 && index < trendData.value.length) activeDayIndex.value = index
  }

  const areaPaths = computed(() => {
    const width = 700
    const height = 200
    const count = trendData.value.length
    const step = count > 1 ? width / (count - 1) : 0
    const maxY = trendMaxY.value

    return areaSeriesMeta.value.map((meta, mIdx) => {
      const points: { x: number; y: number }[] = []
      const bottomPoints: { x: number; y: number }[] = []
      trendData.value.forEach((day, i) => {
        let currentStackY = 0
        for (let j = 0; j <= mIdx; j++) currentStackY += day.segments[j]?.value || 0
        const prevStackY = currentStackY - (day.segments[mIdx]?.value || 0)
        points.push({ x: i * step, y: height - (currentStackY / maxY * height) })
        bottomPoints.unshift({ x: i * step, y: height - (prevStackY / maxY * height) })
      })
      const lineD = getSmoothPath(points)
      const d = lineD + ` L ${bottomPoints[0].x} ${bottomPoints[0].y} ` + getSmoothPath(bottomPoints, true) + ' Z'
      return { color: meta.color, d, lineD }
    })
  })

  const activeDayPoints = computed(() => {
    if (activeDayIndex.value === null) return []
    const height = 200
    const maxY = trendMaxY.value
    const day = trendData.value[activeDayIndex.value]
    let currentStackY = 0
    return day.segments.map((seg) => {
      currentStackY += seg.value
      return { y: height - (currentStackY / maxY * height), color: seg.color }
    })
  })

  const todayCalls = computed(() => options.adminStatistics.value?.summary?.today_calls || 0)
  const todaySuccess = computed(() => options.adminStatistics.value?.summary?.today_success || 0)
  const todayFailed = computed(() => options.adminStatistics.value?.summary?.today_failed || 0)
  const totalCalls = computed(() => options.adminStatistics.value?.summary?.total_calls || 0)
  const yesterdayCalls = computed(() => options.adminStatistics.value?.summary?.yesterday_calls || 0)
  const yesterdaySuccess = computed(() => options.adminStatistics.value?.summary?.yesterday_success || 0)
  const yesterdayFailed = computed(() => options.adminStatistics.value?.summary?.yesterday_failed || 0)

  const todayCallsFoot = computed(() => buildDeltaFoot(todayCalls.value, yesterdayCalls.value, '昨日'))
  const todaySuccessFoot = computed(() => buildDeltaFoot(todaySuccess.value, yesterdaySuccess.value, '昨日'))
  const todayFailedFoot = computed(() => buildDeltaFoot(todayFailed.value, yesterdayFailed.value, '昨日'))
  const totalCallsFoot = computed(() => (options.adminStatistics.value ? `本月 ${options.adminStatistics.value.summary?.this_month_calls || 0}` : '无数据'))

  const modelRanking = computed(() => {
    const data = options.adminStatistics.value?.model_ranking || []
    return data.slice(0, 5).map((item, idx) => {
      const modelName = item.name === 'Unknown' ? '未识别模型' : item.name
      const match = getModelMetaByName(modelName)
      return {
        name: modelName,
        provider: match?.label || '未配置渠道',
        value: item.value,
        percent: item.percent,
        color: match?.color || defaultModelColors[idx % defaultModelColors.length],
      }
    })
  })

  const modelSettingCandidates = computed(() => {
    const result: Array<{ name: string }> = []
    const seen = new Set<string>()
    configuredModelNames.value.forEach((name) => {
      const key = normalizeModelName(name)
      if (!key || seen.has(key)) return
      seen.add(key)
      result.push({ name })
    })
    modelRanking.value.forEach((item) => {
      const key = normalizeModelName(item.name)
      if (!key || seen.has(key)) return
      seen.add(key)
      result.push({ name: item.name })
    })
    return result
  })

  const chartSegments = computed(() => {
    const perimeter = 2 * Math.PI * 40
    let currentOffset = 0
    return modelRanking.value.map((item) => {
      const dashArray = `${(item.percent * perimeter) / 100} ${perimeter}`
      const dashOffset = -(currentOffset * perimeter) / 100
      currentOffset += item.percent
      return { ...item, dashArray, dashOffset }
    })
  })

  const healthGridFlat = computed(() => {
    const totalCells = HEALTH_COLS * HEALTH_ROWS
    const rows = options.adminStatistics.value?.health_cells || []
    if (rows.length >= totalCells) return rows.slice(0, totalCells)
    return rows.concat(Array.from({ length: totalCells - rows.length }, () => ({ status: 0, success: 0, fail: 0, total: 0 })))
  })

  const userRank = computed(() => (options.adminStatistics.value?.user_rank || []).slice(0, 6))
  const latencyList = computed(() => {
    const rows = (options.adminStatistics.value?.latency || [])
      .map((item, idx) => {
        const match = getModelMetaByName(item.name)
        return { name: item.name, val: item.avg_seconds, idx, color: match?.color || defaultModelColors[idx % defaultModelColors.length] || 'var(--admin-warning)' }
      })
      .slice(0, 5)
    const maxVal = Math.max(1, ...rows.map((r) => r.val))
    return rows.map((r) => ({
      name: r.name,
      val: r.val.toFixed(2),
      percent: Math.max(5, Math.round((r.val / maxVal) * 100)),
      color: r.color,
    }))
  })

  const taskSampleTotal = computed(() => {
    const s = options.adminStatistics.value?.summary
    if (!s) return 0
    return (s.pending || 0) + (s.running || 0) + (s.completed || 0) + (s.failed || 0)
  })
  const completedSampleTotal = computed(() => options.adminStatistics.value?.summary?.completed || 0)
  const failedSampleTotal = computed(() => options.adminStatistics.value?.summary?.failed || 0)
  const runningSampleTotal = computed(() => options.adminStatistics.value?.summary?.running || 0)
  const healthRate = computed(() => (taskSampleTotal.value === 0 ? '0.0%' : `${((completedSampleTotal.value / taskSampleTotal.value) * 100).toFixed(1)}%`))
  const failureRate = computed(() => (taskSampleTotal.value === 0 ? '0%' : `${Math.round((failedSampleTotal.value / taskSampleTotal.value) * 100)}%`))
  const runningRate = computed(() => (taskSampleTotal.value === 0 ? '0%' : `${Math.round((runningSampleTotal.value / taskSampleTotal.value) * 100)}%`))
  const successRate = computed(() => (taskSampleTotal.value === 0 ? '0%' : `${Math.round((completedSampleTotal.value / taskSampleTotal.value) * 100)}%`))

  const monitorQueueTotal = computed(() => (options.monitorData.value?.global_queue_len || 0) + (options.monitorData.value?.wait_queue_len || 0))
  const monitorRunningTotal = computed(() => options.monitorData.value?.running_tasks || 0)
  const monitorQueueText = computed(() => `${options.monitorData.value?.global_queue_len || 0}/${Math.max(1, monitorQueueTotal.value)}`)
  const monitorRunningText = computed(() => `${monitorRunningTotal.value}/${Math.max(1, monitorQueueTotal.value + monitorRunningTotal.value)}`)
  const monitorQueuePercent = computed(() => Math.min(100, Math.round(((options.monitorData.value?.global_queue_len || 0) / Math.max(1, monitorQueueTotal.value)) * 100)))
  const monitorRunningPercent = computed(() => Math.min(100, Math.round((monitorRunningTotal.value / Math.max(1, monitorQueueTotal.value + monitorRunningTotal.value)) * 100)))

  const accountLoadRows = computed(() => {
    const loads = options.monitorData.value?.account_loads || {}
    return Object.entries(loads)
      .map(([id, val]) => {
        const current = val.current_concurrent || 0
        const max = Math.max(1, val.max_concurrent || 0)
        const percent = Math.min(100, Math.round((current / max) * 100))
        const color = percent >= 90 ? 'var(--admin-danger)' : percent >= 70 ? 'var(--admin-warning)' : 'var(--admin-success)'
        return { id, current, max, percent, color }
      })
      .sort((a, b) => b.percent - a.percent)
      .slice(0, 3)
  })
  const saturatedAccountCount = computed(() => accountLoadRows.value.filter((item) => item.percent >= 90).length)

  function calculatePercent(cell: HealthCellLike) {
    if (cell.total === 0) return '0.0'
    return ((cell.success / cell.total) * 100).toFixed(1)
  }

  function getCellTimeRange(index: number) {
    const rangeLabel = options.timeRange.value === '7d' ? '最近7天' : options.timeRange.value === '30d' ? '最近30天' : '最近1年'
    return `${rangeLabel} 样本区间 #${index + 1}`
  }

  function shouldShowLabel(index: number) {
    const count = trendData.value.length
    if (count <= 7) return true
    if (options.timeRange.value === '30d') return index === 0 || index === count - 1 || index % 6 === 0
    if (options.timeRange.value === '1y') {
      if (index === 0 || index === count - 1) return true
      const day = trendData.value[index]
      if (!day) return false
      return isMonthStartLabel(day.date)
    }
    return true
  }

  return {
    areaSeriesMeta,
    trendData,
    totalCallsInRange,
    activeDayIndex,
    handleChartHover,
    areaPaths,
    activeDayPoints,
    todayCalls,
    todaySuccess,
    todayFailed,
    totalCalls,
    todayCallsFoot,
    todaySuccessFoot,
    todayFailedFoot,
    totalCallsFoot,
    modelRanking,
    modelSettingCandidates,
    chartSegments,
    healthGridFlat,
    userRank,
    latencyList,
    healthRate,
    failureRate,
    runningRate,
    successRate,
    monitorQueueText,
    monitorRunningText,
    monitorQueuePercent,
    monitorRunningPercent,
    accountLoadRows,
    saturatedAccountCount,
    calculatePercent,
    getCellTimeRange,
    shouldShowLabel,
  }
}

function getSmoothPath(pts: { x: number; y: number }[], reverse = false) {
  if (pts.length === 0) return ''
  let d = reverse ? '' : `M ${pts[0].x} ${pts[0].y}`
  for (let i = 1; i < pts.length; i++) d += ` L ${pts[i].x} ${pts[i].y}`
  return d
}

function normalizeModelName(name: string | null | undefined): string {
  return (name || '').trim().toLowerCase()
}

function colorForModel(name: string, palette: string[]) {
  let hash = 0
  for (let i = 0; i < name.length; i++) hash = ((hash << 5) - hash) + name.charCodeAt(i)
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
