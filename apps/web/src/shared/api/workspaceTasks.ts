// Workspace task lifecycle transport (issue #805, Public Data Plane).
//
// Feature-neutral resource over `/api/v1/workspaces/{workspace_id}/tasks`
// through the central `api.request` + `runtimeUrls` with thin snake_case
// transport types only: snake_case -> camelCase mapping lives in canvas
// task-runner adapters (keep this resource thin).
//
// TODO(#805-regen): move these calls onto `generated-query/public` once the
// merger stream regenerates it from the task operations documented in the
// #805 report. Never import `generated-query/admin|control` here, and route
// pages must consume the canvas runner, never this module directly.
//
// Legacy `/api/user` task behavior (submit/readiness/matrix/cancel) is owned
// by `task.ts` + `taskEvents.ts` and untouched here (removal is #809).

import { api } from './client'
import {
  buildContractRequestHeaders,
  isOpaqueId,
  newRequestId,
  opaqueIdPrefix,
  requireContractData,
  type OpaqueIdPrefix,
} from './contractKernel'
import { resolveApiUrl } from '../config/runtimeUrls'
import { createPollDelay, sleep } from '../utils/polling'
import { subscribeWorkspaceEvent } from './workspaceEventStream'

export type WorkspaceTaskStatus = 'pending' | 'running' | 'completed' | 'failed' | 'cancelled'

const WORKSPACE_TASK_STATUSES: readonly WorkspaceTaskStatus[] = [
  'pending',
  'running',
  'completed',
  'failed',
  'cancelled',
]

export function mapWorkspaceTaskStatus(value: unknown): WorkspaceTaskStatus {
  if (typeof value === 'string') {
    // cancelled_with_results converges to cancelled for the v1 surface
    // (outputs carry the results).
    if (value === 'cancelled_with_results') return 'cancelled'
    if ((WORKSPACE_TASK_STATUSES as readonly string[]).includes(value)) {
      return value as WorkspaceTaskStatus
    }
  }
  throw new Error(`Unknown workspace task status: ${String(value)}`)
}

export function isWorkspaceTaskTerminal(status: WorkspaceTaskStatus): boolean {
  return status === 'completed' || status === 'failed' || status === 'cancelled'
}

function assertOpaqueId(value: string, prefix: OpaqueIdPrefix, label: string): string {
  const trimmed = value.trim()
  if (!isOpaqueId(trimmed) || opaqueIdPrefix(trimmed) !== prefix) {
    throw new Error(`Invalid ${label}: expected ${prefix}_...`)
  }
  return trimmed
}

export function assertWorkspaceTaskWorkspaceId(value: string): string {
  return assertOpaqueId(value, 'ws', 'workspace id')
}

export function assertWorkspaceTaskId(value: string): string {
  return assertOpaqueId(value, 'task', 'task id')
}

export function assertWorkspaceTaskAssetId(value: string): string {
  return assertOpaqueId(value, 'asset', 'asset id')
}

function encodeId(value: string): string {
  return encodeURIComponent(value)
}

export function workspaceTasksSuffix(workspaceId: string): string {
  return `/v1/workspaces/${encodeId(assertWorkspaceTaskWorkspaceId(workspaceId))}/tasks`
}

export function workspaceTaskSuffix(workspaceId: string, taskId: string): string {
  return `${workspaceTasksSuffix(workspaceId)}/${encodeId(assertWorkspaceTaskId(taskId))}`
}

export function workspaceTaskCancelSuffix(workspaceId: string, taskId: string): string {
  return `${workspaceTaskSuffix(workspaceId, taskId)}/cancel`
}

export function workspaceTaskOutputsSuffix(workspaceId: string, taskId: string): string {
  return `${workspaceTaskSuffix(workspaceId, taskId)}/outputs`
}

export function workspaceTaskQuotesSuffix(workspaceId: string): string {
  return `/v1/workspaces/${encodeId(assertWorkspaceTaskWorkspaceId(workspaceId))}/task-quotes`
}

// Canonical browser URLs (full `/api/v1/...` form for Location handling
// and link sharing). The server `Location` header on 202 carries the same
// shape; clients derive it from the returned id when headers are opaque.
export function workspaceTaskLocation(workspaceId: string, taskId: string): string {
  return `/api/v1/workspaces/${encodeURIComponent(workspaceId)}/tasks/${encodeURIComponent(taskId)}`
}

export type WorkspaceTaskOutputRef = {
  kind: 'asset' | 'remote'
  asset_id?: string | undefined
  url?: string | undefined
  checksum_sha256?: string | undefined
  role?: string | undefined
  result_index: number
}

export type WorkspaceTaskBillingSnapshot = {
  sku: string
  display_name: string
  price_version_id: number
  price_version: number
  output_resolution: string
  unit_credits: number
  quantity: number
  total_credits: number
  quoted_at: string
  hold_transaction_id?: string | undefined
  settlement_state: string
}

