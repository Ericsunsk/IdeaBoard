import {
  extractImageResultsFromTaskPayload,
  type TaskImageResult,
  type TaskResultPayloadLike,
} from '../query/taskResult'
import { sanitizeApiErrorMessage } from '../utils/errorMessages'
import { taskApi } from './task'
import type { Task } from './task'
import { notifyUserCreditsInvalidation } from '../query/userCreditsInvalidation'
import { i18n } from '../i18n'
import { subscribeUserEvent } from './userEventStream'

type TaskEventPayload = Pick<
  Task,
  'task_id' | 'status' | 'result' | 'error_msg' | 'outputs' | 'cancel_reason' | 'execution_metrics'
>

const SSE_TIMEOUT_MS = 6 * 60 * 1000
const POLL_INITIAL_DELAY_MS = 1200
const POLL_MAX_DELAY_MS = 8000
const POLL_BACKOFF_FACTOR = 1.6

type WaitTaskResultOptions = {
  timeoutMs?: number
  pollInitialDelayMs?: number
  pollMaxDelayMs?: number
}

export type TaskWaitFailureKind = 'pending' | 'timeout'

/** Transport-level facts only: the feedback boundary translates `kind` to copy. */
export class TaskResultPendingError extends Error {
  readonly kind: TaskWaitFailureKind

  constructor(kind: TaskWaitFailureKind = 'pending', message = '') {
    super(message)
    this.name = 'TaskResultPendingError'
    this.kind = kind
  }
}

export class TaskCancelledError extends Error {
  readonly kind = 'cancelled' as const

  constructor(message = '') {
    super(message)
    this.name = 'TaskCancelledError'
  }
}

export function isTaskResultPendingError(error: unknown): error is TaskResultPendingError {
  return error instanceof TaskResultPendingError
}

type TaskFailurePayload = Pick<TaskEventPayload, 'error_msg' | 'execution_metrics'>

function normalizeTaskFailureValue(value: unknown): string {
  return typeof value === 'string' ? value.trim() : ''
}

function isResultFinalizationFailure(task: TaskFailurePayload): boolean {
  const metrics = task.execution_metrics
  const failureStage = normalizeTaskFailureValue(metrics?.failure_stage)
  const failureCategory = normalizeTaskFailureValue(metrics?.failure_category)
  if (
    failureStage === 'finalizing' &&
    (failureCategory === 'storage_error' || failureCategory === 'finalize_error')
  ) {
    return true
  }

  return false
}

export function createTaskResultFailureError(
  task: TaskFailurePayload,
  fallbackMessage = i18n.global.t('canvas.feedback.imageGenerationFailed'),
): Error {
  if (isResultFinalizationFailure(task)) {
    return new TaskResultPendingError('pending')
  }
  const msg = task.error_msg?.trim() || fallbackMessage
  return new Error(sanitizeApiErrorMessage(msg, fallbackMessage))
}

export function waitTaskImageResultsWithSse(
  taskId: string,
  options?: WaitTaskResultOptions,
): Promise<TaskImageResult[]> {
  const timeoutMs = options?.timeoutMs ?? SSE_TIMEOUT_MS
  const deadlineAt = Date.now() + timeoutMs
  const pollOptions = {
    initialDelayMs: options?.pollInitialDelayMs ?? POLL_INITIAL_DELAY_MS,
    maxDelayMs: options?.pollMaxDelayMs ?? POLL_MAX_DELAY_MS,
  }
  if (typeof EventSource === 'undefined') {
    return waitWithPolling(taskId, deadlineAt, pollOptions)
  }
  return waitWithEventSource(taskId, deadlineAt, pollOptions)
}

function waitWithEventSource(
  taskId: string,
  deadlineAt: number,
  pollOptions: { initialDelayMs: number; maxDelayMs: number },
): Promise<TaskImageResult[]> {
  return new Promise((resolve, reject) => {
    let settled = false
    let timeout = 0
    let refreshing = false
    let unsubscribe: () => void = () => undefined

    const cleanup = () => {
      clearTimeout(timeout)
      unsubscribe()
    }

    const settleResolve = (value: TaskImageResult[]) => {
      if (settled) return
      settled = true
      cleanup()
      resolve(value)
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
      waitWithPolling(taskId, deadlineAt, pollOptions).then(resolve, reject)
    }

    timeout = window.setTimeout(
      () => {
        settleReject(new TaskResultPendingError('timeout'))
      },
      Math.max(0, deadlineAt - Date.now()),
    )

    const handleTaskEvent = (task: TaskEventPayload) => {
      if (!task || typeof task !== 'object' || typeof task.status !== 'string') {
        settleReject(new Error('Malformed task event payload'))
        return
      }

      if (task.status === 'completed') {
        notifyUserCreditsInvalidation({ source: 'task-settlement', taskId, status: 'completed' })
        settleResolve(extractImageResultsFromTaskEvent(task))
        return
      }
      if (task.status === 'failed') {
        notifyUserCreditsInvalidation({ source: 'task-settlement', taskId, status: 'failed' })
        settleReject(createTaskResultFailureError(task))
        return
      }
      if (task.status === 'cancelled') {
        notifyUserCreditsInvalidation({ source: 'task-settlement', taskId, status: 'cancelled' })
        settleReject(createTaskCancelledError(task))
        return
      }
      const cancelledResults = resolveCancelledWithResults(task)
      if (cancelledResults) {
        notifyUserCreditsInvalidation({ source: 'task-settlement', taskId, status: 'cancelled' })
        settleResolve(cancelledResults)
        return
      }
      if (task.status === 'cancelled_with_results') {
        notifyUserCreditsInvalidation({ source: 'task-settlement', taskId, status: 'cancelled' })
        settleReject(createTaskCancelledError(task))
      }
    }

    const refreshTask = async () => {
      if (settled || refreshing) return
      refreshing = true
      try {
        const response = await taskApi.get(taskId)
        handleTaskEvent(response.task)
      } catch (error) {
        if (isTaskNotFoundError(error)) settleReject(error as Error)
      } finally {
        refreshing = false
      }
    }

    unsubscribe = subscribeUserEvent(
      'task.updated',
      (event) => {
        try {
          const payload = JSON.parse(event.data) as { task_id?: unknown }
          if (payload.task_id === taskId) void refreshTask()
        } catch {
          settleReject(new Error('Malformed task event payload'))
        }
      },
      fallbackToPolling,
    )
  })
}

