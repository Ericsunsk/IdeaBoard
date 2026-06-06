import { computed } from 'vue'
import type {
  AdminStatistics,
  AdminStatisticsBottleneckSourcesItem,
  AdminStatisticsFailureCategoriesItem,
  AdminStatisticsModelHealthItem,
  AdminStatisticsQualityTrendItem,
  AdminStatisticsSlowModelsItem,
  AdminStatisticsTrendItem,
  MonitorData,
  MonitorStats,
  Task,
  TaskExecutionMetrics,
} from '../../../../shared/api/generated-query/model'
import { buildAdminTimeRangeBuckets } from './adminTimeRange'
import type { AdminStatisticsRange } from '../queries/adminActions'

const mockModels = [
  'gpt-image-1',
  'gemini-2.5-flash-image',
  'seedream-4.0',
  'flux-kontext-pro',
] as const

export function useAdminMockMode() {
  return computed(() => import.meta.env.VITE_ADMIN_MOCK_DATA === '1')
}

export function createMockAdminStatistics(range: AdminStatisticsRange): AdminStatistics {
  const trend = createMockTrend(range)
  const qualityTrend = createMockQualityTrend(range)

  return {
    summary: {
      total_calls: 128420,
      today_calls: 248,
      today_success: 222,
      today_failed: 26,
      yesterday_calls: 214,
      yesterday_success: 196,
      yesterday_failed: 18,
      this_month_calls: 18492,
      pending: 42,
      running: 18,
      completed: 120846,
      failed: 7532,
    },
    trend,
    model_health: createMockModelHealth(),
    quality_trend: qualityTrend,
    slow_models: createMockSlowModels(),
    bottleneck_sources: createMockBottleneckSources(),
    failure_categories: createMockFailureCategories(),
    backfill_status: {
      range_start: trend[0]?.date || '',
      range_end: trend[trend.length - 1]?.date || '',
      missing_days: 0,
      backfilled_days: 0,
    },
  }
}

export function createMockAdminTasks(now = new Date()): Task[] {
  return [
    createMockTask(1001, 'completed', 960, 34, 0, 'gpt-image-1', now),
    createMockTask(1002, 'completed', 1320, 118, 0, 'gemini-2.5-flash-image', now),
    createMockTask(1003, 'pending', 28, null, 0, 'seedream-4.0', now),
    createMockTask(1004, 'pending', 92, null, 0, 'flux-kontext-pro', now),
    createMockTask(1005, 'running', 44, null, 0, 'gpt-image-1', now),
    createMockTask(1006, 'running', 186, null, 0, 'gemini-2.5-flash-image', now),
    createMockTask(1007, 'failed', 780, 22, 0, 'seedream-4.0', now),
    createMockTask(1008, 'failed', 1440, 152, 0, 'flux-kontext-pro', now),
    createMockTask(1009, 'running', 126, null, 2, 'gpt-image-1', now),
    createMockTask(1010, 'pending', 74, null, 1, 'gemini-2.5-flash-image', now),
    createMockTask(1011, 'cancelled', 360, null, 0, 'seedream-4.0', now),
    createMockTask(1012, 'completed', 420, 48, 0, 'flux-kontext-pro', now),
  ]
}

export const mockMonitorData: MonitorData = {
  global_queue_len: 31,
  wait_queue_len: 11,
  running_tasks: 18,
  account_loads: {
    101: { current_concurrent: 8, max_concurrent: 10 },
    102: { current_concurrent: 5, max_concurrent: 8 },
    103: { current_concurrent: 3, max_concurrent: 6 },
    104: { current_concurrent: 7, max_concurrent: 12 },
    105: { current_concurrent: 1, max_concurrent: 5 },
  },
}

export const mockMonitorStats: MonitorStats = {
  pending: 42,
  running: 18,
  completed: 120846,
  failed: 7532,
}

function createMockTrend(range: AdminStatisticsRange): AdminStatisticsTrendItem[] {
  return buildAdminTimeRangeBuckets(range).map((bucket, index) => {
    const segments = mockModels.slice(0, 3).map((model, modelIndex) => ({
      label: model,
      value: mockWaveValue(index, 18 - modelIndex * 3, 9 + modelIndex * 2),
    }))
    const total = segments.reduce((sum, segment) => sum + segment.value, 0)

    return {
      date: bucket.date,
      label: bucket.label,
      total,
      segments,
    }
  })
}

function createMockQualityTrend(range: AdminStatisticsRange): AdminStatisticsQualityTrendItem[] {
  return buildAdminTimeRangeBuckets(range).map((bucket, index) => {
    const total = mockWaveValue(index, 42, 18)
    const failureRate = Math.min(24, Math.max(4, 8 + Math.sin(index * 0.9) * 7 + (index % 5)))
    const fail = Math.round((total * failureRate) / 100)
    const success = Math.max(0, total - fail)

    return {
      date: bucket.date,
      label: bucket.label,
      total,
      success,
      fail,
      success_rate: total > 0 ? (success / total) * 100 : 0,
    }
  })
}

function createMockModelHealth(): AdminStatisticsModelHealthItem[] {
  return [
    { name: 'gpt-image-1', total: 4820, completed: 4560, failed: 260, failure_rate: 5.4, avg_seconds: 21.6 },
    {
      name: 'gemini-2.5-flash-image',
      total: 3160,
      completed: 2874,
      failed: 286,
      failure_rate: 9.1,
      avg_seconds: 34.2,
    },
    { name: 'seedream-4.0', total: 2118, completed: 1890, failed: 228, failure_rate: 10.8, avg_seconds: 18.9 },
    {
      name: 'flux-kontext-pro',
      total: 1380,
      completed: 1172,
      failed: 208,
      failure_rate: 15.1,
      avg_seconds: 46.8,
    },
    { name: 'imagen-4-preview', total: 820, completed: 662, failed: 158, failure_rate: 19.3, avg_seconds: 58.5 },
  ]
}

