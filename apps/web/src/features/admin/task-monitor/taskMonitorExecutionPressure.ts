import {
  MonitorExecutionPressureKey,
  type MonitorExecutionPressureKey as MonitorExecutionPressureKeyValue,
  type MonitorExecutionPressurePoint,
} from '../../../shared/api/generated-query/model'

export type TaskExecutionSignalKey = MonitorExecutionPressureKeyValue

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
  { key: MonitorExecutionPressureKey.fast_completed, label: '快速完成' },
  { key: MonitorExecutionPressureKey.slow_completed, label: '慢速完成' },
  { key: MonitorExecutionPressureKey.normal_pending, label: '正常排队' },
  { key: MonitorExecutionPressureKey.backlog_pending, label: '积压排队' },
  { key: MonitorExecutionPressureKey.normal_running, label: '正常执行' },
  { key: MonitorExecutionPressureKey.stuck_running, label: '卡住执行' },
  { key: MonitorExecutionPressureKey.fast_failed, label: '快速失败' },
  { key: MonitorExecutionPressureKey.slow_failed, label: '慢速失败' },
  { key: MonitorExecutionPressureKey.cancelled, label: '已取消' },
  { key: MonitorExecutionPressureKey.retrying, label: '重试中' },
  { key: MonitorExecutionPressureKey.other, label: '其他状态' },
]

export function normalizeMonitorExecutionPressurePoints(
  points: MonitorExecutionPressurePoint[],
): TaskExecutionPressurePoint[] {
  const pointsByKey = new Map(points.map((point) => [point.key, point]))
  return TASK_EXECUTION_LIFECYCLE_BUCKETS.map((bucket) => {
    const point = pointsByKey.get(bucket.key)
    return {
      key: bucket.key,
      label: point?.label || bucket.label,
      count: Number(point?.count ?? 0),
      sharePercent: Number(point?.share_percent ?? 0),
      durationPressure: Number(point?.duration_pressure ?? 0),
      anomalyPressure: Number(point?.anomaly_pressure ?? 0),
      avgDurationSeconds: Number(point?.avg_duration_seconds ?? 0),
      maxDurationSeconds: Number(point?.max_duration_seconds ?? 0),
      failedCount: Number(point?.failed_count ?? 0),
      retryCount: Number(point?.retry_count ?? 0),
    }
  })
}
