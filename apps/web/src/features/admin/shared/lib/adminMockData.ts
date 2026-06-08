import { computed } from 'vue'
import type {
  AccountLimits,
  AdminAccount,
  AdminStatistics,
  AdminStatisticsBottleneckSourcesItem,
  AdminStatisticsFailureCategoriesItem,
  AdminStatisticsModelHealthItem,
  AdminStatisticsQualityTrendItem,
  AdminStatisticsSlowModelsItem,
  AdminStatisticsTrendItem,
  Channel,
  MonitorData,
  MonitorStats,
  Task,
  TaskExecutionMetrics,
  GetAdminStatisticsRange,
} from '../../../../shared/api/generated-query/model'
import { buildAdminTimeRangeBuckets } from './adminTimeRange'

const mockModels = [
  'gpt-image-1',
  'gemini-2.5-flash-image',
  'seedream-4.0',
  'flux-kontext-pro',
] as const

export function useAdminMockMode() {
  return computed(() => import.meta.env.VITE_ADMIN_MOCK_DATA === '1')
}

export function createMockAdminStatistics(range: GetAdminStatisticsRange): AdminStatistics {
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

export function createMockAdminChannels(now = new Date()): Channel[] {
  const timestamp = now.toISOString()
  return [
    {
      id: 201,
      type: 'openai-compatible',
      name: 'OpenAI 主线路',
      api_url: 'https://api.openai.com/v1',
      status: 1,
      created_at: timestamp,
      updated_at: timestamp,
    },
    {
      id: 202,
      type: 'google-ai',
      name: 'Google Gemini 官方',
      api_url: 'https://generativelanguage.googleapis.com/v1beta',
      status: 1,
      created_at: timestamp,
      updated_at: timestamp,
    },
    {
      id: 203,
      type: 'openai-compatible',
      name: '火山 Seedream',
      api_url: 'https://ark.cn-beijing.volces.com/api/v3',
      status: 1,
      created_at: timestamp,
      updated_at: timestamp,
    },
    {
      id: 204,
      type: 'openai-compatible',
      name: 'Black Forest Labs',
      api_url: 'https://api.bfl.ai/v1',
      status: 1,
      created_at: timestamp,
      updated_at: timestamp,
    },
    {
      id: 205,
      type: 'openai-compatible',
      name: '备用聚合网关',
      api_url: 'https://gateway.example.com/v1',
      status: 1,
      created_at: timestamp,
      updated_at: timestamp,
    },
    {
      id: 206,
      type: 'google-ai',
      name: 'Google 备用池',
      api_url: 'https://backup-generativelanguage.example.com/v1beta',
      status: 0,
      created_at: timestamp,
      updated_at: timestamp,
    },
  ]
}

export function createMockAdminAccounts(now = new Date()): AdminAccount[] {
  const timestamp = now.toISOString()
  return [
    createMockAdminAccount(
      901,
      201,
      'OpenAI east key',
      'openai-compatible',
      'https://api.openai.com/v1',
      'gpt-image-1',
      10,
      7,
      0,
      1,
      timestamp,
    ),
    createMockAdminAccount(
      902,
      201,
      'OpenAI backup key',
      'openai-compatible',
      'https://api.openai.com/v1',
      'gpt-image-1,imagen-4-preview',
      8,
      2,
      1,
      1,
      timestamp,
      ['imagen-4-preview'],
    ),
    createMockAdminAccount(
      903,
      202,
      'Gemini official key',
      'google-ai',
      'https://generativelanguage.googleapis.com/v1beta',
      'gemini-2.5-flash-image',
      12,
      9,
      0,
      1,
      timestamp,
    ),
    createMockAdminAccount(
      904,
      202,
      'Gemini safety pool',
      'google-ai',
      'https://generativelanguage.googleapis.com/v1beta',
      'gemini-2.5-flash-image,imagen-4-preview',
      6,
      0,
      6,
      0,
      timestamp,
    ),
    createMockAdminAccount(
      905,
      203,
      'Seedream visual pool',
      'openai-compatible',
      'https://ark.cn-beijing.volces.com/api/v3',
      'seedream-4.0',
      10,
      4,
      0,
      1,
      timestamp,
    ),
    createMockAdminAccount(
      906,
      203,
      'Seedream burst pool',
      'openai-compatible',
      'https://ark.cn-beijing.volces.com/api/v3',
      'seedream-4.0,seedream-4.0-edit',
      8,
      6,
      2,
      1,
      timestamp,
      ['seedream-4.0-edit'],
    ),
    createMockAdminAccount(
      907,
      204,
      'Flux Kontext pro',
      'openai-compatible',
      'https://api.bfl.ai/v1',
      'flux-kontext-pro',
      6,
      5,
      1,
      1,
      timestamp,
    ),
    createMockAdminAccount(
      908,
      204,
      'Flux Kontext failover',
      'openai-compatible',
      'https://api.bfl.ai/v1',
      'flux-kontext-max,flux-kontext-pro',
      6,
      1,
      0,
      1,
      timestamp,
    ),
    createMockAdminAccount(
      909,
      205,
      'Gateway mixed pool',
      'openai-compatible',
      'https://gateway.example.com/v1',
      'gpt-image-1,gemini-2.5-flash-image,seedream-4.0',
      16,
      11,
      3,
      1,
      timestamp,
      ['gemini-2.5-flash-image'],
    ),
    createMockAdminAccount(
      910,
      205,
      'Gateway cold standby',
      'openai-compatible',
      'https://gateway.example.com/v1',
      'flux-kontext-pro,imagen-4-preview',
      8,
      0,
      0,
      0,
      timestamp,
    ),
    createMockAdminAccount(
      911,
      206,
      'Gemini spare key',
      'google-ai',
      'https://backup-generativelanguage.example.com/v1beta',
      'gemini-2.5-flash-image',
      5,
      0,
      7,
      0,
      timestamp,
    ),
  ]
}

export const mockAdminAccountLimits: AccountLimits = {
  max_concurrent: { min: 1, max: 64, default: 8 },
  timeout_seconds: { min: 5, max: 600, default: 180 },
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

function createMockAdminAccount(
  id: number,
  channelId: number,
  name: string,
  apiType: string,
  proxyUrl: string,
  modelSet: string,
  maxConcurrent: number,
  currentConcurrent: number,
  failCount: number,
  status: number,
  timestamp: string,
  disabledModels: string[] = [],
): AdminAccount {
  return {
    id,
    name,
    channel_id: channelId,
    platform: apiType,
    api_type: apiType,
    proxy_url: proxyUrl,
    model_set: modelSet,
    max_concurrent: maxConcurrent,
    current_concurrent: currentConcurrent,
    wait_queue_count: Math.max(0, failCount - 1),
    running_status: status === 1 ? 'running' : 'stopped',
    fail_count: failCount,
    weight: status === 1 ? 100 : 40,
    timeout: 180,
    extra_config:
      disabledModels.length > 0 ? JSON.stringify({ disabled_models: disabledModels }) : '{}',
    status,
    created_at: timestamp,
    updated_at: timestamp,
  }
}

export const mockMonitorStats: MonitorStats = {
  pending: 42,
  running: 18,
  completed: 120846,
  failed: 7532,
}

function createMockTrend(range: GetAdminStatisticsRange): AdminStatisticsTrendItem[] {
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

function createMockQualityTrend(range: GetAdminStatisticsRange): AdminStatisticsQualityTrendItem[] {
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
    {
      name: 'gpt-image-1',
      total: 4820,
      completed: 4560,
      failed: 260,
      failure_rate: 5.4,
      avg_seconds: 21.6,
    },
    {
      name: 'gemini-2.5-flash-image',
      total: 3160,
      completed: 2874,
      failed: 286,
      failure_rate: 9.1,
      avg_seconds: 34.2,
    },
    {
      name: 'seedream-4.0',
      total: 2118,
      completed: 1890,
      failed: 228,
      failure_rate: 10.8,
      avg_seconds: 18.9,
    },
    {
      name: 'flux-kontext-pro',
      total: 1380,
      completed: 1172,
      failed: 208,
      failure_rate: 15.1,
      avg_seconds: 46.8,
    },
    {
      name: 'imagen-4-preview',
      total: 820,
      completed: 662,
      failed: 158,
      failure_rate: 19.3,
      avg_seconds: 58.5,
    },
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