export type WorkspaceTaskItem = {
  id: string
  workspace_id: string
  workflow_kind: string
  media_asset_ids?: string[] | undefined
  status: WorkspaceTaskStatus
  retry_count: number
  execution_token?: string | undefined
  output_refs?: WorkspaceTaskOutputRef[] | undefined
  billing_snapshot: WorkspaceTaskBillingSnapshot
  error_msg?: string | undefined
  cancel_reason?: string | undefined
  created_at: string
  updated_at: string
  completed_at?: string | null | undefined
}

export type CreateWorkspaceTaskInput = {
  workflow_kind: string
  media_asset_ids?: string[] | undefined
  params?: Record<string, unknown> | undefined
  quantity?: number | undefined
  resolution?: string | undefined
}

export type WorkspaceTasksRequestOptions = {
  signal?: AbortSignal | null | undefined
  requestId?: string | undefined
  idempotencyKey?: string | undefined
}

function toRequestConfig(options: WorkspaceTasksRequestOptions | undefined): {
  signal?: AbortSignal | undefined
  headers: Record<string, string>
} {
  return {
    ...(options?.signal == null ? {} : { signal: options.signal }),
    headers: buildContractRequestHeaders({
      requestId: options?.requestId?.trim() ? options.requestId.trim() : newRequestId(),
      ...(options?.idempotencyKey == null ? {} : { idempotencyKey: options.idempotencyKey }),
    }),
  }
}

function withCursorQuery(url: string, params?: { cursor?: string | null | undefined; pageSize?: number | undefined }): string {
  const search = new URLSearchParams()
  if (params?.cursor?.trim()) search.set('cursor', params.cursor.trim())
  search.set('page_size', String(params?.pageSize ?? 20))
  return `${url}?${search.toString()}`
}

export function toWorkspaceTask(raw: unknown): WorkspaceTaskItem {
  const item = requireContractData<WorkspaceTaskItem>(raw, 'Task load failed') as WorkspaceTaskItem
  // Thin validation: status mapping throws on unknown states; ids are
  // shape-checked so enumeration feedback never leaks.
  mapWorkspaceTaskStatus((item as Record<string, unknown>).status)
  assertWorkspaceTaskId(item.id)
  assertWorkspaceTaskWorkspaceId(item.workspace_id)
  return item
}

export function toWorkspaceTaskOutputs(raw: unknown): { task_id: string; outputs: WorkspaceTaskOutputRef[] } {
  const data = requireContractData<{ task_id: string; outputs: WorkspaceTaskOutputRef[] }>(raw, 'Task outputs load failed')
  assertWorkspaceTaskId(data.task_id)
  const outputs = Array.isArray(data.outputs) ? data.outputs : []
  for (const ref of outputs) {
    if (ref.kind !== 'asset' && ref.kind !== 'remote') {
      throw new Error(`Unknown task output kind: ${String((ref as Record<string, unknown>).kind)}`)
    }
    if (ref.kind === 'asset') {
      assertWorkspaceTaskAssetId(ref.asset_id ?? '')
    }
    if (ref.kind === 'remote' && typeof ref.url === 'string') {
      const lowered = ref.url.toLowerCase()
      for (const marker of ['ws-uploads', 'builtin-local', 'storage_key']) {
        if (lowered.includes(marker)) throw new Error('Task output leaks storage internals')
      }
    }
  }
  return { task_id: data.task_id, outputs }
}

export const workspaceTasksApiUrls = {
  collection: (workspaceId: string): string => resolveApiUrl(workspaceTasksSuffix(workspaceId)),
  item: (workspaceId: string, taskId: string): string => resolveApiUrl(workspaceTaskSuffix(workspaceId, taskId)),
  cancel: (workspaceId: string, taskId: string): string => resolveApiUrl(workspaceTaskCancelSuffix(workspaceId, taskId)),
  outputs: (workspaceId: string, taskId: string): string => resolveApiUrl(workspaceTaskOutputsSuffix(workspaceId, taskId)),
  quotes: (workspaceId: string): string => resolveApiUrl(workspaceTaskQuotesSuffix(workspaceId)),
}

export async function listWorkspaceTasks(
  workspaceId: string,
  params?: { cursor?: string | null | undefined; pageSize?: number | undefined },
  options?: WorkspaceTasksRequestOptions,
): Promise<unknown> {
  return api.request<unknown>(withCursorQuery(resolveApiUrl(workspaceTasksSuffix(workspaceId)), params), toRequestConfig(options))
}

