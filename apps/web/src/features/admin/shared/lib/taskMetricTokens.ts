// Task status filter vocabulary (issue #809, moved under the Admin shared
// boundary by #863). The legacy generated `TaskStatus` enum is
// gone; the Control Plane types the status filter as a plain string, so this
// Admin-owned domain module owns the frozen value set. Feature code imports
// it from here, never generated code, and generic `shared` modules must not
// depend on it.
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
 * chartColor 与 Admin 主题对齐（Admin shared 内聚，源头见 style.css --color-admin-*）：
 * pending/info, running/warning, completed/success, failed/danger,
 * cancelled/mist。
 */
export const taskMetricDefinitions: readonly TaskMetricDefinition[] = [
  {
    key: 'all',
    status: null,
    label: 'All',
    pillTitle: 'Show all tasks',
    dotClass: 'bg-admin-text-3',
    activePillClass:
      'bg-admin-surface-muted text-admin-text-1 font-semibold ring-1 ring-admin-border-soft',
    hasChartLine: false,
    getCount: (stats) => stats.total,
  },
  {
    key: 'pending',
    status: TaskStatus.pending,
    label: 'Pending',
    pillTitle: 'Filter pending tasks',
    dotClass: 'bg-admin-info',
    activePillClass: 'bg-admin-info-soft text-admin-info font-semibold ring-1 ring-admin-info-soft',
    hasChartLine: true,
    chartColor: '#4775a8',
    chartLabel: 'Pending',
    getCount: (stats) => stats.pending,
  },
  {
    key: 'running',
    status: TaskStatus.running,
    label: 'Running',
    pillTitle: 'Filter running tasks',
    dotClass: 'bg-admin-warning',
    activePillClass:
      'bg-admin-warning-soft text-admin-warning font-semibold ring-1 ring-admin-warning-soft',
    hasChartLine: true,
    chartColor: '#d48830',
    chartLabel: 'Running',
    getCount: (stats) => stats.running,
  },
  {
    key: 'completed',
    status: TaskStatus.completed,
    label: 'Success',
    pillTitle: 'Filter completed tasks',
    dotClass: 'bg-admin-success',
    activePillClass:
      'bg-admin-success-soft text-admin-success font-semibold ring-1 ring-admin-success-soft',
    hasChartLine: true,
    chartColor: '#2f9e73',
    chartLabel: 'Success',
    getCount: (stats) => stats.completed,
  },
  {
    key: 'failed',
    status: TaskStatus.failed,
    label: 'Errors',
    pillTitle: 'Filter failed tasks',
    dotClass: 'bg-admin-danger',
    activePillClass:
      'bg-admin-danger-soft text-admin-danger font-semibold ring-1 ring-admin-danger-soft',
    hasChartLine: true,
    chartColor: '#d14d5a',
    chartLabel: 'Errors',
    getCount: (stats) => stats.failed,
  },
  {
    key: 'cancelled',
    status: TaskStatus.cancelled,
    label: 'Cancelled',
    pillTitle: 'Filter cancelled tasks',
    dotClass: 'bg-admin-text-3 opacity-60',
    activePillClass:
      'bg-admin-surface-muted text-admin-text-2 font-semibold ring-1 ring-admin-border-soft',
    hasChartLine: true,
    chartColor: '#8f8f8f',
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
