import type {
  AdminStatistics,
  MonitorData,
  MonitorStats,
} from '../../../../shared/api/generated-query/model'

export type AdminAnomalySeverity = 'critical' | 'warning' | 'info'
export type AdminAnomalySource = 'runtime' | 'statistics' | 'account' | 'provider' | 'system'

export type AdminAnomalyItem = {
  id: string
  severity: AdminAnomalySeverity
  source: AdminAnomalySource
  title: string
  summary: string
  metric?: string
  detectedAt: number
  route: string
  actionLabel: string
}

type AdminAnomalyInput = {
  monitorData: MonitorData | null
  monitorStats: MonitorStats | null
  statistics: AdminStatistics | null
  isMonitorDegraded: boolean
  isMonitorUnavailable: boolean
  isStatisticsUnavailable: boolean
  now?: number
}

type AdminAnomalyDraft = Omit<AdminAnomalyItem, 'detectedAt'>

const TASK_MONITOR_ROUTE = '/admin/task-monitor'
const STATISTICS_ROUTE = '/admin/statistics'

const QUEUE_WARNING_THRESHOLD = 30
const QUEUE_CRITICAL_THRESHOLD = 80
const ACCOUNT_LOAD_WARNING_RATIO = 0.9
const MODEL_FAILURE_WARNING_RATE = 10
const MODEL_FAILURE_CRITICAL_RATE = 15
const QUALITY_SUCCESS_WARNING_RATE = 88
const QUALITY_SUCCESS_CRITICAL_RATE = 80
const SLOW_MODEL_P95_WARNING_SECONDS = 60
const BOTTLENECK_SHARE_WARNING_RATE = 55
const FAILURE_CATEGORY_SHARE_WARNING_RATE = 50

export function buildAdminAnomalyItems(input: AdminAnomalyInput): AdminAnomalyItem[] {
  const now = input.now ?? Date.now()
  const drafts: AdminAnomalyDraft[] = []

  pushSystemAnomalies(drafts, input)
  pushRuntimeAnomalies(drafts, input.monitorData, input.monitorStats)
  pushStatisticsAnomalies(drafts, input.statistics)

  return drafts.map((item) => ({ ...item, detectedAt: now }))
}

function pushSystemAnomalies(drafts: AdminAnomalyDraft[], input: AdminAnomalyInput) {
  if (input.isMonitorDegraded) {
    drafts.push({
      id: 'system.monitor-stream-degraded',
      severity: 'warning',
      source: 'system',
      title: '监控流连接异常',
      summary: '实时监控流处于降级状态，数据可能正在回退到轮询。',
      route: TASK_MONITOR_ROUTE,
      actionLabel: '查看任务监控',
    })
  }

  if (input.isMonitorUnavailable) {
    drafts.push({
      id: 'system.monitor-unavailable',
      severity: 'critical',
      source: 'system',
      title: '实时监控不可用',
      summary: '后台未能获取任务运行状态，请检查监控接口。',
      route: TASK_MONITOR_ROUTE,
      actionLabel: '查看任务监控',
    })
  }

  if (input.isStatisticsUnavailable) {
    drafts.push({
      id: 'system.statistics-unavailable',
      severity: 'warning',
      source: 'system',
      title: '统计数据不可用',
      summary: '后台未能获取数据仪表盘统计结果。',
      route: STATISTICS_ROUTE,
      actionLabel: '查看数据仪表盘',
    })
  }
}

function pushRuntimeAnomalies(
  drafts: AdminAnomalyDraft[],
  monitorData: MonitorData | null,
  monitorStats: MonitorStats | null,
) {
  if (!monitorData) return

  const queueTotal =
    Math.max(0, monitorData.global_queue_len || 0) + Math.max(0, monitorData.wait_queue_len || 0)
  if (queueTotal >= QUEUE_WARNING_THRESHOLD) {
    drafts.push({
      id: 'runtime.queue-backlog',
      severity: queueTotal >= QUEUE_CRITICAL_THRESHOLD ? 'critical' : 'warning',
      source: 'runtime',
      title: '队列积压偏高',
      summary: '任务等待队列正在堆积，可能影响生成延迟。',
      metric: `${queueTotal} 个等待任务`,
      route: TASK_MONITOR_ROUTE,
      actionLabel: '查看执行压力',
    })
  }

  const peakAccountLoad = getPeakAccountLoad(monitorData)
  if (peakAccountLoad && peakAccountLoad.ratio >= ACCOUNT_LOAD_WARNING_RATIO) {
    drafts.push({
      id: `account.load-${peakAccountLoad.accountId}`,
      severity: 'warning',
      source: 'account',
      title: '账号并发接近上限',
      summary: `账号 ${peakAccountLoad.accountId} 当前负载接近并发上限。`,
      metric: `${peakAccountLoad.current}/${peakAccountLoad.max}`,
      route: TASK_MONITOR_ROUTE,
      actionLabel: '查看实时并发',
    })
  }

  if (monitorStats) {
    const activeTotal =
      Math.max(0, monitorStats.pending || 0) + Math.max(0, monitorStats.running || 0)
    if (activeTotal >= QUEUE_CRITICAL_THRESHOLD) {
      drafts.push({
        id: 'runtime.active-volume',
        severity: 'warning',
        source: 'runtime',
        title: '活跃任务量偏高',
        summary: '待处理和执行中的任务量较高。',
        metric: `${activeTotal} 个活跃任务`,
        route: TASK_MONITOR_ROUTE,
        actionLabel: '查看任务分布',
      })
    }
  }
}

