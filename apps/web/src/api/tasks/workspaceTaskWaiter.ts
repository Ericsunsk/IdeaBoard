// Workspace task terminal waiter (issue #859).
//
// Terminal wait over the versioned workspace event stream with polling
// fallback, split out of `workspaceTasks.ts`. Canvas image-result waiting
// composes this waiter in
// `features/canvas/.../workspace/canvasTaskWaiting` (the legacy `/api/user`
// task waiter is retired).
//
// Prefers the workspace SSE stream, using self-contained completion outputs
// with the latest Task snapshot and refreshing other matching envelopes.
// Falls back to polling when EventSource is unavailable or the
// transport errors; reconciles after subscribing and at low frequency because
// a healthy live-only stream can miss terminal events; rejects on timeout or
// abort. Failure/copy mapping stays with the caller (feedback boundary).

import {
  assertWorkspaceTaskId,
  assertWorkspaceTaskWorkspaceId,
  isWorkspaceTaskTerminal,
  type WorkspaceTaskItem,
  type WorkspaceTasksRequestOptions,
} from './workspaceTaskContract'
import { getWorkspaceTask, isWorkspaceTaskGoneError } from './workspaceTaskTransport'
import { createPollDelay, sleep } from '../../core/utils/polling'
import {
  TASK_POLL_BACKOFF_FACTOR,
  TASK_POLL_INITIAL_DELAY_MS,
  TASK_POLL_MAX_DELAY_MS,
} from '../../core/config/timing'
import { subscribeWorkspaceEvent } from '../realtime/workspaceEventStream'
import { runWithDeadline } from '../../core/utils/request'

const WORKSPACE_TASK_WAIT_TYPES = [
  'task.created',
  'task.running',
  'task.completed',
  'task.failed',
  'task.cancelled',
] as const

const WORKSPACE_TASK_WAIT_TIMEOUT_MS = 6 * 60 * 1000
const WORKSPACE_TASK_RECONCILE_INTERVAL_MS = 15_000
const WORKSPACE_TASK_POLL_INITIAL_DELAY_MS = TASK_POLL_INITIAL_DELAY_MS
const WORKSPACE_TASK_POLL_MAX_DELAY_MS = TASK_POLL_MAX_DELAY_MS
const WORKSPACE_TASK_POLL_BACKOFF_FACTOR = TASK_POLL_BACKOFF_FACTOR

export type WaitWorkspaceTaskOptions = {
  deadlineAt?: number | undefined
  timeoutMs?: number | undefined
  pollInitialDelayMs?: number | undefined
  pollMaxDelayMs?: number | undefined
  requestId?: string | undefined
  signal?: AbortSignal | undefined
}

