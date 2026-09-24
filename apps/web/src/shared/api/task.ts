import { api } from './client'
import { taskCancellationIdempotencyKey } from './idempotencyKey'
import { resolveApiData, resolveApiOk, type ApiResponseWithData } from './apiResponse'
import { i18n } from '../i18n'

// User task lifecycle boundary (issue #809).
//
// The legacy generated `tasks/tasks` client is gone, but the backend user
// plane still serves `/api/user/tasks/*` with the shared `{code, msg, data}`
// envelope, so this boundary carries a thin hand-rolled transport over the
// central `api.request`. Types are feature-neutral views of the wire shapes
// (`apps/api/internal/tasking`); canvas adapters keep importing these names,
// never generated code directly.
export type SubmitTaskParams = {
  ratio?: string
  custom_width?: number
  custom_height?: number
  original_image_width?: number
  original_image_height?: number
  output_count?: number
  output_resolution?: string
  variation?: { strength?: number }
  [key: string]: unknown
}

export type SubmitTaskData = {
  prompt: string
  feature?: string
  client_action_id?: string
  priority?: number
  image_urls?: string[]
  image_asset_ids?: number[]
  reference_image_asset_ids?: number[]
  params?: SubmitTaskParams
}

export type SubmitTaskRequest = SubmitTaskData
export type SubmitTaskReadinessRequestData = SubmitTaskData
export type SubmitTaskParamsAlias = SubmitTaskParams

export type SubmitTaskReadinessBilling = {
  sufficient: boolean
  unit_credits: number
  total_credits: number
  available_credits: number
  reserved_credits: number
  price_version_id: number
  price_version?: number
  quantity?: number
  [key: string]: unknown
}

export type SubmitTaskReadinessData = {
  ready: boolean
  message?: string
  error_code?: string
  feature?: string
  billing?: SubmitTaskReadinessBilling | null
}

export type ApiResponseTaskSubmitReadinessData = SubmitTaskReadinessData

export type SubmitTaskReadinessMatrixRequest = {
  requests: SubmitTaskData[]
}

export type SubmitTaskReadinessMatrixItem = SubmitTaskReadinessData

export type TaskOutput = {
  id?: number
  task_id?: string
  asset_id?: string
  output_kind?: string
  role?: string
  url?: string
  mime_type?: string
  width?: number
  height?: number
  size_bytes?: number
  result_index?: number
}

export type TaskExecutionMetrics = {
  failure_stage?: string | undefined
  failure_category?: string | undefined
  [key: string]: unknown
}

export type Task = {
  task_id: string
  correlation_id?: string
  prompt?: string
  status: string
  error_msg?: string
  cancel_reason?: string
  result?: string
  outputs?: TaskOutput[]
  execution_metrics?: TaskExecutionMetrics | null
  created_at?: string
  updated_at?: string
  completed_at?: string | null
}

export type ListUserTasksParams = {
  user_public_id?: string
  account_id?: number
  status?: string
  keyword?: string
  page?: number
  page_size?: number
}

type SubmitTaskResponse = {
  task_id: string
}

async function submit(request: SubmitTaskData): Promise<SubmitTaskResponse> {
  const fallback = () => i18n.global.t('common.errors.taskSubmitFailed')
  const data = await resolveApiData(
    api.request<ApiResponseWithData<SubmitTaskResponse>>('/user/tasks/submit', {
      method: 'POST',
      body: request as unknown as Record<string, unknown>,
    }),
    fallback(),
  )
  if (!data.task_id?.trim()) throw new Error(fallback())
  return { ...data, task_id: data.task_id.trim() }
}

async function checkSubmitReadiness(
  request: SubmitTaskReadinessRequestData,
): Promise<SubmitTaskReadinessData> {
  const fallback = () => i18n.global.t('common.errors.billingInfoUnavailable')
  const data = await resolveApiData(
    api.request<ApiResponseWithData<SubmitTaskReadinessData>>(
      '/user/tasks/submit/readiness',
      {
        method: 'POST',
        body: request as unknown as Record<string, unknown>,
      },
    ),
    fallback(),
  )
  if (typeof data.ready !== 'boolean') throw new Error(fallback())
  return data
}

async function checkSubmitReadinessMatrix(
  request: SubmitTaskReadinessMatrixRequest,
): Promise<SubmitTaskReadinessMatrixItem[]> {
  const fallback = () => i18n.global.t('common.errors.billingInfoUnavailable')
  const data = await resolveApiData(
    api.request<ApiResponseWithData<{ items: SubmitTaskReadinessMatrixItem[] }>>(
      '/user/tasks/submit/readiness/matrix',
      {
        method: 'POST',
        body: request as unknown as Record<string, unknown>,
      },
    ),
    fallback(),
  )
  if (!Array.isArray(data.items)) throw new Error(fallback())
  return data.items as SubmitTaskReadinessMatrixItem[]
}

async function get(taskID: string): Promise<{ task: Task }> {
  const fallback = () => i18n.global.t('common.errors.taskLoadFailed')
  const data = await resolveApiData(
    api.request<ApiResponseWithData<{ task: Task }>>(
      `/user/tasks/${encodeURIComponent(taskID)}`,
      { method: 'GET' },
    ),
    fallback(),
  )
  if (!data.task) throw new Error(fallback())
  return data
}

async function cancel(taskID: string) {
  await resolveApiOk(
    api.request<{ code: number; msg: string }>(
      `/user/tasks/${encodeURIComponent(taskID)}/cancel`,
      {
        method: 'POST',
        headers: { 'Idempotency-Key': taskCancellationIdempotencyKey(taskID) },
      },
    ),
    i18n.global.t('common.errors.taskCancelFailed'),
  )
}

export const taskApi = {
  submit,
  checkSubmitReadiness,
  checkSubmitReadinessMatrix,
  get,
  cancel,
}