function createMockSlowModels(): AdminStatisticsSlowModelsItem[] {
  return [
    { name: 'imagen-4-preview', avg_seconds: 58.5, p95_seconds: 94.2, samples: 820 },
    { name: 'flux-kontext-pro', avg_seconds: 46.8, p95_seconds: 78.4, samples: 1380 },
    { name: 'gemini-2.5-flash-image', avg_seconds: 34.2, p95_seconds: 61.7, samples: 3160 },
    { name: 'gpt-image-1', avg_seconds: 21.6, p95_seconds: 39.5, samples: 4820 },
  ]
}

function createMockBottleneckSources(): AdminStatisticsBottleneckSourcesItem[] {
  return [
    { source: 'provider', label: 'Provider', total: 428, failed: 78, avg_latency_ms: 43800 },
    { source: 'queue', label: 'Queue', total: 312, failed: 22, avg_latency_ms: 18600 },
    { source: 'storage', label: 'Storage', total: 164, failed: 18, avg_latency_ms: 9400 },
    { source: 'worker', label: 'Worker', total: 126, failed: 9, avg_latency_ms: 7200 },
    { source: 'finalize', label: 'Finalize', total: 88, failed: 6, avg_latency_ms: 5100 },
    { source: 'input', label: 'Input', total: 54, failed: 41, avg_latency_ms: 1200 },
  ]
}

function createMockFailureCategories(): AdminStatisticsFailureCategoriesItem[] {
  return [
    { category: 'provider_error', label: 'Provider Error', total: 182, avg_latency_ms: 42600 },
    { category: 'timeout', label: 'Timeout', total: 96, avg_latency_ms: 71400 },
    { category: 'quota', label: 'Quota', total: 58, avg_latency_ms: 2200 },
    { category: 'storage_error', label: 'Storage Error', total: 42, avg_latency_ms: 9800 },
    { category: 'input_invalid', label: 'Input Invalid', total: 36, avg_latency_ms: 900 },
    { category: 'finalize_error', label: 'Finalize Error', total: 24, avg_latency_ms: 6200 },
  ]
}

function createMockTask(
  id: number,
  status: string,
  createdAgoSeconds: number,
  durationSeconds: number | null,
  retryCount: number,
  model: string,
  now: Date,
): Task {
  const createdAt = new Date(now.getTime() - createdAgoSeconds * 1000)
  const completedAt =
    durationSeconds == null ? null : new Date(createdAt.getTime() + durationSeconds * 1000)
  const taskId = `mock-task-${id}`

  return {
    id,
    task_id: taskId,
    user_id: 9000 + (id % 5),
    channel_id: 20 + (id % 4),
    account_id: 100 + (id % 5),
    prompt: `Mock ${model} visual regression sample ${id}`,
    params: JSON.stringify({ model, mock: true, aspect_ratio: id % 2 === 0 ? '16:9' : '1:1' }),
    status,
    retry_count: retryCount,
    error_msg: status === 'failed' ? mockFailureMessage(id) : '',
    result: status === 'completed' ? JSON.stringify({ model, mock: true }) : '',
    outputs: [],
    execution_metrics: createMockExecutionMetrics(taskId, status, createdAt, completedAt, id),
    created_at: createdAt.toISOString(),
    updated_at: (completedAt || now).toISOString(),
    completed_at: completedAt?.toISOString() || null,
  }
}

function createMockExecutionMetrics(
  taskId: string,
  status: string,
  createdAt: Date,
  completedAt: Date | null,
  id: number,
): TaskExecutionMetrics {
  const totalLatency = completedAt ? completedAt.getTime() - createdAt.getTime() : 0
  const isFailed = status === 'failed'

  return {
    task_id: taskId,
    current_stage: status === 'completed' ? 'completed' : isFailed ? 'failed' : 'provider',
    bottleneck_source: id % 3 === 0 ? 'queue' : id % 3 === 1 ? 'provider' : 'storage',
    failure_stage: isFailed ? 'provider' : '',
    failure_category: isFailed ? (id % 2 === 0 ? 'timeout' : 'provider_error') : '',
    assigned_at: new Date(createdAt.getTime() + 4000).toISOString(),
    provider_started_at: new Date(createdAt.getTime() + 8000).toISOString(),
    provider_finished_at: completedAt?.toISOString() || null,
    finalize_started_at: completedAt ? new Date(completedAt.getTime() - 4000).toISOString() : null,
    finalize_finished_at: completedAt?.toISOString() || null,
    queue_wait_ms: 4000 + id * 300,
    worker_overhead_ms: 1600 + id * 120,
    provider_latency_ms: Math.max(0, totalLatency - 5600),
    finalize_latency_ms: completedAt ? 4000 : 0,
    total_latency_ms: Math.max(0, totalLatency),
    created_at: createdAt.toISOString(),
    updated_at: (completedAt || new Date()).toISOString(),
  }
}

function mockWaveValue(index: number, base: number, amplitude: number) {
  const wave = Math.sin(index * 0.72) * amplitude
  const lift = (index % 4) * 3
  return Math.max(1, Math.round(base + wave + lift))
}

function mockFailureMessage(id: number) {
  return id % 2 === 0 ? 'Provider timeout after retry window' : 'Provider returned transient error'
}
