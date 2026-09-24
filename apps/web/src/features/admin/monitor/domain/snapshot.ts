// Monitor snapshot view types: feature-owned copies of the monitor read
// model served by the Control Plane (GET /api/control/v1/platform/monitor).
// The backend serves the same tasking read model as before; only the
// transport moved, so views keep working without generated legacy model
// code. MonitorStats doubles as the list-level status rollup (the control
// task list is cursor-paginated without totals).
const MonitorExecutionPressureKey = {
  fast_completed: 'fast_completed',
  slow_completed: 'slow_completed',
  normal_pending: 'normal_pending',
  backlog_pending: 'backlog_pending',
  normal_running: 'normal_running',
  stuck_running: 'stuck_running',
  fast_failed: 'fast_failed',
  slow_failed: 'slow_failed',
  cancelled: 'cancelled',
  retrying: 'retrying',
  other: 'other',
} as const

export type MonitorExecutionPressureKey =
  (typeof MonitorExecutionPressureKey)[keyof typeof MonitorExecutionPressureKey]

export type MonitorLatencySignal = {
  samples: number
  avg_ms: number
  p95_ms: number
  max_ms: number
}

export type MonitorExecutionPressurePoint = {
  key: MonitorExecutionPressureKey
  label: string
  count: number
  share_percent: number
  duration_pressure: number
  anomaly_pressure: number
  avg_duration_seconds: number
  max_duration_seconds: number
  failed_count: number
  retry_count: number
}

export type MonitorGoldenSignals = {
  window_seconds: number
  sample_count: number
  provider_attempts: number
  provider_error_count: number
  provider_error_rate: number
  queue_delay_ms: MonitorLatencySignal
  provider_latency_ms: MonitorLatencySignal
  finalize_latency_ms: MonitorLatencySignal
}

export type MonitorData = {
  global_queue_len: number
  wait_queue_len: number
  running_attempts: number
  execution_pressure: MonitorExecutionPressurePoint[]
  golden_signals: MonitorGoldenSignals
}

export type MonitorStats = {
  pending: number
  running: number
  completed: number
  failed: number
  cancelled: number
}

export type MonitorResponse = {
  monitor: MonitorData
  stats: MonitorStats
}