export async function createWorkspaceTask(
  workspaceId: string,
  input: CreateWorkspaceTaskInput,
  options?: WorkspaceTasksRequestOptions,
): Promise<WorkspaceTaskItem> {
  const body = {
    data: {
      workflow_kind: input.workflow_kind,
      inputs: { media_asset_ids: input.media_asset_ids ?? [] },
      ...(input.params == null ? {} : { params: input.params }),
      ...(input.quantity == null ? {} : { quantity: input.quantity }),
      ...(input.resolution == null ? {} : { resolution: input.resolution }),
    },
  }
  const raw = await api.request<unknown>(resolveApiUrl(workspaceTasksSuffix(workspaceId)), {
    ...toRequestConfig(options),
    method: 'POST',
    body: body as never,
  })
  // 202 Accepted carries the same envelope as 200; the Location header
  // (`/api/v1/workspaces/{ws}/tasks/{id}`) is authoritative when exposed.
  return toWorkspaceTask(raw)
}

export async function getWorkspaceTask(
  workspaceId: string,
  taskId: string,
  options?: WorkspaceTasksRequestOptions,
): Promise<WorkspaceTaskItem> {
  const raw = await api.request<unknown>(resolveApiUrl(workspaceTaskSuffix(workspaceId, taskId)), toRequestConfig(options))
  return toWorkspaceTask(raw)
}

export async function cancelWorkspaceTask(
  workspaceId: string,
  taskId: string,
  input?: { reason?: string | undefined },
  options?: WorkspaceTasksRequestOptions,
): Promise<WorkspaceTaskItem> {
  const body = input?.reason?.trim() ? { data: { reason: input.reason.trim() } } : undefined
  const raw = await api.request<unknown>(resolveApiUrl(workspaceTaskCancelSuffix(workspaceId, taskId)), {
    ...toRequestConfig(options),
    method: 'POST',
    ...(body == null ? {} : { body: body as never }),
  })
  return toWorkspaceTask(raw)
}

export async function listWorkspaceTaskOutputs(
  workspaceId: string,
  taskId: string,
  options?: WorkspaceTasksRequestOptions,
): Promise<{ task_id: string; outputs: WorkspaceTaskOutputRef[] }> {
  const raw = await api.request<unknown>(resolveApiUrl(workspaceTaskOutputsSuffix(workspaceId, taskId)), toRequestConfig(options))
  return toWorkspaceTaskOutputs(raw)
}

export async function quoteWorkspaceTask(
  workspaceId: string,
  input: { workflow_kind: string; quantity?: number | undefined; resolution?: string | undefined },
  options?: WorkspaceTasksRequestOptions,
): Promise<WorkspaceTaskBillingSnapshot> {
  const body = {
    data: {
      workflow_kind: input.workflow_kind,
      ...(input.quantity == null ? {} : { quantity: input.quantity }),
      ...(input.resolution == null ? {} : { resolution: input.resolution }),
    },
  }
  const raw = await api.request<unknown>(resolveApiUrl(workspaceTaskQuotesSuffix(workspaceId)), {
    ...toRequestConfig(options),
    method: 'POST',
    body: body as never,
  })
  return requireContractData<WorkspaceTaskBillingSnapshot>(raw, 'Quote failed')
}

// ---------------------------------------------------------------------------
// Workspace task waiters (issue #807).
//
// Terminal wait over the versioned workspace event stream with polling
// fallback. Legacy `/api/user` task waiting (`taskEvents.ts`) is untouched
// (removal is #809): this waiter only speaks workspace ids + the v1 stream.

const WORKSPACE_TASK_WAIT_TYPES = [
  'task.created',
  'task.running',
  'task.completed',
  'task.failed',
  'task.cancelled',
] as const

const WORKSPACE_TASK_WAIT_TIMEOUT_MS = 6 * 60 * 1000
const WORKSPACE_TASK_POLL_INITIAL_DELAY_MS = 1200
const WORKSPACE_TASK_POLL_MAX_DELAY_MS = 8000
const WORKSPACE_TASK_POLL_BACKOFF_FACTOR = 1.6

export type WaitWorkspaceTaskOptions = {
  timeoutMs?: number | undefined
  pollInitialDelayMs?: number | undefined
  pollMaxDelayMs?: number | undefined
  requestId?: string | undefined
  signal?: AbortSignal | undefined
}

/**
 * Resolve with the terminal workspace task (`completed`/`failed`/`cancelled`).
 * Prefers the workspace SSE stream (refresh on the matching `task.*`
 * envelope, since envelopes carry refs only); falls back to polling when
 * EventSource is unavailable or the transport errors. Rejects on timeout
 * or abort. Failure/copy mapping stays with the caller (feedback boundary).
 */
