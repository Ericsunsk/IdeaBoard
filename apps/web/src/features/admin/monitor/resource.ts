import { useQuery } from '@tanstack/vue-query'
import { computed, toValue, type MaybeRef, type MaybeRefOrGetter } from 'vue'
import { requireContractData } from '@/api/contractKernel'
import { withSignal } from '@/core/utils/request'
import { getControlMonitor } from '@/api/generated-query/control/control-platform/control-platform'
import {
  cancelControlTask,
  getControlTask,
  listControlTasks,
} from '@/api/generated-query/control/control-tasks/control-tasks'
import type {
  ControlTask,
  ControlTaskAttempt,
  ControlTaskRoutingSnapshot,
  ListControlTasksParams as GeneratedListControlTasksParams,
  PaginatedControlTasks,
} from '@/api/generated-query/control/model'
import type { ControlTaskEnvelope } from '@/api/generated-query/control/model/controlTaskEnvelope'
import { buildControlMutationHeaders } from '../shared/core/controlMutation'
import { keepPrevious } from '@/core/query/queryDefaults'
import type { TaskStatus } from '../shared/core/taskMetricTokens'

// Monitor and task read models use Control API envelopes through this resource.
// Pages consume normalized view models; the SSE monitor stream uses its raw protocol.
export type ListAdminTasksParams = {
  status?: TaskStatus
  user_public_id?: string
  account_id?: number
  keyword?: string
  page?: number
  page_size?: number
}

type AdminMonitorExecutionSnapshot = {
  resolved_model: string
  provider: string
  gateway_step: string
}

type AdminTaskBillingSnapshot = {
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
  // Catalog existence is unknown on the control task read path (no
  // billable lookup there): undefined means unknown, only an explicit
  // false captions the gone-feature notice.
  feature_exists?: boolean | undefined
}

