// Workspace task terminal waiter (issue #859).
//
// Terminal wait over the versioned workspace event stream with polling
// fallback, split out of `workspaceTasks.ts`. Canvas image-result waiting
// composes this waiter in
// `features/canvas/.../workspace/canvasTaskWaiting` (the legacy `/api/user`
// task waiter is retired).
//
// Race contract (preserved exactly): prefers the workspace SSE stream
// (refresh on the matching `task.*` envelope, since envelopes carry refs
// only); falls back to polling when EventSource is unavailable or the
// transport errors; reconciles current state after subscribing in case the
// task settled first; rejects on timeout or abort. Failure/copy mapping
// stays with the caller (feedback boundary).

import {
  assertWorkspaceTaskId,
  assertWorkspaceTaskWorkspaceId,
  isWorkspaceTaskTerminal,
  type WorkspaceTaskItem,
  type WorkspaceTasksRequestOptions,
} from './workspaceTaskContract'
import { getWorkspaceTask, isWorkspaceTaskGoneError } from './workspaceTaskTransport'
import { createPollDelay, sleep } from '../core/utils/polling'
import {
  TASK_POLL_BACKOFF_FACTOR,
  TASK_POLL_INITIAL_DELAY_MS,
  TASK_POLL_MAX_DELAY_MS,
} from '../core/config/timing'
import { subscribeWorkspaceEvent } from './workspaceEventStream'
import { runWithDeadline } from '../core/utils/request'

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
  deadlineAt?: number | undefined
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
    // The initial GET, stream refreshes and polling share one deadline.
    try {
      const item = await getWorkspaceTask(workspace, task, readOptions)
      if (isWorkspaceTaskTerminal(item.status)) return item
    } catch (error) {
      if (isWorkspaceTaskGoneError(error)) throw error
      signal.throwIfAborted()
    }
    if (typeof EventSource === 'undefined') {
      return pollWorkspaceTaskTerminal(workspace, task, deadlineAt, pollOptions, readOptions)
    }
    return waitWorkspaceTaskWithStream(workspace, task, deadlineAt, pollOptions, readOptions)
  })
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
    let refreshing = false
    let unsubscribe: () => void = () => undefined

    const cleanup = () => {
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
      settleReject(readOptions.signal?.reason ?? new Error('Workspace task wait aborted'))
    }
    readOptions.signal?.addEventListener('abort', onAbort, { once: true })

    const refreshTask = async () => {
      if (settled || refreshing) return
      refreshing = true
      try {
        const item = await getWorkspaceTask(workspaceId, taskId, readOptions)
        if (isWorkspaceTaskTerminal(item.status)) settleResolve(item)
      } catch (error) {
        if (isWorkspaceTaskGoneError(error)) settleReject(error as Error)
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
      if (isWorkspaceTaskGoneError(error)) throw error
    }
    if (item && isWorkspaceTaskTerminal(item.status)) return item
    const remainingMs = deadlineAt - Date.now()
    if (remainingMs <= 0) break
    await sleep(Math.min(nextDelay.next(), remainingMs), readOptions.signal ?? undefined)
  }
  throw new Error('Workspace task wait timed out')
}