export function waitWorkspaceTaskTerminal(
  workspaceId: string,
  taskId: string,
  options?: WaitWorkspaceTaskOptions,
): Promise<WorkspaceTaskItem> {
  const workspace = assertWorkspaceTaskWorkspaceId(workspaceId)
  const task = assertWorkspaceTaskId(taskId)
  const timeoutMs = options?.timeoutMs ?? WORKSPACE_TASK_WAIT_TIMEOUT_MS
  const deadlineAt = Date.now() + timeoutMs
  const pollOptions = {
    initialDelayMs: options?.pollInitialDelayMs ?? WORKSPACE_TASK_POLL_INITIAL_DELAY_MS,
    maxDelayMs: options?.pollMaxDelayMs ?? WORKSPACE_TASK_POLL_MAX_DELAY_MS,
  }
  const readOptions = {
    ...(options?.requestId == null ? {} : { requestId: options.requestId }),
    ...(options?.signal == null ? {} : { signal: options.signal }),
  }
  if (typeof EventSource === 'undefined') {
    return pollWorkspaceTaskTerminal(workspace, task, deadlineAt, pollOptions, readOptions)
  }
  return waitWorkspaceTaskWithStream(workspace, task, deadlineAt, pollOptions, readOptions)
}

function waitWorkspaceTaskWithStream(
  workspaceId: string,
  taskId: string,
  deadlineAt: number,
  pollOptions: { initialDelayMs: number; maxDelayMs: number },
  readOptions: WorkspaceTasksRequestOptions,
): Promise<WorkspaceTaskItem> {
  return new Promise((resolve, reject) => {
    let settled = false
    let timeout = 0
    let refreshing = false
    let unsubscribe: () => void = () => undefined

    const cleanup = () => {
      window.clearTimeout(timeout)
      readOptions.signal?.removeEventListener('abort', onAbort)
      unsubscribe()
    }
    const settleResolve = (item: WorkspaceTaskItem) => {
      if (settled) return
      settled = true
      cleanup()
      resolve(item)
    }
    const settleReject = (error: Error) => {
      if (settled) return
      settled = true
      cleanup()
      reject(error)
    }
    const fallbackToPolling = () => {
      if (settled) return
      settled = true
      cleanup()
      pollWorkspaceTaskTerminal(workspaceId, taskId, deadlineAt, pollOptions, readOptions).then(
        resolve,
        reject,
      )
    }
    const onAbort = () => {
      settleReject(new Error('Workspace task wait aborted'))
    }
    readOptions.signal?.addEventListener('abort', onAbort, { once: true })

    timeout = window.setTimeout(
      () => {
        settleReject(new Error('Workspace task wait timed out'))
      },
      Math.max(0, deadlineAt - Date.now()),
    )

    const refreshTask = async () => {
      if (settled || refreshing) return
      refreshing = true
      try {
        const item = await getWorkspaceTask(workspaceId, taskId, readOptions)
        if (isWorkspaceTaskTerminal(item.status)) settleResolve(item)
      } catch (error) {
        if (isWorkspaceTaskGone(error)) settleReject(error as Error)
      } finally {
        refreshing = false
      }
    }

    try {
      unsubscribe = subscribeWorkspaceEvent(
        workspaceId,
        (envelope) => {
          const raw = envelope.data.task_id
          if (typeof raw === 'string' && raw.trim() === taskId) void refreshTask()
        },
        { types: [...WORKSPACE_TASK_WAIT_TYPES], onError: fallbackToPolling },
      )
    } catch {
      fallbackToPolling()
      return
    }
    // Reconcile current state in case the task settled before subscribing.
    void refreshTask()
  })
}

async function pollWorkspaceTaskTerminal(
  workspaceId: string,
  taskId: string,
  deadlineAt: number,
  options: { initialDelayMs: number; maxDelayMs: number },
  readOptions: WorkspaceTasksRequestOptions,
): Promise<WorkspaceTaskItem> {
  const nextDelay = createPollDelay({
    initialDelayMs: options.initialDelayMs,
    maxDelayMs: options.maxDelayMs,
    backoffFactor: WORKSPACE_TASK_POLL_BACKOFF_FACTOR,
  })
  for (;;) {
    if (readOptions.signal?.aborted === true) throw new Error('Workspace task wait aborted')
    let item: WorkspaceTaskItem | undefined
    try {
      item = await getWorkspaceTask(workspaceId, taskId, readOptions)
    } catch (error) {
      if (isWorkspaceTaskGone(error)) throw error
    }
    if (item && isWorkspaceTaskTerminal(item.status)) return item
    const remainingMs = deadlineAt - Date.now()
    if (remainingMs <= 0) break
    await sleep(Math.min(nextDelay.next(), remainingMs))
  }
  throw new Error('Workspace task wait timed out')
}

function isWorkspaceTaskGone(error: unknown): boolean {
  if (typeof error !== 'object' || error === null) return false
  const status = 'status' in error ? (error as { status?: unknown }).status : undefined
  if (status === 404) return true
  const code = 'code' in error ? (error as { code?: unknown }).code : undefined
  return code === 'task_not_found' || code === 404
}
