// Workspace task lifecycle resource (issue #805, Public Data Plane).
//
// Feature-neutral resource over the generated `generated-query/public`
// workspace-tasks hooks (prompt + mask_asset_id in the model). Thin
// snake_case transport types only: snake_case -> camelCase mapping lives
// in canvas task-runner adapters (keep this resource thin).
//
// Transport rides `publicMutator` (`/api/v1/...`) through the generated
// functions; this module keeps the opaque-id asserts, status convergence
// (`cancelled_with_results` -> `cancelled`), storage-leak guards, and
// terminal waiters (SSE with polling fallback) that have no generated
// equivalent. Route pages must consume the canvas runner, never this
// module directly.
//
// Legacy `/api/user` task behavior (submit/readiness/matrix/cancel/wait) is
// retired (W2 stream 1): canvas owns its workspace task adapter in
// `features/canvas/application/execution/workspace/canvasWorkspaceTaskAdapter`.

import {
  cancelWorkspaceTask as cancelWorkspaceTaskTransport,
  createWorkspaceTask as createWorkspaceTaskTransport,
  getWorkspaceTask as getWorkspaceTaskTransport,
  listWorkspaceTaskOutputs as listWorkspaceTaskOutputsTransport,
  listWorkspaceTasks as listWorkspaceTasksTransport,
  quoteWorkspaceTask as quoteWorkspaceTaskTransport,
} from './generated-query/public/workspace-tasks/workspace-tasks'
import type {
  CreateWorkspaceTaskRequest,
  PaginatedWorkspaceTasks,
  QuoteWorkspaceTaskRequest,
} from './generated-query/public/model'
import {
  buildContractRequestHeaders,
  isOpaqueId,
  newRequestId,
  opaqueIdPrefix,
  requireContractData,
  type OpaqueIdPrefix,
} from './contractKernel'
import { createPollDelay, sleep } from '../utils/polling'
import { PAGE_SIZES } from '../config/pagination'
import {
  TASK_POLL_BACKOFF_FACTOR,
  TASK_POLL_INITIAL_DELAY_MS,
  TASK_POLL_MAX_DELAY_MS,
} from '../config/timing'
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

// Canonical browser URLs (full `/api/v1/...` form for Location handling
// and link sharing). The server `Location` header on 202 carries the same
// shape; clients derive it from the returned id when headers are opaque.
// No generated equivalent (orval URL helpers return bare `/workspaces/...`
// paths without the `/api/v1` prefix or Location semantics).
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
  /**
   * Free-text prompt (v1 `data.prompt`, max 8KB bytes). Required for
   * generation/inpaint, optional for render/variations/fusion/outpaint,
   * never sent for cutout/upscale (see the canvas adapter prompt policy).
   */
  prompt?: string | undefined
  params?: Record<string, unknown> | undefined
  quantity?: number | undefined
  resolution?: string | undefined
}

export type WorkspaceTasksRequestOptions = {
  signal?: AbortSignal | null | undefined
  requestId?: string | undefined
  idempotencyKey?: string | undefined
}

