// Task status filter vocabulary (issue #809, moved under the Admin shared
// boundary by #863). The value set is sourced from the v1 contract
// (`@/api/workspaceTaskContract`): the Control Plane types the status
// filter as a plain string, so this Admin-owned domain module owns the
// runtime map, but the strings must stay identical to the contract — adding
// a contract state fails the `satisfies` check below until the map follows.
// Feature code imports it from here, never generated code, and generic
// `shared` modules must not depend on it.

import type { WorkspaceTaskStatus } from '@/api/tasks/workspaceTaskContract'

export type TaskStatus = WorkspaceTaskStatus

export const TaskStatus = {
  pending: 'pending',
  running: 'running',
  completed: 'completed',
  failed: 'failed',
  cancelled: 'cancelled',
} as const satisfies Record<TaskStatus, TaskStatus>

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
  readonly chartColorRole?: 'info' | 'warning' | 'success' | 'danger' | 'mist'
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
 * chartColorRole describes the semantic series role; chart adapters resolve
 * its concrete color from the current Admin runtime tokens at render time.
 *
 * 过滤 pills 是中性分类控件，不是状态表达：一律中性样式（bg-muted 载体 +
 * muted-foreground 圆点），不蹭 success/warning/info/destructive 状态色；
 * 图表线条仍保留语义色（chartColorRole），与状态 Pill（domain/status）同源。
 */
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
    dotClass: 'bg-muted-foreground',
    activePillClass: 'bg-muted text-foreground font-semibold ring-1 ring-border',
    hasChartLine: true,
    chartColorRole: 'info',
    chartLabel: 'Pending',
    getCount: (stats) => stats.pending,
  },
  {
    key: 'running',
    status: TaskStatus.running,
    label: 'Running',
    pillTitle: 'Filter running tasks',
    dotClass: 'bg-muted-foreground',
    activePillClass: 'bg-muted text-foreground font-semibold ring-1 ring-border',
    hasChartLine: true,
    chartColorRole: 'warning',
    chartLabel: 'Running',
    getCount: (stats) => stats.running,
  },
  {
    key: 'completed',
    status: TaskStatus.completed,
    label: 'Success',
    pillTitle: 'Filter completed tasks',
    dotClass: 'bg-muted-foreground',
    activePillClass: 'bg-muted text-foreground font-semibold ring-1 ring-border',
    hasChartLine: true,
    chartColorRole: 'success',
    chartLabel: 'Success',
    getCount: (stats) => stats.completed,
  },
  {
    key: 'failed',
    status: TaskStatus.failed,
    label: 'Errors',
    pillTitle: 'Filter failed tasks',
    dotClass: 'bg-muted-foreground',
    activePillClass: 'bg-muted text-foreground font-semibold ring-1 ring-border',
    hasChartLine: true,
    chartColorRole: 'danger',
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
    chartColorRole: 'mist',
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