type TaskExecutionMetrics = {
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

type MonitorTaskOutput = {
  id: number
  task_id: string
  asset_id?: string | undefined
  output_kind: string
  role: string
  url: string
  storage_key?: string
  mime_type: string
  width: number
  height: number
  size_bytes: number
  source_url?: string
  checksum_sha256?: string | undefined
  storage_profile?: string | undefined
  metadata?: string | undefined
  result_index: number
  created_at: string
  updated_at: string
}

type AdminTaskExecutionAttempt = {
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
  // Routing observability (#852): retry-layer distinction per send
  // (gateway vs model_fallback vs river_task) plus the same-model
  // gateway retry count and the truncated per-send failure reason.
  service?: string | undefined
  requested_model?: string | undefined
  retry_layer?: string | undefined
  gateway_retry_count?: number | undefined
  error_msg?: string | undefined
}

// Read-only projection over the immutable submit-time routing snapshot
// (#852, ADR-0012 rule 9): requested service/resolution, eligible pool
// entries, deterministic selection, single retry candidate, pool version.
type AdminTaskRoutingSnapshot = {
  service: string
  requested_resolution: string
  eligible_models: string[]
  selected_model: string
  retry_candidate?: string | undefined
  pool_version: number
}

type AdminMonitorDetail = {
  task_id: string
  correlation_id: string
  user_public_id: string
  user_display_name: string
  prompt: string
  user_prompt: string
  params: string
  service: string
  priority: number
  status: string
  retry_count: number
  error_msg: string
  result: string
  outputs?: MonitorTaskOutput[] | undefined
  execution_metrics?: TaskExecutionMetrics | undefined
  created_at: string
  updated_at: string
  completed_at: string | null
  cancelled_at: string | null
  cancel_reason: string
  billing?: AdminTaskBillingSnapshot | undefined
  latest_execution?: AdminMonitorExecutionSnapshot | undefined
  execution_attempts: AdminTaskExecutionAttempt[]
  // Submit-time routing snapshot projection (#852). Absent when the
  // task never pinned a snapshot (missing model data).
  routing?: AdminTaskRoutingSnapshot | undefined
  // Execution attributed to the resolved (final) model (#852).
  final_resolved_model?: string | undefined
}

export type AdminMonitorListItem = {
  task_id: string
  user_public_id: string
  user_display_name: string
  prompt: string
  service: string
  credits: number
  priority: number
  status: string
  error_msg: string
  result: string
  created_at: string
  completed_at: string | null
  latest_execution?: AdminMonitorExecutionSnapshot | undefined
  // Requested service/resolution, pool version, and selected vs final
  // model from the pinned snapshot (#852). Absent when never pinned.
  requested_resolution?: string | undefined
  pool_version?: number | undefined
  selected_model?: string | undefined
  final_resolved_model?: string | undefined
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

type MonitorLatencySignal = {
  samples: number
  avg_ms: number
  p95_ms: number
  max_ms: number
}

type MonitorGoldenSignals = {
  window_seconds: number
  sample_count: number
  provider_attempts: number
  provider_error_count: number
  provider_error_rate: number
  queue_delay_ms: MonitorLatencySignal
  provider_latency_ms: MonitorLatencySignal
  finalize_latency_ms: MonitorLatencySignal
}

type MonitorExecutionPressurePoint = {
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

export type ApiResponseMonitor = { data: MonitorResponse }

export type AdminTaskListQueryParams = ListAdminTasksParams
export type AdminMonitorDetailModel = AdminMonitorDetail
export type AdminMonitorListItemModel = AdminMonitorListItem
export type AdminMonitorExecutionAttempt = AdminTaskExecutionAttempt

export const ADMIN_TASK_LIST_QUERY_KEY = ['admin', 'tasks', 'list'] as const
export const ADMIN_MONITOR_QUERY_KEY = ['admin', 'monitor'] as const
const ADMIN_TASK_DETAIL_QUERY_ROOT = [...ADMIN_MONITOR_QUERY_KEY, 'detail'] as const

type NestedQueryOptions = {
  query?: {
    refetchInterval?: number | false | (() => number | false)
    refetchIntervalInBackground?: boolean
    refetchOnWindowFocus?: boolean
    enabled?: MaybeRefOrGetter<boolean>
  }
}

function selectAdminTaskList(response: PaginatedControlTasks): ApiResponseAdminTaskListData {
  const counts = response.meta.counts ?? {
    total: 0,
    pending_count: 0,
    running_count: 0,
    completed_count: 0,
    failed_count: 0,
    cancelled_count: 0,
    other_count: 0,
  }
  return {
    list: (response.data ?? []).map(mapControlTaskToListItem),
    total: counts.total,
    pending_count: counts.pending_count,
    running_count: counts.running_count,
    completed_count: counts.completed_count,
    failed_count: counts.failed_count,
    cancelled_count: counts.cancelled_count,
    other_count: counts.other_count,
  }
}
function selectAdminTaskDetail(response: ControlTaskEnvelope): ApiResponseAdminTaskData {
  return { task: mapControlTaskToDetail(response.data.task) }
}
function selectAdminMonitor(response: ApiResponseMonitor): MonitorResponse {
  return requireContractData<MonitorResponse>(response, '加载监控数据失败')
}

export function useAdminTaskListResource(
  params: MaybeRef<AdminTaskListQueryParams>,
  options?: NestedQueryOptions,
) {
  return useQuery<PaginatedControlTasks, Error, ApiResponseAdminTaskListData>({
    ...keepPrevious(),
    ...(options?.query ?? {}),
    queryKey: computed(() => [...ADMIN_TASK_LIST_QUERY_KEY, toValue(params)]),
    queryFn: ({ signal }) => fetchAdminTaskList(toValue(params), signal),
    select: selectAdminTaskList,
  })
}

export function getAdminTaskListQueryKey() {
  return ADMIN_TASK_LIST_QUERY_KEY
}

export function getAdminTaskDetailQueryKey(taskID: string) {
  return [...ADMIN_TASK_DETAIL_QUERY_ROOT, taskID] as const
}

// encodeTaskListCursor mirrors the backend opaque cursor
// (base64url of {"page": N}, platform/pagination): page 1 omits the
// cursor, later pages encode it. Non-canonical spellings fail closed
// server-side as pagination_cursor_invalid.
export function encodeTaskListCursor(page: number): string | undefined {
  if (!Number.isSafeInteger(page) || page <= 1) return undefined
  const raw = JSON.stringify({ page })
  return btoa(raw).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/g, '')
}

function toGeneratedListParams(params: AdminTaskListQueryParams): GeneratedListControlTasksParams {
  const query: GeneratedListControlTasksParams = {}
  if (params.status) query.status = params.status
  if (params.page_size !== undefined) query.page_size = params.page_size
  if (params.keyword) query.keyword = params.keyword
  // The control task filter takes the user public id (usr_...) as
  // user_id; the account scope has no control task filter and is
  // documented as ignored here.
  if (params.user_public_id) query.user_id = params.user_public_id
  const cursor = encodeTaskListCursor(params.page ?? 1)
  if (cursor !== undefined) query.cursor = cursor
  return query
}

function mapControlAttempt(attempt: ControlTaskAttempt): AdminTaskExecutionAttempt {
  return {
    attempt_no: attempt.attempt_no,
    status: attempt.status,
    stage: attempt.stage ?? '',
    failure_category: attempt.failure_category ?? '',
    provider: attempt.provider ?? '',
    gateway_step: attempt.gateway_step ?? '',
    resolved_model: attempt.resolved_model ?? '',
    provider_started_at: attempt.provider_started_at ?? null,
    provider_finished_at: attempt.provider_finished_at ?? null,
    completed_at: attempt.completed_at ?? null,
    cancelled_at: attempt.cancelled_at ?? null,
    provider_latency_ms: attempt.provider_latency_ms ?? 0,
    total_latency_ms: attempt.total_latency_ms ?? 0,
    service: attempt.service,
    requested_model: attempt.requested_model,
    retry_layer: attempt.retry_layer,
    gateway_retry_count: attempt.gateway_retry_count,
    error_msg: attempt.error_msg,
  }
}

function mapControlRouting(
  routing: ControlTaskRoutingSnapshot | undefined,
): AdminTaskRoutingSnapshot | undefined {
  if (!routing) return undefined
  return {
    service: routing.service,
    requested_resolution: routing.requested_resolution,
    eligible_models: [...(routing.eligible_models ?? [])],
    selected_model: routing.selected_model,
    retry_candidate: routing.retry_candidate,
    pool_version: routing.pool_version,
  }
}

function mapControlLatestExecution(task: ControlTask): AdminMonitorExecutionSnapshot | undefined {
  if (!task.latest_execution) return undefined
  return {
    resolved_model: task.latest_execution.resolved_model ?? '',
    provider: task.latest_execution.provider ?? '',
    gateway_step: task.latest_execution.gateway_step ?? '',
  }
}

function mapControlTaskToListItem(task: ControlTask): AdminMonitorListItem {
  return {
    task_id: task.task_id,
    user_public_id: task.user_public_id ?? '',
    user_display_name: task.user_display_name ?? '',
    prompt: task.prompt ?? '',
    service: task.service ?? '',
    credits: task.credits ?? 0,
    priority: task.priority ?? 0,
    status: task.status ?? '',
    error_msg: task.error_msg ?? '',
    result: task.result ?? '',
    created_at: task.created_at ?? '',
    completed_at: task.completed_at ?? null,
    latest_execution: mapControlLatestExecution(task),
    requested_resolution: task.requested_resolution,
    pool_version: task.pool_version,
    selected_model: task.selected_model,
    final_resolved_model: task.final_resolved_model,
  }
}

function mapControlTaskToDetail(task: ControlTask): AdminMonitorDetail {
  return {
    ...mapControlTaskToListItem(task),
    correlation_id: task.correlation_id ?? '',
    user_prompt: task.user_prompt ?? '',
    params: task.params ?? '',
    retry_count: task.retry_count ?? 0,
    updated_at: task.updated_at ?? '',
    cancelled_at: task.cancelled_at ?? null,
    cancel_reason: task.cancel_reason ?? '',
    billing: mapControlBilling(task.billing),
    outputs: (task.outputs ?? []).map(mapControlOutput),
    execution_metrics: mapControlExecutionMetrics(task.execution_metrics),
    execution_attempts: (task.execution_attempts ?? []).map(mapControlAttempt),
    routing: mapControlRouting(task.routing),
    final_resolved_model: task.final_resolved_model,
  }
}

function wireString(record: Record<string, unknown>, key: string): string {
  const value = record[key]
  return typeof value === 'string' ? value : ''
}

function wireNumber(record: Record<string, unknown>, key: string): number {
  const value = record[key]
  return typeof value === 'number' && Number.isFinite(value) ? value : 0
}

function wireNullableString(record: Record<string, unknown>, key: string): string | null {
  const value = record[key]
  if (value === null || value === undefined) return null
  return typeof value === 'string' ? value : null
}

function mapControlBilling(billing: ControlTask['billing']): AdminTaskBillingSnapshot | undefined {
  if (!billing) return undefined
  const record = billing as Record<string, unknown>
  const settledAt = wireNullableString(record, 'settled_at')
  return {
    sku: wireString(record, 'sku'),
    output_resolution: wireString(record, 'output_resolution'),
    display_name: wireString(record, 'display_name'),
    price_version_id: wireNumber(record, 'price_version_id'),
    price_version: wireNumber(record, 'price_version'),
    unit_credits: wireNumber(record, 'unit_credits'),
    quantity: wireNumber(record, 'quantity'),
    total_credits: wireNumber(record, 'total_credits'),
    captured_credits: wireNumber(record, 'captured_credits'),
    quoted_at: wireString(record, 'quoted_at'),
    settlement_state: wireString(record, 'settlement_state'),
    settled_at: settledAt,
    // The control task read model does not resolve catalog existence
    // (no billable lookup on the read path): absent means unknown, and
    // the dialog only captions a definite false.
    feature_exists:
      record['feature_exists'] === true
        ? true
        : record['feature_exists'] === false
          ? false
          : undefined,
  }
}

function mapControlOutput(output: Record<string, unknown>): MonitorTaskOutput {
  const record = output as Record<string, unknown>
  return {
    id: wireNumber(record, 'id'),
    task_id: wireString(record, 'task_id'),
    asset_id: wireString(record, 'asset_id') || undefined,
    output_kind: wireString(record, 'output_kind'),
    role: wireString(record, 'role'),
    url: wireString(record, 'url'),
    mime_type: wireString(record, 'mime_type'),
    width: wireNumber(record, 'width'),
    height: wireNumber(record, 'height'),
    size_bytes: wireNumber(record, 'size_bytes'),
    checksum_sha256: wireString(record, 'checksum_sha256') || undefined,
    result_index: wireNumber(record, 'result_index'),
    created_at: wireString(record, 'created_at'),
    updated_at: wireString(record, 'updated_at'),
  }
}

function mapControlExecutionMetrics(
  metrics: ControlTask['execution_metrics'],
): TaskExecutionMetrics | undefined {
  if (!metrics) return undefined
  const record = metrics as Record<string, unknown>
  return {
    task_id: wireString(record, 'task_id'),
    current_stage: wireString(record, 'current_stage'),
    bottleneck_source: wireString(record, 'bottleneck_source'),
    failure_stage: wireString(record, 'failure_stage'),
    failure_category: wireString(record, 'failure_category'),
    assigned_at: wireNullableString(record, 'assigned_at'),
    provider_started_at: wireNullableString(record, 'provider_started_at'),
    provider_finished_at: wireNullableString(record, 'provider_finished_at'),
    finalize_started_at: wireNullableString(record, 'finalize_started_at'),
    finalize_finished_at: wireNullableString(record, 'finalize_finished_at'),
    queue_wait_ms: wireNumber(record, 'queue_wait_ms'),
    worker_overhead_ms: wireNumber(record, 'worker_overhead_ms'),
    provider_latency_ms: wireNumber(record, 'provider_latency_ms'),
    finalize_latency_ms: wireNumber(record, 'finalize_latency_ms'),
    total_latency_ms: wireNumber(record, 'total_latency_ms'),
    created_at: wireString(record, 'created_at'),
    updated_at: wireString(record, 'updated_at'),
  }
}

async function fetchAdminTaskList(
  params: AdminTaskListQueryParams,
  signal?: AbortSignal,
): Promise<PaginatedControlTasks> {
  try {
    return await listControlTasks(toGeneratedListParams(params), { signal })
  } catch (error) {
    throw withTaskCopy(error, '加载任务列表失败')
  }
}

async function fetchAdminTaskDetail(
  taskID: string,
  signal?: AbortSignal,
): Promise<ControlTaskEnvelope> {
  try {
    return await getControlTask(taskID, { signal })
  } catch (error) {
    throw withTaskCopy(error, '加载任务详情失败')
  }
}

function withTaskCopy(error: unknown, fallback: string): Error {
  if (error instanceof Error) return error
  return new Error(fallback)
}

export async function getAdminTaskList(
  params: AdminTaskListQueryParams,
): Promise<ApiResponseAdminTaskListData> {
  return selectAdminTaskList(await fetchAdminTaskList(params))
}

export function getAdminMonitorQueryKey() {
  return ADMIN_MONITOR_QUERY_KEY
}

export function useAdminMonitorResource(options?: NestedQueryOptions) {
  return useQuery<ApiResponseMonitor, Error, MonitorResponse>({
    ...(options?.query ?? {}),
    queryKey: ADMIN_MONITOR_QUERY_KEY,
    queryFn: ({ signal }) => getControlMonitor(withSignal(signal)) as Promise<ApiResponseMonitor>,
    select: selectAdminMonitor,
  })
}

export async function cancelAdminTaskByID(taskID: string): Promise<void> {
  requireContractData(
    await cancelControlTask(taskID, { headers: buildControlMutationHeaders() }),
    '取消任务失败',
  )
}

export function useAdminTaskDetailResource(taskID: MaybeRef<string>, options?: NestedQueryOptions) {
  return useQuery<ControlTaskEnvelope, Error, ApiResponseAdminTaskData>({
    ...(options?.query ?? {}),
    queryKey: computed(() => getAdminTaskDetailQueryKey(toValue(taskID))),
    queryFn: ({ signal }) => fetchAdminTaskDetail(toValue(taskID), signal ?? undefined),
    select: selectAdminTaskDetail,
  })
}

export async function getAdminTaskDetail(taskID: string): Promise<AdminMonitorDetailModel> {
  const detail = await fetchAdminTaskDetail(taskID)
  return selectAdminTaskDetail(detail).task
}