function toGeneratedOptions(options: WorkspaceTasksRequestOptions | undefined): {
  headers: Record<string, string>
  signal?: AbortSignal | undefined
} {
  return {
    headers: buildContractRequestHeaders({
      requestId: options?.requestId?.trim() ? options.requestId.trim() : newRequestId(),
      ...(options?.idempotencyKey == null ? {} : { idempotencyKey: options.idempotencyKey }),
    }),
    ...(options?.signal == null ? {} : { signal: options.signal }),
  }
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

export async function listWorkspaceTasks(
  workspaceId: string,
  params?: { cursor?: string | null | undefined; pageSize?: number | undefined },
  options?: WorkspaceTasksRequestOptions,
): Promise<PaginatedWorkspaceTasks> {
  const query: { cursor?: string; page_size?: number } = { page_size: params?.pageSize ?? PAGE_SIZES.standard }
  if (params?.cursor?.trim()) query.cursor = params.cursor.trim()
  return listWorkspaceTasksTransport(
    assertWorkspaceTaskWorkspaceId(workspaceId),
    query,
    toGeneratedOptions(options),
  )
}

export async function createWorkspaceTask(
  workspaceId: string,
  input: CreateWorkspaceTaskInput,
  options?: WorkspaceTasksRequestOptions,
): Promise<WorkspaceTaskItem> {
  const data: CreateWorkspaceTaskRequest['data'] = {
    workflow_kind: input.workflow_kind as CreateWorkspaceTaskRequest['data']['workflow_kind'],
    inputs: { media_asset_ids: input.media_asset_ids ?? [] },
  }
  if (input.prompt != null) data.prompt = input.prompt
  if (input.params != null) {
    data.params = input.params as NonNullable<CreateWorkspaceTaskRequest['data']['params']>
  }
  if (input.quantity != null) data.quantity = input.quantity
  if (input.resolution != null) data.resolution = input.resolution
  const envelope = await createWorkspaceTaskTransport(
    assertWorkspaceTaskWorkspaceId(workspaceId),
    { data },
    toGeneratedOptions(options),
  )
  // 202 Accepted carries the same envelope as 200; the Location header
  // (`/api/v1/workspaces/{ws}/tasks/{id}`) is authoritative when exposed.
  return toWorkspaceTask(envelope)
}

export async function getWorkspaceTask(
  workspaceId: string,
  taskId: string,
  options?: WorkspaceTasksRequestOptions,
): Promise<WorkspaceTaskItem> {
  const envelope = await getWorkspaceTaskTransport(
    assertWorkspaceTaskWorkspaceId(workspaceId),
    assertWorkspaceTaskId(taskId),
    toGeneratedOptions(options),
  )
  return toWorkspaceTask(envelope)
}

export async function cancelWorkspaceTask(
  workspaceId: string,
  taskId: string,
  input?: { reason?: string | undefined },
  options?: WorkspaceTasksRequestOptions,
): Promise<WorkspaceTaskItem> {
  const envelope = await cancelWorkspaceTaskTransport(
    assertWorkspaceTaskWorkspaceId(workspaceId),
    assertWorkspaceTaskId(taskId),
    input?.reason?.trim() ? { data: { reason: input.reason.trim() } } : undefined,
    toGeneratedOptions(options),
  )
  return toWorkspaceTask(envelope)
}

export async function listWorkspaceTaskOutputs(
  workspaceId: string,
  taskId: string,
  options?: WorkspaceTasksRequestOptions,
): Promise<{ task_id: string; outputs: WorkspaceTaskOutputRef[] }> {
  const envelope = await listWorkspaceTaskOutputsTransport(
    assertWorkspaceTaskWorkspaceId(workspaceId),
    assertWorkspaceTaskId(taskId),
    toGeneratedOptions(options),
  )
  return toWorkspaceTaskOutputs(envelope)
}

export async function quoteWorkspaceTask(
  workspaceId: string,
  input: { workflow_kind: string; quantity?: number | undefined; resolution?: string | undefined },
  options?: WorkspaceTasksRequestOptions,
): Promise<WorkspaceTaskBillingSnapshot> {
  const data: QuoteWorkspaceTaskRequest['data'] = {
    workflow_kind: input.workflow_kind as QuoteWorkspaceTaskRequest['data']['workflow_kind'],
  }
  if (input.quantity != null) data.quantity = input.quantity
  if (input.resolution != null) data.resolution = input.resolution
  const envelope = await quoteWorkspaceTaskTransport(
    assertWorkspaceTaskWorkspaceId(workspaceId),
    { data },
    toGeneratedOptions(options),
  )
  return requireContractData<WorkspaceTaskBillingSnapshot>(envelope, 'Quote failed')
}

// ---------------------------------------------------------------------------
// Workspace task waiters (issue #807).
//
// Terminal wait over the versioned workspace event stream with polling
// fallback. Canvas image-result waiting composes this waiter in
// `features/canvas/.../canvasWorkspaceTaskAdapter` (the legacy `/api/user`
// task waiter is retired).

const WORKSPACE_TASK_WAIT_TYPES = [
  'task.created',
  'task.running',
  'task.completed',
  'task.failed',
  'task.cancelled',
] as const

const WORKSPACE_TASK_WAIT_TIMEOUT_MS = 6 * 60 * 1000
const WORKSPACE_TASK_POLL_INITIAL_DELAY_MS = TASK_POLL_INITIAL_DELAY_MS
const WORKSPACE_TASK_POLL_MAX_DELAY_MS = TASK_POLL_MAX_DELAY_MS
const WORKSPACE_TASK_POLL_BACKOFF_FACTOR = TASK_POLL_BACKOFF_FACTOR

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