/**
 * Resolve with the terminal workspace task (`completed`/`failed`/`cancelled`).
 * Prefers completion outputs from the workspace SSE stream and refreshes
 * matching task events as needed, with low-frequency reconciliation for missed
 * notifications; falls back to polling when
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
  const deadlineAt = options?.deadlineAt ?? Date.now() + timeoutMs
  const pollOptions = {
    initialDelayMs: options?.pollInitialDelayMs ?? WORKSPACE_TASK_POLL_INITIAL_DELAY_MS,
    maxDelayMs: options?.pollMaxDelayMs ?? WORKSPACE_TASK_POLL_MAX_DELAY_MS,
  }
  return runWithDeadline(deadlineAt, options?.signal, async (signal) => {
    const readOptions = {
      ...(options?.requestId == null ? {} : { requestId: options.requestId }),
      signal,
    }
    let initialItem: WorkspaceTaskItem | undefined
    // The initial GET, stream refreshes and polling share one deadline.
    try {
      const item = await getWorkspaceTask(workspace, task, readOptions)
      if (isWorkspaceTaskTerminal(item.status)) return item
      initialItem = item
    } catch (error) {
      if (isWorkspaceTaskGoneError(error)) throw error
      signal.throwIfAborted()
    }
    if (typeof EventSource === 'undefined') {
      return pollWorkspaceTaskTerminal(workspace, task, deadlineAt, pollOptions, readOptions)
    }
    return waitWorkspaceTaskWithStream(
      workspace,
      task,
      deadlineAt,
      pollOptions,
      readOptions,
      initialItem,
    )
  })
}

function parseEventOutputs(outputs: Array<Record<string, unknown>>) {
  return outputs.map((out, index) => {
    const assetId =
      typeof out.asset_id === 'string' && out.asset_id.trim() ? out.asset_id.trim() : undefined
    const url = typeof out.url === 'string' && out.url.trim() ? out.url.trim() : undefined
    const role = typeof out.role === 'string' && out.role.trim() ? out.role.trim() : 'result'
    const resultIndex = typeof out.result_index === 'number' ? out.result_index : index
    const width = typeof out.width === 'number' && out.width > 0 ? out.width : undefined
    const height = typeof out.height === 'number' && out.height > 0 ? out.height : undefined
    const mimeType =
      typeof out.mime_type === 'string' && out.mime_type.trim() ? out.mime_type.trim() : undefined
    return {
      kind: assetId ? ('asset' as const) : ('remote' as const),
      asset_id: assetId,
      url,
      role,
      result_index: resultIndex,
      width,
      height,
      mime_type: mimeType,
      // eslint-disable-next-line governance/boolean-prefix -- Wire contract field
      preview_ready: out.preview_ready === true,
    }
  })
}

function buildCompletedItemFromEvent(
  currentItem: WorkspaceTaskItem,
  outputRefs: ReturnType<typeof parseEventOutputs>,
  occurredAt: string,
): WorkspaceTaskItem {
  return {
    ...currentItem,
    status: 'completed',
    output_refs: outputRefs,
    updated_at: occurredAt,
  }
}

function waitWorkspaceTaskWithStream(
  workspaceId: string,
  taskId: string,
  deadlineAt: number,
  pollOptions: { initialDelayMs: number; maxDelayMs: number },
  readOptions: WorkspaceTasksRequestOptions,
  initialItem?: WorkspaceTaskItem | undefined,
): Promise<WorkspaceTaskItem> {
  return new Promise((resolve, reject) => {
    let settled = false
    let refreshPromise: Promise<WorkspaceTaskItem> | undefined
    let hasPendingRefresh = false
    let currentItem = initialItem
    let unsubscribe: () => void = () => undefined
    let reconcileTimer: ReturnType<typeof setTimeout> | undefined

    const cleanup = () => {
      clearTimeout(reconcileTimer)
      reconcileTimer = undefined
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
      Promise.resolve(refreshPromise)
        .catch(() => undefined)
        .then((item) => {
          readOptions.signal?.throwIfAborted()
          if (item && isWorkspaceTaskTerminal(item.status)) return item
          return pollWorkspaceTaskTerminal(
            workspaceId,
            taskId,
            deadlineAt,
            pollOptions,
            readOptions,
          )
        })
        .then(resolve, reject)
    }
    const onAbort = () => {
      settleReject(readOptions.signal?.reason ?? new Error('Workspace task wait aborted'))
    }
    readOptions.signal?.addEventListener('abort', onAbort, { once: true })

    const scheduleReconciliation = () => {
      if (settled) return
      clearTimeout(reconcileTimer)
      reconcileTimer = setTimeout(() => {
        reconcileTimer = undefined
        // An in-flight GET owns the next timer; a timer never queues another GET.
        if (refreshPromise === undefined) void refreshTask()
      }, WORKSPACE_TASK_RECONCILE_INTERVAL_MS)
    }

    const refreshTask = async () => {
      if (settled) return
      if (refreshPromise !== undefined) {
        // The in-flight snapshot may predate this event.
        hasPendingRefresh = true
        return
      }
      try {
        refreshPromise = getWorkspaceTask(workspaceId, taskId, readOptions)
        const item = await refreshPromise
        currentItem = item
        if (isWorkspaceTaskTerminal(item.status)) settleResolve(item)
        else scheduleReconciliation()
      } catch (error) {
        if (isWorkspaceTaskGoneError(error)) settleReject(error as Error)
      } finally {
        refreshPromise = undefined
        if (hasPendingRefresh) {
          hasPendingRefresh = false
          void refreshTask()
        } else if (reconcileTimer === undefined) {
          scheduleReconciliation()
        }
      }
    }

    try {
      unsubscribe = subscribeWorkspaceEvent(
        workspaceId,
        (envelope) => {
          const raw = envelope.data.task_id
          if (typeof raw === 'string' && raw.trim() === taskId) {
            if (
              envelope.type === 'task.completed' &&
              currentItem !== undefined &&
              Array.isArray(envelope.data.outputs) &&
              envelope.data.outputs.length > 0
            ) {
              const outputRefs = parseEventOutputs(
                envelope.data.outputs as Array<Record<string, unknown>>,
              )
              const item = buildCompletedItemFromEvent(
                currentItem,
                outputRefs,
                envelope.occurred_at,
              )
              settleResolve(item)
              return
            }
            void refreshTask()
          }
        },
        { types: [...WORKSPACE_TASK_WAIT_TYPES], onError: fallbackToPolling },
      )
    } catch {
      fallbackToPolling()
      return
    }
    // Close the gap between the initial snapshot and listener registration:
    // a healthy live-only stream cannot replay completion from that window.
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
      if (isWorkspaceTaskGoneError(error)) throw error
    }
    if (item && isWorkspaceTaskTerminal(item.status)) return item
    const remainingMs = deadlineAt - Date.now()
    if (remainingMs <= 0) break
    await sleep(Math.min(nextDelay.next(), remainingMs), readOptions.signal ?? undefined)
  }
  throw new Error('Workspace task wait timed out')
}
