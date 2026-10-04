// Task status filter vocabulary (issue #809, moved under the Admin shared
// boundary by #863). The legacy generated `TaskStatus` enum is
// gone; the Control Plane types the status filter as a plain string, so this
// Admin-owned domain module owns the frozen value set. Feature code imports
// it from here, never generated code, and generic `shared` modules must not
// depend on it.
import { adminThemeTokenFallbacks } from './adminThemeTokens'

export const TaskStatus = {
  pending: 'pending',
  running: 'running',
  completed: 'completed',
  failed: 'failed',
  cancelled: 'cancelled',
} as const

export type TaskStatus = (typeof TaskStatus)[keyof typeof TaskStatus]

export type TaskMetricKey = 'all' | 'pending' | 'running' | 'completed' | 'failed' | 'cancelled'

export interface TaskMetricStatsSnapshot {
  total: number
  pending: number
  running: number
  completed: number
  failed: number
  cancelled: number
}

export interface TaskMetricDefinition {
  readonly key: TaskMetricKey
  readonly status: TaskStatus | null
  readonly label: string
  readonly pillTitle: string
  readonly dotClass: string
  readonly activePillClass: string
  readonly hasChartLine: boolean
  readonly chartColor?: string
  readonly chartLabel?: string
  readonly getCount: (stats: TaskMetricStatsSnapshot) => number
}

/**
 * 任务监控图表视觉规范 (Chart Visual Tokens)
 */
export const monitorChartSpecs = {
  heightPx: 180,
  lineWidth: 1,
  lineTension: 0.35,
  fillOpacity: 0.12,
  pointRadius: 0,
  pointHoverRadius: 2,
  pointHitRadius: 10,
} as const

/**
 * 任务指标元定义与视觉 Token (Single Source of Truth)
 *
 * chartColor 与 Admin 主题对齐（Admin shared 内聚，源头见 style.css --color-admin-*）：
 * pending/info, running/warning, completed/success, failed/danger, cancelled/mist。
 * Chart.js 绘制 canvas 需要具体色值，withAlpha 也无法处理 var()，因此这里取
 * adminThemeTokenFallbacks 里的 fallback，而不是就地再写一份 hex。
 */
const { chartMist, danger, info, success, warning } = adminThemeTokenFallbacks

export const taskMetricDefinitions: readonly TaskMetricDefinition[] = [
  {
    key: 'all',
    status: null,
    label: 'All',
    pillTitle: 'Show all tasks',
    dotClass: 'bg-muted-foreground',
    activePillClass: 'bg-muted text-foreground font-semibold ring-1 ring-border',
    hasChartLine: false,
    getCount: (stats) => stats.total,
  },
  {
    key: 'pending',
    status: TaskStatus.pending,
    label: 'Pending',
    pillTitle: 'Filter pending tasks',
    dotClass: 'bg-info',
    activePillClass: 'bg-info-soft text-info font-semibold ring-1 ring-info/20',
    hasChartLine: true,
    chartColor: info,
    chartLabel: 'Pending',
    getCount: (stats) => stats.pending,
  },
  {
    key: 'running',
    status: TaskStatus.running,
    label: 'Running',
    pillTitle: 'Filter running tasks',
    dotClass: 'bg-warning',
    activePillClass: 'bg-warning-soft text-warning font-semibold ring-1 ring-warning/20',
    hasChartLine: true,
    chartColor: warning,
    chartLabel: 'Running',
    getCount: (stats) => stats.running,
  },
  {
    key: 'completed',
    status: TaskStatus.completed,
    label: 'Success',
    pillTitle: 'Filter completed tasks',
    dotClass: 'bg-success',
    activePillClass: 'bg-success-soft text-success font-semibold ring-1 ring-success/20',
    hasChartLine: true,
    chartColor: success,
    chartLabel: 'Success',
    getCount: (stats) => stats.completed,
  },
  {
    key: 'failed',
    status: TaskStatus.failed,
    label: 'Errors',
    pillTitle: 'Filter failed tasks',
    dotClass: 'bg-destructive',
    activePillClass: 'bg-destructive/15 text-destructive font-semibold ring-1 ring-destructive/20',
    hasChartLine: true,
    chartColor: danger,
    chartLabel: 'Errors',
    getCount: (stats) => stats.failed,
  },
  {
    key: 'cancelled',
    status: TaskStatus.cancelled,
    label: 'Cancelled',
    pillTitle: 'Filter cancelled tasks',
    dotClass: 'bg-muted-foreground opacity-60',
    activePillClass: 'bg-muted text-muted-foreground font-semibold ring-1 ring-border',
    hasChartLine: true,
    chartColor: chartMist,
    chartLabel: 'Cancelled',
    getCount: (stats) => stats.cancelled,
  },
]

export function getTaskMetricByStatus(
  status: string | null | undefined,
): TaskMetricDefinition | undefined {
  if (!status) return undefined
  return taskMetricDefinitions.find((item) => item.status === status)
}
