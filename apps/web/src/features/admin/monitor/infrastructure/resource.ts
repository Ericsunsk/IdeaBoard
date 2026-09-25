import { useQuery } from '@tanstack/vue-query'
import { toValue, type MaybeRef, type MaybeRefOrGetter } from 'vue'
import { api } from '../../../../shared/api/client'
import { CONTROL_API_PREFIX } from '../../../../shared/api/controlMutator'
import {
  createApiDataSelector,
  resolveApiData,
  type ApiResponseWithData,
} from '../../../../shared/api/apiResponse'
import { getGetControlMonitorUrl } from '../../../../shared/api/generated-query/control/control-platform/control-platform'
import { taskCancellationIdempotencyKey } from '../../../../shared/api/idempotencyKey'
import type { ListAdminTasksStatus } from '../../../../shared/domain/taskMetricTokens'

// Admin monitor/task boundary (final cut, partial). The runtime snapshot
// moved to the Control Plane (`GET /api/control/v1/platform/monitor`, a
// guarded proxy keeping the legacy envelope), so `useAdminMonitorResource`
// repoints at the generated Control path with view types untouched. The
// `/api/admin/tasks*` list/detail/cancel transports below stay until the
// native Control task dialect (cursor pagination, operator-reason cancel)
// is adopted by the admin UI — see the blocked list.
export type ListAdminTasksParams = {
  status?: ListAdminTasksStatus
  user_public_id?: string
  account_id?: number
  keyword?: string
  page?: number
  page_size?: number
}

export type AdminMonitorExecutionSnapshot = {
  resolved_model: string
  provider: string
  gateway_step: string
}

export type AdminTaskBillingSnapshot = {
  sku: string
  output_resolution: string
  display_name: string
  price_version_id: number
  price_version: number
  unit_credits: number
  quantity: number
  total_credits: number
  captured_credits: number
  quoted_at: string
  settlement_state: string
  settled_at: string | null
  feature_exists: boolean
}

export type TaskExecutionMetrics = {
  task_id: string
  current_stage: string
  bottleneck_source: string
  failure_stage: string
  failure_category: string
  assigned_at?: string | null
  provider_started_at?: string | null
  provider_finished_at?: string | null
  finalize_started_at?: string | null
  finalize_finished_at?: string | null
  queue_wait_ms: number
  worker_overhead_ms: number
  provider_latency_ms: number
  finalize_latency_ms: number
  total_latency_ms: number
  created_at: string
  updated_at: string
}

export type MonitorTaskOutput = {
  id: number
  task_id: string
  asset_id?: string
  output_kind: string
  role: string
  url: string
  storage_key?: string
  mime_type: string
  width: number
  height: number
  size_bytes: number
  source_url?: string
  checksum_sha256?: string
  storage_profile?: string
  metadata?: string
  result_index: number
  created_at: string
  updated_at: string
}

export type AdminTaskExecutionAttempt = {
  attempt_no: number
  status: string
  stage: string
  failure_category: string
  provider: string
  gateway_step: string
  resolved_model: string
  provider_started_at: string | null
  provider_finished_at: string | null
  completed_at: string | null
  cancelled_at: string | null
  provider_latency_ms: number
  total_latency_ms: number
}

export type AdminMonitorDetail = {
  task_id: string
  correlation_id: string
  user_public_id: string
  user_display_name: string
  prompt: string
  user_prompt: string
  params: string
  feature_slug: string
  priority: number
  status: string
  retry_count: number
  error_msg: string
  result: string
  outputs?: MonitorTaskOutput[]
  execution_metrics?: TaskExecutionMetrics
  created_at: string
  updated_at: string
  completed_at: string | null
  cancelled_at: string | null
  cancel_reason: string
  billing?: AdminTaskBillingSnapshot
  latest_execution?: AdminMonitorExecutionSnapshot
  execution_attempts: AdminTaskExecutionAttempt[]
}

export type AdminMonitorListItem = {
  task_id: string
  user_public_id: string
  user_display_name: string
  prompt: string
  feature_slug: string
  credits: number
  priority: number
  status: string
  error_msg: string
  result: string
  created_at: string
  completed_at: string | null
  latest_execution?: AdminMonitorExecutionSnapshot
}

export type ApiResponseAdminTaskData = {
  task: AdminMonitorDetail
}

export type ApiResponseAdminTaskListData = {
  list: AdminMonitorListItem[]
  total: number
  pending_count: number
  running_count: number
  completed_count: number
  failed_count: number
  cancelled_count: number
  other_count: number
}