async function waitWithPolling(
  taskId: string,
  deadlineAt: number,
  options: { initialDelayMs: number; maxDelayMs: number },
): Promise<TaskImageResult[]> {
  let nextDelayMs = options.initialDelayMs
  let lastError: unknown

  while (Date.now() <= deadlineAt) {
    let task: TaskEventPayload | undefined
    try {
      const taskRes = await taskApi.get(taskId)
      task = taskRes.task
      lastError = undefined
    } catch (error) {
      if (isTaskNotFoundError(error)) throw error
      lastError = error
    }

    if (task?.status === 'completed') {
      notifyUserCreditsInvalidation({ source: 'task-settlement', taskId, status: 'completed' })
      return extractImageResultsFromTaskEvent(task)
    }
    if (task?.status === 'failed') {
      notifyUserCreditsInvalidation({ source: 'task-settlement', taskId, status: 'failed' })
      throw createTaskResultFailureError(task)
    }
    if (task?.status === 'cancelled') {
      notifyUserCreditsInvalidation({ source: 'task-settlement', taskId, status: 'cancelled' })
      throw createTaskCancelledError(task)
    }
    if (task) {
      const cancelledResults = resolveCancelledWithResults(task)
      if (cancelledResults) {
        notifyUserCreditsInvalidation({ source: 'task-settlement', taskId, status: 'cancelled' })
        return cancelledResults
      }
      if (task.status === 'cancelled_with_results') {
        notifyUserCreditsInvalidation({ source: 'task-settlement', taskId, status: 'cancelled' })
        throw createTaskCancelledError(task)
      }
    }

    const remainingMs = deadlineAt - Date.now()
    if (remainingMs <= 0) break
    await sleep(Math.min(nextDelayMs, remainingMs))
    nextDelayMs = Math.min(
      options.maxDelayMs,
      Math.max(nextDelayMs + 1, Math.ceil(nextDelayMs * POLL_BACKOFF_FACTOR)),
    )
  }

  if (lastError) {
    throw new TaskResultPendingError('timeout')
  }
  throw new TaskResultPendingError('timeout')
}

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => {
    window.setTimeout(resolve, ms)
  })
}

function isTaskNotFoundError(error: unknown): boolean {
  if (typeof error !== 'object' || !error) return false
  const status = 'status' in error ? (error as { status?: unknown }).status : undefined
  if (status === 404) return true
  const code = 'code' in error ? (error as { code?: unknown }).code : undefined
  return code === 404
}

export function createTaskCancelledError(
  task?: Pick<Task, 'error_msg' | 'cancel_reason'>,
): TaskCancelledError {
  const detail = task?.cancel_reason?.trim() || task?.error_msg?.trim() || ''
  // Backend reason text stays as debug detail; the feedback boundary shows copy for kind='cancelled'.
  return new TaskCancelledError(sanitizeApiErrorMessage(detail, ''))
}

function extractImageResultsFromTaskEvent(task: TaskEventPayload): TaskImageResult[] {
  return extractImageResultsFromTaskPayload(task)
}

/**
 * Unified terminal-status reconciliation (issue #676): `cancelled_with_results`
 * materializes like a completion when the receipt bundles image outputs, and
 * surfaces a cancellation otherwise. Callers treat every terminal status
 * through this single seam instead of fragmenting the mapping per transport.
 * The payload is intentionally narrow (status + outputs) so fast-path task
 * views without a full event envelope can share it.
 */
export function resolveCancelledWithResults(task: {
  status: string
  outputs?: TaskResultPayloadLike['outputs']
}): TaskImageResult[] | null {
  if (task.status !== 'cancelled_with_results') return null
  const results = extractImageResultsFromTaskPayload({ outputs: task.outputs ?? [] })
  return results.length > 0 ? results : null
}