function pushStatisticsAnomalies(drafts: AdminAnomalyDraft[], statistics: AdminStatistics | null) {
  if (!statistics) return

  const riskyModel = [...statistics.model_health]
    .filter((item) => item.total >= 20)
    .sort((a, b) => b.failure_rate - a.failure_rate)[0]
  if (riskyModel && riskyModel.failure_rate >= MODEL_FAILURE_WARNING_RATE) {
    drafts.push({
      id: `provider.model-failure-${normalizeId(riskyModel.name)}`,
      severity: riskyModel.failure_rate >= MODEL_FAILURE_CRITICAL_RATE ? 'critical' : 'warning',
      source: 'provider',
      title: '模型失败率偏高',
      summary: `${riskyModel.name} 近期失败率高于阈值。`,
      metric: `${riskyModel.failure_rate.toFixed(1)}%`,
      route: STATISTICS_ROUTE,
      actionLabel: '查看模型风险',
    })
  }

  const slowModel = [...statistics.slow_models]
    .filter((item) => item.samples >= 10)
    .sort((a, b) => b.p95_seconds - a.p95_seconds)[0]
  if (slowModel && slowModel.p95_seconds >= SLOW_MODEL_P95_WARNING_SECONDS) {
    drafts.push({
      id: `provider.model-latency-${normalizeId(slowModel.name)}`,
      severity: 'warning',
      source: 'provider',
      title: '模型尾延迟偏高',
      summary: `${slowModel.name} 的 P95 耗时偏高。`,
      metric: `${slowModel.p95_seconds.toFixed(1)}s`,
      route: STATISTICS_ROUTE,
      actionLabel: '查看模型尾延迟',
    })
  }

  const latestQuality = statistics.quality_trend[statistics.quality_trend.length - 1]
  if (
    latestQuality &&
    latestQuality.total >= 10 &&
    latestQuality.success_rate < QUALITY_SUCCESS_WARNING_RATE
  ) {
    drafts.push({
      id: 'statistics.quality-drop',
      severity: latestQuality.success_rate < QUALITY_SUCCESS_CRITICAL_RATE ? 'critical' : 'warning',
      source: 'statistics',
      title: '服务质量下降',
      summary: `${latestQuality.label} 成功率低于预期。`,
      metric: `${latestQuality.success_rate.toFixed(1)}%`,
      route: STATISTICS_ROUTE,
      actionLabel: '查看服务质量',
    })
  }

  const bottleneck = getDominantShare(statistics.bottleneck_sources)
  if (bottleneck && bottleneck.share >= BOTTLENECK_SHARE_WARNING_RATE) {
    drafts.push({
      id: `statistics.bottleneck-${bottleneck.source}`,
      severity: 'warning',
      source: 'statistics',
      title: '瓶颈来源过于集中',
      summary: `${bottleneck.label} 占据主要瓶颈来源。`,
      metric: `${bottleneck.share.toFixed(1)}%`,
      route: STATISTICS_ROUTE,
      actionLabel: '查看瓶颈来源',
    })
  }

  const failureCategory = getDominantShare(statistics.failure_categories)
  if (failureCategory && failureCategory.share >= FAILURE_CATEGORY_SHARE_WARNING_RATE) {
    drafts.push({
      id: `statistics.failure-category-${failureCategory.category}`,
      severity: 'warning',
      source: 'statistics',
      title: '失败类型过于集中',
      summary: `${failureCategory.label} 是当前主要失败类型。`,
      metric: `${failureCategory.share.toFixed(1)}%`,
      route: STATISTICS_ROUTE,
      actionLabel: '查看失败类型',
    })
  }
}

function getPeakAccountLoad(monitorData: MonitorData) {
  return Object.entries(monitorData.account_loads || {})
    .map(([accountId, load]) => {
      const current = Math.max(0, load.current_concurrent || 0)
      const max = Math.max(0, load.max_concurrent || 0)
      return {
        accountId,
        current,
        max,
        ratio: max > 0 ? current / max : 0,
      }
    })
    .sort((a, b) => b.ratio - a.ratio)[0]
}

function getDominantShare<Row extends { label: string; total: number }>(rows: Row[]) {
  const total = rows.reduce((sum, item) => sum + Math.max(0, item.total || 0), 0)
  if (total <= 0) return null
  const dominant = [...rows].sort((a, b) => b.total - a.total)[0]
  if (!dominant) return null
  return {
    ...dominant,
    share: (Math.max(0, dominant.total || 0) / total) * 100,
  }
}

function normalizeId(value: string) {
  return (
    value
      .toLowerCase()
      .replace(/[^a-z0-9]+/g, '-')
      .replace(/^-|-$/g, '') || 'unknown'
  )
}