export type MonitorLatencySignal = {
  samples: number
  avg_ms: number
  p95_ms: number
  max_ms: number
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

export type MonitorExecutionPressurePoint = {
  key: string
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

export type ApiResponseMonitor = ApiResponseWithData<MonitorResponse>

export type AdminTaskListQueryParams = ListAdminTasksParams
export type AdminMonitorDetailModel = AdminMonitorDetail
export type AdminMonitorListItemModel = AdminMonitorListItem
export type AdminMonitorExecutionAttempt = AdminTaskExecutionAttempt

function taskListUrl(params: ListAdminTasksParams): string {
  const search = new URLSearchParams()
  for (const [key, value] of Object.entries(params ?? {})) {
    if (value !== undefined) search.append(key, String(value))
  }
  const query = search.toString()
  return query ? `/admin/tasks?${query}` : '/admin/tasks'
}

export const ADMIN_TASK_LIST_QUERY_KEY = ['admin', 'tasks'] as const
export const ADMIN_MONITOR_QUERY_KEY = ['admin', 'monitor'] as const

type NestedQueryOptions<TQueryFnData> = {
  query?: {
    placeholderData?: (prev: TQueryFnData | undefined) => TQueryFnData | undefined
    refetchInterval?: number | false | (() => number | false)
    refetchIntervalInBackground?: boolean
    refetchOnWindowFocus?: boolean
    enabled?: MaybeRefOrGetter<boolean>
  }
}

const selectAdminTaskList = createApiDataSelector<ApiResponseAdminTaskListData>('加载任务列表失败')
const selectAdminTaskDetail = createApiDataSelector<ApiResponseAdminTaskData>('加载任务详情失败')
const selectAdminMonitor = createApiDataSelector<MonitorResponse>('加载监控数据失败')

export function useAdminTaskListResource(
  params: MaybeRef<AdminTaskListQueryParams>,
  options?: NestedQueryOptions<ApiResponseWithData<ApiResponseAdminTaskListData>>,
) {
  return useQuery<
    ApiResponseWithData<ApiResponseAdminTaskListData>,
    Error,
    ApiResponseAdminTaskListData
  >({
    ...(options?.query ?? {}),
    queryKey: [ADMIN_TASK_LIST_QUERY_KEY, params] as const,
    queryFn: ({ signal }) =>
      api.request<ApiResponseWithData<ApiResponseAdminTaskListData>>(
        taskListUrl(toValue(params) ?? {}),
        {
          method: 'GET',
          ...(signal == null ? {} : { signal }),
        },
      ),
    select: selectAdminTaskList,
  })
}

export function getAdminTaskListQueryKey() {
  return ADMIN_TASK_LIST_QUERY_KEY
}

export function getAdminTaskDetailQueryKey(taskID: string) {
  return ['admin', 'tasks', taskID] as const
}

export async function getAdminTaskList(
  params: AdminTaskListQueryParams,
): Promise<ApiResponseAdminTaskListData> {
  return resolveApiData(
    api.request<ApiResponseWithData<ApiResponseAdminTaskListData>>(taskListUrl(params), {
      method: 'GET',
    }),
    '加载任务列表失败',
  )
}

export function getAdminMonitorQueryKey() {
  return ADMIN_MONITOR_QUERY_KEY
}

export function useAdminMonitorResource(
  options?: NestedQueryOptions<ApiResponseWithData<MonitorResponse>>,
) {
  return useQuery<ApiResponseWithData<MonitorResponse>, Error, MonitorResponse>({
    ...(options?.query ?? {}),
    queryKey: ADMIN_MONITOR_QUERY_KEY,
    queryFn: ({ signal }) =>
      api.request<ApiResponseWithData<MonitorResponse>>(
        `${CONTROL_API_PREFIX}${getGetControlMonitorUrl()}`,
        {
          method: 'GET',
          ...(signal == null ? {} : { signal }),
        },
      ),
    select: selectAdminMonitor,
  })
}

export async function cancelAdminTaskByID(taskID: string): Promise<void> {
  await resolveApiData(
    api.request<ApiResponseWithData<unknown>>(
      `/admin/tasks/${encodeURIComponent(taskID)}/cancel`,
      {
        method: 'POST',
        headers: { 'Idempotency-Key': taskCancellationIdempotencyKey(taskID) },
      },
    ),
    '取消任务失败',
  )
}

export function useAdminTaskDetailResource(
  taskID: MaybeRef<string>,
  options?: NestedQueryOptions<ApiResponseWithData<ApiResponseAdminTaskData>>,
) {
  return useQuery<
    ApiResponseWithData<ApiResponseAdminTaskData>,
    Error,
    ApiResponseAdminTaskData
  >({
    ...(options?.query ?? {}),
    queryKey: [ADMIN_MONITOR_QUERY_KEY, 'detail', taskID] as const,
    queryFn: ({ signal }) =>
      api.request<ApiResponseWithData<ApiResponseAdminTaskData>>(
        `/admin/tasks/${encodeURIComponent(toValue(taskID) ?? '')}`,
        {
          method: 'GET',
          ...(signal == null ? {} : { signal }),
        },
      ),
    select: selectAdminTaskDetail,
  })
}

export async function getAdminTaskDetail(taskID: string): Promise<AdminMonitorDetailModel> {
  const data = await resolveApiData(
    api.request<ApiResponseWithData<ApiResponseAdminTaskData>>(
      `/admin/tasks/${encodeURIComponent(taskID)}`,
      { method: 'GET' },
    ),
    '加载任务详情失败',
  )
  return data.task
}
