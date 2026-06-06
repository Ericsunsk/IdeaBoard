import type { Task } from '../../../../shared/api/task'
import { calcDurationSeconds } from '../../../../shared/utils/dateTime'

export const TASK_EXECUTION_SLOW_SECONDS = 60
const TASK_EXECUTION_STUCK_SECONDS = 120

export type TaskExecutionSignalKey =
  | 'fast_completed'
  | 'slow_completed'
  | 'normal_pending'
  | 'backlog_pending'
  | 'normal_running'
  | 'stuck_running'
  | 'fast_failed'
  | 'slow_failed'
  | 'retrying'
  | 'other'

export type TaskExecutionPressurePoint = {
  key: TaskExecutionSignalKey
  label: string
  count: number
  sharePercent: number
  durationPressure: number
  anomalyPressure: number
  avgDurationSeconds: number
  maxDurationSeconds: number
  failedCount: number
  retryCount: number
}

type TaskExecutionLifecycleDefinition = {
  key: TaskExecutionSignalKey
  label: string
}

const TASK_EXECUTION_LIFECYCLE_BUCKETS: TaskExecutionLifecycleDefinition[] = [
  { key: 'fast_completed', label: '快速完成' },
  { key: 'slow_completed', label: '慢速完成' },
  { key: 'normal_pending', label: '正常排队' },
  { key: 'backlog_pending', label: '积压排队' },
  { key: 'normal_running', label: '正常执行' },
  { key: 'stuck_running', label: '卡住执行' },
  { key: 'fast_failed', label: '快速失败' },
  { key: 'slow_failed', label: '慢速失败' },
  { key: 'retrying', label: '重试中' },
  { key: 'other', label: '其他状态' },
]

export function buildTaskExecutionPressurePoints(tasks: Task[], now = new Date()) {
  const timedTasks = tasks.map((task) => ({
    task,
    durationSeconds: getTaskDurationSeconds(task, now),
  }))

  return TASK_EXECUTION_LIFECYCLE_BUCKETS.map((bucket) => {
    const rows = timedTasks.filter(
      ({ task, durationSeconds }) => resolveTaskLifecycleKey(task, durationSeconds) === bucket.key,
    )
    const durations = rows
      .map(({ durationSeconds }) => durationSeconds)
      .filter((durationSeconds): durationSeconds is number => durationSeconds != null)
    const maxDurationSeconds = durations.length > 0 ? Math.max(...durations) : 0
    const avgDurationSeconds =
      durations.length > 0
        ? durations.reduce((sum, durationSeconds) => sum + durationSeconds, 0) / durations.length
        : 0

    return {
      key: bucket.key,
      label: bucket.label,
      count: rows.length,
      sharePercent: toPercent(rows.length, tasks.length),
      durationPressure: toPercent(maxDurationSeconds, TASK_EXECUTION_SLOW_SECONDS),
      anomalyPressure: getAnomalyPressure(bucket.key, rows),
      avgDurationSeconds,
      maxDurationSeconds,
      failedCount: rows.filter(({ task }) => task.status === 'failed').length,
      retryCount: rows.reduce((sum, { task }) => sum + Number(task.retry_count || 0), 0),
    }
  })
}

function resolveTaskLifecycleKey(
  task: Task,
  durationSeconds: number | null,
): TaskExecutionSignalKey {
  const retryCount = Number(task.retry_count || 0)
  if ((task.status === 'pending' || task.status === 'running') && retryCount > 0) return 'retrying'

  if (task.status === 'completed') {
    return durationSeconds != null && durationSeconds >= TASK_EXECUTION_SLOW_SECONDS
      ? 'slow_completed'
      : 'fast_completed'
  }

  if (task.status === 'failed') {
    return durationSeconds != null && durationSeconds >= TASK_EXECUTION_SLOW_SECONDS
      ? 'slow_failed'
      : 'fast_failed'
  }

  if (task.status === 'pending') {
    return durationSeconds != null && durationSeconds >= TASK_EXECUTION_SLOW_SECONDS
      ? 'backlog_pending'
      : 'normal_pending'
  }

  if (task.status === 'running') {
    return durationSeconds != null && durationSeconds >= TASK_EXECUTION_STUCK_SECONDS
      ? 'stuck_running'
      : 'normal_running'
  }

  return 'other'
}

function getTaskDurationSeconds(task: Task, now: Date) {
  if (!task.created_at) return null
  return calcDurationSeconds(task.created_at, task.completed_at || now)
}

function getAnomalyPressure(
  key: TaskExecutionSignalKey,
  rows: Array<{ task: Task; durationSeconds: number | null }>,
) {
  if (rows.length <= 0) return 0

  const retryBoost = Math.min(
    18,
    rows.reduce((sum, { task }) => sum + Number(task.retry_count || 0), 0) * 6,
  )

  return clampPercent(getLifecycleBasePressure(key) + retryBoost)
}

function getLifecycleBasePressure(key: TaskExecutionSignalKey) {
  const pressureByKey: Record<TaskExecutionSignalKey, number> = {
    fast_completed: 8,
    slow_completed: 28,
    normal_pending: 22,
    backlog_pending: 58,
    normal_running: 32,
    stuck_running: 82,
    fast_failed: 78,
    slow_failed: 94,
    retrying: 68,
    other: 40,
  }
  return pressureByKey[key]
}

function toPercent(value: number, total: number) {
  if (total <= 0) return 0
  return clampPercent((value / total) * 100)
}

function clampPercent(value: number) {
  return Math.min(100, Math.max(0, value))
}
